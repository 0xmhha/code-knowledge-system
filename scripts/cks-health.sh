#!/usr/bin/env bash
# cks-health.sh — drive the cks.ops.health MCP tool over stdio and print the result.
#
# Usage:  GO_STABLENET_ROOT=/path/to/go-stablenet ./scripts/cks-health.sh [config.yaml]
#
# Sends the MCP initialize handshake + a tools/call for cks.ops.health to a
# transient cks-mcp process, captures stdout to a temp file (avoids SIGPIPE on
# early parser exit), then extracts the structured health payload.
set -uo pipefail
CKS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$CKS_ROOT"
CONFIG="${1:-./cks-stablenet.yaml}"
OUT="$(mktemp)"; trap 'rm -f "$OUT"' EXIT

{
  printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"cks-health","version":"1"}}}\n'
  printf '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}\n'
  printf '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"cks.ops.health","arguments":{}}}\n'
  sleep 4   # let the server probe backends (ckv hits Ollama) + flush before stdin closes
} | ./bin/cks-mcp -config "$CONFIG" >"$OUT" 2>/dev/null || true

python3 -c '
import sys, json
payload=None
for line in open(sys.argv[1]):
    line=line.strip()
    if not line: continue
    try: m=json.loads(line)
    except Exception: continue
    if m.get("id")==2:
        res=m.get("result",{})
        payload=res.get("structuredContent")
        if payload is None:
            for c in res.get("content",[]):
                if c.get("type")=="text":
                    try: payload=json.loads(c["text"])
                    except Exception: payload=c["text"]
if payload is None:
    print("ERROR: no cks.ops.health response found"); sys.exit(2)
print(json.dumps(payload, indent=2, ensure_ascii=False))
status=payload.get("status") if isinstance(payload,dict) else None
sys.exit(0 if status=="ok" else 1)
' "$OUT"
