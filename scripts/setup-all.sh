#!/usr/bin/env bash
# setup-all.sh — one-click setup of the go-stablenet code-knowledge stack on a
# fresh machine: prereqs → Ollama+bge-m3 → all binaries → dataset → config/env →
# jira-gateway → activation instructions.
#
# Assumes the six repos are cloned as SIBLINGS under one parent:
#   <parent>/code-knowledge-system   (this repo — run the script from here)
#   <parent>/code-knowledge-vector
#   <parent>/code-knowledge-graph
#   <parent>/go-stablenet
#   <parent>/coding-agent
#   <parent>/chainbench              (optional)
#
# Usage:
#   ./scripts/setup-all.sh                 # full (includes the long ckv embed)
#   SKIP_CKV=1 ./scripts/setup-all.sh      # everything except the multi-hour ckv embed
#   SKIP_OLLAMA=1 ./scripts/setup-all.sh   # assume Ollama+bge-m3 already present
#
# Idempotent: re-running rebuilds/refreshes each artifact. Safe to interrupt and resume.
set -euo pipefail

CKS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$CKS_ROOT"
PARENT="$(cd "$CKS_ROOT/.." && pwd)"

CKG_REPO="${CKG_REPO:-$PARENT/code-knowledge-graph}"
CKV_REPO="${CKV_REPO:-$PARENT/code-knowledge-vector}"
CODING_AGENT="${CODING_AGENT:-$PARENT/coding-agent}"
GO_STABLENET_ROOT="${GO_STABLENET_ROOT:-$PARENT/go-stablenet}"
EMBED_MODEL="${EMBED_MODEL:-bge-m3}"
OLLAMA_URL="${CKV_OLLAMA_ENDPOINT:-http://localhost:11434}"
SKIP_CKV="${SKIP_CKV:-0}"
SKIP_OLLAMA="${SKIP_OLLAMA:-0}"

log()  { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[1;33m! %s\033[0m\n' "$*"; }
die()  { printf '\033[1;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

log "0. prerequisites"
command -v go    >/dev/null || die "Go not found (need 1.23+). https://go.dev/dl/"
command -v cc    >/dev/null || warn "C toolchain (cc) not found — cks links sqlite-vec (CGO). Install Xcode CLT."
command -v git   >/dev/null || die "git not found"
for d in "$CKG_REPO" "$CKV_REPO" "$GO_STABLENET_ROOT"; do [ -d "$d" ] || die "missing sibling repo: $d"; done
echo "go: $(go version)"

# ── 1. Ollama (app cask, NOT brew formula) + bge-m3 ─────────────────────────
if [ "$SKIP_OLLAMA" != "1" ]; then
  log "1. Ollama + $EMBED_MODEL"
  if ! command -v ollama >/dev/null 2>&1; then
    command -v brew >/dev/null || die "Homebrew required to install Ollama (or set SKIP_OLLAMA=1 and install manually)"
    warn "Installing Ollama via APP CASK (the brew *formula* lacks llama-server on Apple Silicon)"
    brew install --cask ollama-app
  fi
  curl -fsS "$OLLAMA_URL/api/version" >/dev/null 2>&1 || { warn "starting 'ollama serve' in background"; nohup ollama serve >/tmp/ollama-serve.log 2>&1 & sleep 5; }
  ollama list 2>/dev/null | grep -q "$EMBED_MODEL" || ollama pull "$EMBED_MODEL"
  # sanity: bge-m3 must return 1024-dim
  dim=$(curl -fsS "$OLLAMA_URL/api/embed" -d "{\"model\":\"$EMBED_MODEL\",\"input\":\"x\"}" 2>/dev/null \
        | python3 -c 'import sys,json;e=json.load(sys.stdin).get("embeddings",[[]]);print(len(e[0]) if e and e[0] else 0)' 2>/dev/null || echo 0)
  [ "$dim" = "1024" ] && echo "ollama $EMBED_MODEL OK (dim=1024)" || warn "embedding sanity failed (dim=$dim) — check 'ollama serve'"
else
  log "1. Ollama — SKIPPED (SKIP_OLLAMA=1)"
fi

# ── 2-4. binaries + dataset + config/env (delegates to build-stablenet-dataset.sh) ──
log "2-4. binaries + dataset + config (build-stablenet-dataset.sh)"
GO_STABLENET_ROOT="$GO_STABLENET_ROOT" CKG_REPO="$CKG_REPO" CKV_REPO="$CKV_REPO" \
  EMBED_MODEL="$EMBED_MODEL" CKV_OLLAMA_ENDPOINT="$OLLAMA_URL" SKIP_CKV="$SKIP_CKV" \
  "$CKS_ROOT/scripts/build-stablenet-dataset.sh"

# ── 5. jira-gateway (coding-agent in-tree MCP) ──────────────────────────────
if [ -d "$CODING_AGENT/tools/jira-gateway-mcp" ]; then
  log "5. jira-gateway build"
  ( cd "$CODING_AGENT/tools/jira-gateway-mcp" && go build -o bin/jira-gateway-mcp ./cmd/server )
  echo "built: $CODING_AGENT/tools/jira-gateway-mcp/bin/jira-gateway-mcp"
else
  warn "5. jira-gateway — coding-agent repo not found at $CODING_AGENT (skipping)"
fi

# ── 6. jira.env scaffold (secret, outside any repo) ─────────────────────────
JENV="$HOME/.config/coding-agent/jira.env"
if [ ! -f "$JENV" ]; then
  log "6. jira.env scaffold"
  mkdir -p "$(dirname "$JENV")"; ( umask 077; cat > "$JENV" <<'EOF'
# jira-gateway credentials (SECRET — never commit). chmod 600.
# JIRA_BASE_URL: your Atlassian site. JIRA_API_TOKEN: id.atlassian.com/manage-profile/security/api-tokens
export JIRA_BASE_URL="https://CHANGE-ME.atlassian.net"
export JIRA_USER_EMAIL="CHANGE-ME@example.com"
export JIRA_API_TOKEN="CHANGE-ME"
EOF
)
  chmod 600 "$JENV"; echo "created $JENV (fill in before using jira_* tools)"
else
  echo "6. jira.env exists ($JENV) — left as-is"
fi

# ── 7. Claude Code settings.json env (launch-method-independent) ────────────
log "7. apply env to ~/.claude/settings.json (so MCP works from GUI or terminal)"
"$CKS_ROOT/scripts/apply-cc-settings.sh" || warn "apply-cc-settings failed (run it manually)"

# ── 8. autonomous (no-prompt) execution for the go-stablenet project ────────
log "8. enable autopilot (bypassPermissions) for go-stablenet"
GO_STABLENET_ROOT="$GO_STABLENET_ROOT" "$CKS_ROOT/scripts/enable-autopilot.sh" \
  || warn "enable-autopilot failed (run it manually)"

# ── done ────────────────────────────────────────────────────────────────────
log "SETUP COMPLETE — next steps"
cat <<EOF
  1) Install the plugin (in Claude Code):
       /plugin marketplace add 0xmhha/coding-agent
       /plugin install coding-agent@coding-agent
  2) Launch the autonomous pipeline via the launcher (ensures bypassPermissions
     is in place, then opens Claude Code in go-stablenet):
       "$CKS_ROOT/scripts/coding-agent.sh"                      # interactive
       "$CKS_ROOT/scripts/coding-agent.sh" /coding-agent:work STABLE-1234
     (MCP env is already global via ~/.claude/settings.json — no per-launch source.)
  3) Verify cks health any time:
       "$CKS_ROOT/scripts/cks-health.sh"        # expect status: ok
  4) (optional) Jira token: edit $JENV, then re-run scripts/apply-cc-settings.sh
EOF
