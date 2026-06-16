#!/usr/bin/env bash
# cks-mcpd.sh — run/manage MULTIPLE cks-mcp instances on one machine over HTTP.
#
# cks-mcp supports two transports (see internal/config ListenConfig):
#   - stdio: one client per subprocess (Claude Code spawns it per session)
#   - http : Streamable HTTP on a port — lets ONE host run several cks instances
#            (different DBs/models) on different ports, shared across sessions.
# This manager wraps the http path: each named instance gets its own port,
# derived config, pidfile, and log, and registers into Claude Code under a
# distinct server name so `/mcp` lists them as separate MCPs
# (tools namespaced mcp__cks-<name>__cks_*).
#
# Usage:
#   ./scripts/cks-mcpd.sh start  <name> [--config <path>] [--port <n>] \
#                                       [--host <ip> | --bind <host> --advertise <ip>] \
#                                       [--allow-remote] [--cidr <cidr>]...
#   ./scripts/cks-mcpd.sh stop   <name> | --all
#
# Networking (default = loopback-only, safe):
#   --host <ip>        bind AND advertise on <ip> (LAN reachable). Auto-sets allow_remote.
#   --bind 0.0.0.0     bind all interfaces; pair with --advertise <reachable-ip> for register.
#   --advertise <ip>   IP that `register` writes into the Claude Code URL.
#   --allow-remote     required to bind a non-loopback addr (auto-on when --host/--bind is non-loopback).
#   --cidr <a.b.c.d/n> permit a client network (repeatable). Without it, only loopback+LAN/private.
#                      cks has NO per-client auth — it filters by source IP only.
#   ./scripts/cks-mcpd.sh restart <name>
#   ./scripts/cks-mcpd.sh list
#   ./scripts/cks-mcpd.sh status <name>
#   ./scripts/cks-mcpd.sh logs   <name> [-f]
#   ./scripts/cks-mcpd.sh register   <name> [--scope user|local|project]
#   ./scripts/cks-mcpd.sh unregister <name>
#
# Examples:
#   ./scripts/cks-mcpd.sh start stablenet --config cks-stablenet.yaml --port 8801
#   ./scripts/cks-mcpd.sh start txpool    --config cks-txpool.yaml             # auto-port
#   ./scripts/cks-mcpd.sh register stablenet      # claude mcp add --transport http ...
#   ./scripts/cks-mcpd.sh list
#   ./scripts/cks-mcpd.sh stop --all
#
# Env overrides:
#   CKS_MCP_BIN   path to cks-mcp binary   (default: <repo>/bin/cks-mcp)
#   CKS_RUN_DIR   instance state directory (default: <repo>/run/cks-mcpd)
#   CKS_PORT_BASE first port for auto-pick (default: 8801)
set -euo pipefail

abs() { ( cd "$1" 2>/dev/null && pwd ) || { echo "ERROR: path not found: $1" >&2; exit 1; }; }

CKS_ROOT="$(abs "$(dirname "${BASH_SOURCE[0]}")/..")"
CKS_MCP_BIN="${CKS_MCP_BIN:-$CKS_ROOT/bin/cks-mcp}"
RUN_DIR="${CKS_RUN_DIR:-$CKS_ROOT/run/cks-mcpd}"
PORT_BASE="${CKS_PORT_BASE:-8801}"
PORT_MAX=8899
NAME_PREFIX="cks-"   # Claude Code server name = ${NAME_PREFIX}${name}

die()  { echo "cks-mcpd: $*" >&2; exit 1; }
info() { echo "cks-mcpd: $*" >&2; }

inst_dir()  { echo "$RUN_DIR/$1"; }
pidfile()   { echo "$RUN_DIR/$1/cks-mcp.pid"; }
portfile()  { echo "$RUN_DIR/$1/port"; }
advfile()   { echo "$RUN_DIR/$1/advertise"; }   # IP that `register` puts in the URL
cfgfile()   { echo "$RUN_DIR/$1/config.yaml"; }
logfile()   { echo "$RUN_DIR/$1/cks-mcp.log"; }
srvname()   { echo "${NAME_PREFIX}$1"; }

# is_loopback <host> -> 0 for 127.* / ::1 / localhost
is_loopback() {
  case "$1" in localhost|127.*|::1) return 0 ;; *) return 1 ;; esac
}

# detect_lan_ip -> primary routable IPv4 of this host (macOS), empty if none
detect_lan_ip() {
  local ifc ip
  for ifc in $(route -n get default 2>/dev/null | awk '/interface:/{print $2}') en0 en1 en2 en3; do
    ip="$(ipconfig getifaddr "$ifc" 2>/dev/null || true)"
    [[ -n "$ip" ]] && { echo "$ip"; return 0; }
  done
  return 1
}

valid_name() { [[ "$1" =~ ^[A-Za-z0-9_-]+$ ]] || die "invalid name '$1' (use [A-Za-z0-9_-])"; }

# alive <name> -> 0 if a process for the instance is running
alive() {
  local pf; pf="$(pidfile "$1")"
  [[ -f "$pf" ]] || return 1
  local pid; pid="$(cat "$pf" 2>/dev/null || true)"
  [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null
}

# port_open <port> [host] -> 0 if something is listening on host:port (default 127.0.0.1)
port_open() {
  local host="${2:-127.0.0.1}"
  (exec 3<>"/dev/tcp/$host/$1") 2>/dev/null && { exec 3>&- 3<&-; return 0; } || return 1
}

# port_used_by_instance <port> -> echoes instance name claiming the port (if any)
port_used_by_instance() {
  local p="$1" d n
  for d in "$RUN_DIR"/*/; do
    [[ -f "$d/port" ]] || continue
    n="$(basename "$d")"
    [[ "$(cat "$d/port")" == "$p" ]] && { echo "$n"; return 0; }
  done
  return 1
}

pick_port() {
  local p="$PORT_BASE"
  while (( p <= PORT_MAX )); do
    if ! port_used_by_instance "$p" >/dev/null 2>&1 && ! port_open "$p"; then
      echo "$p"; return 0
    fi
    p=$((p+1))
  done
  die "no free port in $PORT_BASE..$PORT_MAX"
}

# render_config <base_config> <bind_host> <port> <allow_remote:true|false> <cidrs_csv> > derived
# Strips the top-level `listen:` block from the base and appends an http one.
render_config() {
  local base="$1" bind="$2" port="$3" allow="$4" cidrs="$5"
  [[ -f "$base" ]] || die "base config not found: $base"
  awk '
    /^listen:[[:space:]]*$/ { skip=1; next }
    skip && /^[^[:space:]#]/  { skip=0 }
    !skip { print }
  ' "$base"
  echo "listen:"
  echo "  transport: \"http\""
  echo "  http_addr: \"$bind:$port\""
  echo "  allow_remote: $allow"
  if [[ -n "$cidrs" ]]; then
    echo "  allowed_cidrs:"
    local c; IFS=',' read -ra _cs <<< "$cidrs"
    for c in "${_cs[@]}"; do echo "    - \"$c\""; done
  fi
}

cmd_start() {
  local name="" base="$CKS_ROOT/cks-stablenet.yaml" port="" allow="" cidrs=""
  local bind="" advertise=""
  name="${1:-}"; [[ -n "$name" ]] || die "start: missing <name>"; shift || true
  valid_name "$name"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config) base="$2"; shift 2 ;;
      --port)   port="$2"; shift 2 ;;
      # --host <ip>: bind AND advertise on <ip> (the common "reach me on the LAN" case).
      --host)   bind="$2"; advertise="$2"; shift 2 ;;
      # --bind <host>: what to bind (e.g. 0.0.0.0 = all interfaces).
      --bind)   bind="$2"; shift 2 ;;
      # --advertise <ip>: IP that `register` puts in the URL (needed when binding 0.0.0.0).
      --advertise) advertise="$2"; shift 2 ;;
      --allow-remote) allow="true"; shift ;;
      --cidr)   cidrs="${cidrs:+$cidrs,}$2"; shift 2 ;;
      *) die "start: unknown arg '$1'" ;;
    esac
  done

  [[ -x "$CKS_MCP_BIN" ]] || die "cks-mcp binary not found/executable: $CKS_MCP_BIN (build it or set CKS_MCP_BIN)"
  # resolve base to absolute
  case "$base" in /*) : ;; *) base="$CKS_ROOT/$base" ;; esac

  # Bind/advertise/allow-remote resolution:
  #   - default bind = loopback (safe).
  #   - binding a non-loopback addr REQUIRES allow_remote (cks rejects otherwise),
  #     so auto-enable it unless explicitly set.
  [[ -n "$bind" ]] || bind="127.0.0.1"
  if ! is_loopback "$bind"; then
    [[ -n "$allow" ]] || allow="true"
  fi
  [[ -n "$allow" ]] || allow="false"
  # advertise IP for register: explicit > non-wildcard non-loopback bind > detected LAN IP > loopback
  if [[ -z "$advertise" ]]; then
    if [[ "$bind" != "0.0.0.0" && "$bind" != "::" ]] && ! is_loopback "$bind"; then
      advertise="$bind"
    elif [[ "$allow" == "true" ]]; then
      advertise="$(detect_lan_ip || true)"; [[ -n "$advertise" ]] || advertise="127.0.0.1"
    else
      advertise="127.0.0.1"
    fi
  fi
  if [[ "$allow" == "true" && -z "$cidrs" ]]; then
    info "NOTE: allow_remote=true with no --cidr → default LAN policy (loopback + private/RFC1918 ranges only)."
    info "      Public/non-LAN clients are still rejected; add --cidr <a.b.c.d/n> to permit them. cks has no per-client auth."
  fi

  if alive "$name"; then
    info "instance '$name' already running (pid $(cat "$(pidfile "$name")"), port $(cat "$(portfile "$name")"))"
    return 0
  fi

  [[ -n "$port" ]] || port="$(pick_port)"
  local claimed; if claimed="$(port_used_by_instance "$port")" && [[ "$claimed" != "$name" ]]; then
    die "port $port already claimed by instance '$claimed'"
  fi
  port_open "$port" && die "port $port already in use by another process"

  local d; d="$(inst_dir "$name")"; mkdir -p "$d"
  render_config "$base" "$bind" "$port" "$allow" "$cidrs" > "$(cfgfile "$name")"
  echo "$port" > "$(portfile "$name")"
  echo "$advertise" > "$(advfile "$name")"

  info "starting '$name' bind=$bind:$port advertise=$advertise allow_remote=$allow (config: $base)"
  nohup "$CKS_MCP_BIN" -config "$(cfgfile "$name")" >>"$(logfile "$name")" 2>&1 &
  echo "$!" > "$(pidfile "$name")"

  # Wait for the listener. cks-mcp logs "serving Streamable HTTP" BEFORE it
  # loads the ckg/ckv backends, so the port only binds once those are ready —
  # cold start on a real dataset can take tens of seconds. Override the window
  # with CKS_START_TIMEOUT (seconds).
  local timeout="${CKS_START_TIMEOUT:-45}" i probe="$advertise"
  is_loopback "$bind" && probe="127.0.0.1"
  for (( i=0; i*2 < timeout*10; i++ )); do
    alive "$name" || { info "process exited early — see $(logfile "$name")"; tail -n 20 "$(logfile "$name")" >&2 || true; return 1; }
    port_open "$port" "$probe" && { info "'$name' up: http://$advertise:$port/mcp  (register: $0 register $name)"; return 0; }
    sleep 0.2
  done
  info "'$name' started (pid $(cat "$(pidfile "$name")")) but port $port not accepting after ${timeout}s — still loading backends? check $(logfile "$name")"
}

cmd_stop() {
  local target="${1:-}"; [[ -n "$target" ]] || die "stop: missing <name> or --all"
  if [[ "$target" == "--all" ]]; then
    local d n; for d in "$RUN_DIR"/*/; do [[ -d "$d" ]] || continue; n="$(basename "$d")"; cmd_stop "$n"; done
    return 0
  fi
  valid_name "$target"
  if ! alive "$target"; then info "'$target' not running"; rm -f "$(pidfile "$target")"; return 0; fi
  local pid; pid="$(cat "$(pidfile "$target")")"
  info "stopping '$target' (pid $pid)"
  kill "$pid" 2>/dev/null || true
  local i; for i in $(seq 1 25); do kill -0 "$pid" 2>/dev/null || break; sleep 0.2; done
  kill -0 "$pid" 2>/dev/null && { info "force-killing $pid"; kill -9 "$pid" 2>/dev/null || true; }
  rm -f "$(pidfile "$target")"
}

cmd_restart() { local n="${1:-}"; [[ -n "$n" ]] || die "restart: missing <name>"; cmd_stop "$n" || true; cmd_start "$n"; }

cmd_list() {
  printf "%-16s %-7s %-8s %-6s %s\n" NAME PORT PID ALIVE SERVER
  [[ -d "$RUN_DIR" ]] || return 0
  local d n port pid st
  for d in "$RUN_DIR"/*/; do
    [[ -d "$d" ]] || continue
    n="$(basename "$d")"
    port="$(cat "$d/port" 2>/dev/null || echo '-')"
    pid="$(cat "$d/cks-mcp.pid" 2>/dev/null || echo '-')"
    if alive "$n"; then st="yes"; else st="no"; fi
    printf "%-16s %-7s %-8s %-6s %s\n" "$n" "$port" "$pid" "$st" "$(srvname "$n")"
  done
}

cmd_status() {
  local n="${1:-}"; [[ -n "$n" ]] || die "status: missing <name>"; valid_name "$n"
  [[ -d "$(inst_dir "$n")" ]] || die "no such instance: $n"
  local port host; port="$(cat "$(portfile "$n")" 2>/dev/null || echo '-')"
  host="$(cat "$(advfile "$n")" 2>/dev/null || echo 127.0.0.1)"
  echo "name:    $n"
  echo "server:  $(srvname "$n")"
  echo "port:    $port"
  echo "url:     http://$host:$port/mcp"
  echo "pid:     $(cat "$(pidfile "$n")" 2>/dev/null || echo '-')"
  echo "alive:   $(alive "$n" && echo yes || echo no)"
  echo "config:  $(cfgfile "$n")"
  echo "log:     $(logfile "$n")"
}

cmd_logs() {
  local n="${1:-}"; [[ -n "$n" ]] || die "logs: missing <name>"; shift || true
  valid_name "$n"
  local lf; lf="$(logfile "$n")"; [[ -f "$lf" ]] || die "no log for '$n'"
  if [[ "${1:-}" == "-f" ]]; then tail -f "$lf"; else tail -n 40 "$lf"; fi
}

cmd_register() {
  local n="${1:-}"; [[ -n "$n" ]] || die "register: missing <name>"; shift || true
  valid_name "$n"
  command -v claude >/dev/null 2>&1 || die "'claude' CLI not on PATH"
  local port scope="user" host
  port="$(cat "$(portfile "$n")" 2>/dev/null || true)"; [[ -n "$port" ]] || die "instance '$n' has no port (start it first)"
  host="$(cat "$(advfile "$n")" 2>/dev/null || echo 127.0.0.1)"
  while [[ $# -gt 0 ]]; do case "$1" in --scope) scope="$2"; shift 2 ;; --host) host="$2"; shift 2 ;; *) die "register: unknown arg '$1'" ;; esac; done
  local sv; sv="$(srvname "$n")"
  info "registering '$sv' -> http://$host:$port/mcp (scope=$scope)"
  claude mcp remove "$sv" -s "$scope" >/dev/null 2>&1 || true
  claude mcp add --transport http "$sv" "http://$host:$port/mcp" -s "$scope"
  info "done. Restart the Claude Code session to pick it up; verify with /mcp."
}

cmd_unregister() {
  local n="${1:-}"; [[ -n "$n" ]] || die "unregister: missing <name>"; shift || true
  valid_name "$n"
  command -v claude >/dev/null 2>&1 || die "'claude' CLI not on PATH"
  local scope="user"
  while [[ $# -gt 0 ]]; do case "$1" in --scope) scope="$2"; shift 2 ;; *) die "unknown arg '$1'" ;; esac; done
  local sv; sv="$(srvname "$n")"
  info "removing '$sv' (scope=$scope)"
  claude mcp remove "$sv" -s "$scope"
}

usage() { sed -n '2,40p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

main() {
  local cmd="${1:-}"; shift || true
  case "$cmd" in
    start)      cmd_start "$@" ;;
    stop)       cmd_stop "$@" ;;
    restart)    cmd_restart "$@" ;;
    list|ls)    cmd_list "$@" ;;
    status)     cmd_status "$@" ;;
    logs)       cmd_logs "$@" ;;
    register)   cmd_register "$@" ;;
    unregister) cmd_unregister "$@" ;;
    ""|-h|--help|help) usage ;;
    *) die "unknown command '$cmd' (try --help)" ;;
  esac
}

main "$@"
