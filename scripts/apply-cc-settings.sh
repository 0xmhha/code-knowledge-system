#!/usr/bin/env bash
# apply-cc-settings.sh — write the cks/jira/chainbench env into Claude Code's
# user settings (~/.claude/settings.json "env" block) so the coding-agent
# plugin's MCP servers resolve their ${VAR} placeholders REGARDLESS of how
# Claude Code is launched (GUI/Dock or terminal). No ~/.zshrc, no per-launch
# `source activate.sh`.
#
# Source of truth: cks.env (paths, generated) + ~/.config/coding-agent/jira.env
# (secrets). This script resolves them via activate.sh, then MERGES the env keys
# into settings.json without touching other settings. Re-run after editing
# jira.env or moving repos.
#
# Usage:  ./scripts/apply-cc-settings.sh
set -euo pipefail
CKS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Resolve all env (CKS_*, JIRA_*, CHAINBENCH_DIR) via the on-demand loader.
# shellcheck disable=SC1090
source "$CKS_ROOT/activate.sh" >/dev/null 2>&1 || true

SETTINGS="$HOME/.claude/settings.json"
mkdir -p "$HOME/.claude"

CKS_MCP_BIN="${CKS_MCP_BIN:-}" CKS_MCP_URL="${CKS_MCP_URL:-}" \
JIRA_GATEWAY_BIN="${JIRA_GATEWAY_BIN:-}" JIRA_BASE_URL="${JIRA_BASE_URL:-}" \
JIRA_USER_EMAIL="${JIRA_USER_EMAIL:-}" JIRA_API_TOKEN="${JIRA_API_TOKEN:-}" \
CHAINBENCH_DIR="${CHAINBENCH_DIR:-}" SETTINGS="$SETTINGS" \
python3 - <<'PY'
import json, os, sys

path = os.environ["SETTINGS"]
# CKS_CONFIG is gone: the plugin's cks entry is HTTP-only (${CKS_MCP_URL}) and
# the bench connects over HTTP too, so nothing resolves CKS_CONFIG anymore.
keys = ["CKS_MCP_BIN", "CKS_MCP_URL", "JIRA_GATEWAY_BIN", "JIRA_BASE_URL",
        "JIRA_USER_EMAIL", "JIRA_API_TOKEN", "CHAINBENCH_DIR"]
# Drop the stale key from existing settings on re-run.
stale = ["CKS_CONFIG"]

try:
    with open(path) as f:
        data = json.load(f)
except FileNotFoundError:
    data = {}
except json.JSONDecodeError as e:
    sys.exit(f"ERROR: {path} is not valid JSON ({e}); fix it first.")

env = data.get("env", {})
if not isinstance(env, dict):
    sys.exit('ERROR: existing "env" is not an object; refusing to overwrite.')

written = []
for k in keys:
    v = os.environ.get(k, "")
    if v:
        env[k] = v
        written.append(k)
for k in stale:
    if env.pop(k, None) is not None:
        print(f"  removed stale env.{k}")
data["env"] = env

with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")

print(f"merged into {path} (other settings preserved)")
for k in written:
    shown = env[k]
    if k == "JIRA_API_TOKEN" and "CHANGE-ME" not in shown:
        shown = shown[:4] + "…(redacted)"
    print(f"  env.{k} = {shown}")
miss = [k for k in keys if not os.environ.get(k)]
if miss:
    print("  (unset, skipped):", ", ".join(miss))
PY

echo ""
echo "Done. Restart Claude Code (any launch method) — /doctor should be clean."
echo "Note: JIRA_API_TOKEN is still a placeholder until you edit ~/.config/coding-agent/jira.env and re-run this."
