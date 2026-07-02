#!/usr/bin/env bash
#
# serve-cks-http.sh — start/stop cks-mcp HTTP instances ON DEMAND.
#
# This is NOT a boot service. An instance runs only between `start` and `stop`;
# it does not auto-start at login/reboot and does not respawn on its own.
#
# Several instances can run side by side, each with its own NAME and PORT
# (one per dataset/index). A caller connects by ip:port and can confirm which
# instance it reached via cks.ops.health (name + description + indexed_head).
#
#   serve-cks-http.sh start   [name] [http_addr] [config]
#   serve-cks-http.sh stop     [name]
#   serve-cks-http.sh restart [name] [http_addr] [config]
#   serve-cks-http.sh status  [name]      # (no name → list all managed instances)
#   serve-cks-http.sh logs    [name] [N]
#
# Defaults (also overridable by env):
#   name       = cks-stablenet                         (or $CKS_NAME)
#   http_addr  = (from config's listen.http_addr)      (or $CKS_HTTP_ADDR)
#   config     = <repo>/cks-stablenet.yaml             (or $CKS_HTTP_CONFIG)
#   CKS_MCP_BIN          = <repo>/bin/cks-mcp
#   CKV_OLLAMA_ENDPOINT  = http://localhost:11434
#   CKS_RUNDIR           = <repo>/run
#
# Per-instance pidfile/log are keyed by name: run/cks-<name>.{pid,log}.
# NOTE: uses CKS_HTTP_CONFIG, NOT the ambient CKS_CONFIG (which targets a
# different consumer); the served config is decided here so `start` is
# deterministic regardless of the surrounding shell env.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${CKS_MCP_BIN:-$ROOT/bin/cks-mcp}"
RUNDIR="${CKS_RUNDIR:-$ROOT/run}"
OLLAMA="${CKV_OLLAMA_ENDPOINT:-http://localhost:11434}"
mkdir -p "$RUNDIR"

cmd="${1:-}"
name="${2:-${CKS_NAME:-cks-stablenet}}"
PIDFILE="$RUNDIR/$name.pid"
LOGFILE="$RUNDIR/$name.log"

is_running() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; }

do_start() {
  local addr="${3:-${CKS_HTTP_ADDR:-}}"
  local config="${4:-${CKS_HTTP_CONFIG:-$ROOT/cks-stablenet.yaml}}"
  if is_running; then echo "[$name] already running (pid $(cat "$PIDFILE"))"; return 0; fi
  [ -x "$BIN" ]    || { echo "error: binary not executable: $BIN (run 'make build-bins')"; exit 1; }
  [ -f "$config" ] || { echo "error: config not found: $config"; exit 1; }
  local args=(--config "$config" --name "$name")
  [ -n "$addr" ] && args+=(--http-addr "$addr")
  CKV_OLLAMA_ENDPOINT="$OLLAMA" CKS_OLLAMA_ENDPOINT="$OLLAMA" \
    nohup "$BIN" "${args[@]}" >>"$LOGFILE" 2>&1 &
  echo $! > "$PIDFILE"
  sleep 1
  if is_running; then
    echo "[$name] started (pid $(cat "$PIDFILE"))  config=$config${addr:+  addr=$addr}"
    echo "log: $LOGFILE"
  else
    echo "[$name] failed to start — last log lines:"; tail -n 8 "$LOGFILE" 2>/dev/null || true
    rm -f "$PIDFILE"; exit 1
  fi
}

do_stop() {
  if is_running; then
    local pid; pid="$(cat "$PIDFILE")"
    kill "$pid" 2>/dev/null || true
    for _ in 1 2 3 4 5; do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
    kill -9 "$pid" 2>/dev/null || true
    rm -f "$PIDFILE"; echo "[$name] stopped (pid $pid)"
  else
    rm -f "$PIDFILE"; echo "[$name] not running"
  fi
}

case "$cmd" in
  start)   do_start "$@" ;;
  stop)    do_stop ;;
  restart) do_stop; sleep 1; do_start "$@" ;;
  status)
    if [ -n "${2:-}" ]; then
      is_running && echo "[$name] running (pid $(cat "$PIDFILE"))" || echo "[$name] stopped"
    else
      shopt -s nullglob
      found=0
      for pf in "$RUNDIR"/*.pid; do
        found=1; n="$(basename "$pf" .pid)"
        if kill -0 "$(cat "$pf" 2>/dev/null)" 2>/dev/null; then echo "[$n] running (pid $(cat "$pf"))"; else echo "[$n] stale pidfile"; fi
      done
      [ "$found" -eq 0 ] && echo "(no managed instances)"
    fi
    ;;
  logs)    tail -n "${3:-40}" -f "$LOGFILE" ;;
  *)
    echo "usage: $0 {start|stop|restart|status|logs} [name] [http_addr] [config]"
    exit 2
    ;;
esac
