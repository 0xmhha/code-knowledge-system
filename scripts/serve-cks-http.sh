#!/usr/bin/env bash
#
# serve-cks-http.sh — start/stop the standalone cks-mcp HTTP server ON DEMAND.
#
# This is NOT a boot service. The server runs only between `start` and `stop`;
# it does not auto-start at login/reboot and does not respawn on its own. You
# control it explicitly:
#
#   scripts/serve-cks-http.sh start      # launch (detached) and write a pidfile
#   scripts/serve-cks-http.sh stop       # terminate the running server
#   scripts/serve-cks-http.sh restart    # stop then start (e.g. after `make build-bins`)
#   scripts/serve-cks-http.sh status     # running? which pid / config / addr
#   scripts/serve-cks-http.sh logs [N]   # tail -f the last N (default 40) log lines
#
# Override via env (defaults shown):
#   CKS_MCP_BIN          = <repo>/bin/cks-mcp
#   CKS_HTTP_CONFIG      = <repo>/cks-stablenet.yaml   (must be transport: http)
#   CKV_OLLAMA_ENDPOINT  = http://localhost:11434
#   CKS_RUNDIR           = <repo>/run                  (pidfile + log live here)
#
# NOTE: this uses CKS_HTTP_CONFIG, NOT the ambient CKS_CONFIG. CKS_CONFIG is
# injected globally (settings.json) for the plugin/launcher context and may
# point elsewhere; the HTTP server config is decided here so `start` is
# deterministic regardless of the surrounding shell env.
#
# The HTTP bind address + allow_remote come from the config's `listen:` block,
# so multi-machine access is governed there (bind a LAN IP / 0.0.0.0, allow_remote: true).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${CKS_MCP_BIN:-$ROOT/bin/cks-mcp}"
CONFIG="${CKS_HTTP_CONFIG:-$ROOT/cks-stablenet.yaml}"
RUNDIR="${CKS_RUNDIR:-$ROOT/run}"
PIDFILE="$RUNDIR/cks-mcp.pid"
LOGFILE="$RUNDIR/cks-mcp.log"
OLLAMA="${CKV_OLLAMA_ENDPOINT:-http://localhost:11434}"

mkdir -p "$RUNDIR"

is_running() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; }

addr_from_config() {
  # best-effort: echo the http_addr for status output
  grep -E '^[[:space:]]*http_addr:' "$CONFIG" 2>/dev/null | head -1 | sed -E 's/.*http_addr:[[:space:]]*"?([^"#]+)"?.*/\1/' | tr -d ' '
}

cmd="${1:-}"
case "$cmd" in
  start)
    if is_running; then
      echo "already running (pid $(cat "$PIDFILE"))"
      exit 0
    fi
    [ -x "$BIN" ]   || { echo "error: binary not executable: $BIN (run 'make build-bins')"; exit 1; }
    [ -f "$CONFIG" ] || { echo "error: config not found: $CONFIG"; exit 1; }
    CKV_OLLAMA_ENDPOINT="$OLLAMA" CKS_OLLAMA_ENDPOINT="$OLLAMA" \
      nohup "$BIN" --config "$CONFIG" >>"$LOGFILE" 2>&1 &
    echo $! > "$PIDFILE"
    sleep 1
    if is_running; then
      echo "started (pid $(cat "$PIDFILE"))  addr=$(addr_from_config)  config=$CONFIG"
      echo "log: $LOGFILE"
    else
      echo "error: failed to start — last log lines:"
      tail -n 8 "$LOGFILE" 2>/dev/null || true
      rm -f "$PIDFILE"
      exit 1
    fi
    ;;
  stop)
    if is_running; then
      pid="$(cat "$PIDFILE")"
      kill "$pid" 2>/dev/null || true
      for _ in 1 2 3 4 5; do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
      kill -9 "$pid" 2>/dev/null || true
      rm -f "$PIDFILE"
      echo "stopped (pid $pid)"
    else
      rm -f "$PIDFILE"
      echo "not running"
    fi
    ;;
  restart)
    "$0" stop || true
    sleep 1
    "$0" start
    ;;
  status)
    if is_running; then
      echo "running (pid $(cat "$PIDFILE"))  addr=$(addr_from_config)  config=$CONFIG"
    else
      echo "stopped"
    fi
    ;;
  logs)
    n="${2:-40}"
    tail -n "$n" -f "$LOGFILE"
    ;;
  *)
    echo "usage: $0 {start|stop|restart|status|logs [N]}"
    echo "  config: $CONFIG"
    echo "  binary: $BIN"
    exit 2
    ;;
esac
