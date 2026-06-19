#!/usr/bin/env bash
# run-coding-agent.sh — launch Claude Code with the coding-agent plugin's cks MCP
# server pointed at a specific dataset (NOT the global default).
#
# Why a launcher: Claude Code interpolates ${CKS_CONFIG} in the plugin's
# .mcp.json from the SHELL ENVIRONMENT, and shell env WINS over settings.json. So
# to switch datasets reliably we export the dataset's paths in the shell *before*
# launching, AFTER any activate.sh (so our override is last-write-wins).
#
# Generalized from knowledge-data/pr-14/run-coding-agent-pr14.sh.
#
# Usage:
#   CODE=/abs/path/to/source/repo ENV_FILE=/abs/dataset/cks-<name>.env \
#     ./run-coding-agent.sh
#   ... ./run-coding-agent.sh /coding-agent:analyze "..."   # pass a slash command through
#
# Optional:
#   CKS_ROOT   code-knowledge-system checkout (for activate.sh); default: repo layout
set -euo pipefail

abs() { ( cd "$1" 2>/dev/null && pwd ); }
HERE="$(abs "$(dirname "${BASH_SOURCE[0]}")")"

ENV_FILE="${ENV_FILE:?set ENV_FILE=/abs/path/to/dataset/cks-<name>.env}"
CODE="${CODE:?set CODE=/abs/path/to/source/repo (the git repo Claude edits)}"
[ -f "$ENV_FILE" ] || { echo "ERROR: ENV_FILE not found: $ENV_FILE" >&2; exit 1; }
CODE="$(abs "$CODE")"; [ -n "$CODE" ] && [ -d "$CODE" ] || { echo "ERROR: CODE dir not found" >&2; exit 1; }

CKS_ROOT="$(abs "${CKS_ROOT:-$HERE/../..}")"

# 1. Base env (jira-gateway + chainbench bins, etc.) — best-effort, non-fatal.
#    This may also set CKS_* to a default dataset, which we override in step 2.
if [ -n "$CKS_ROOT" ] && [ -f "$CKS_ROOT/activate.sh" ]; then
  # shellcheck disable=SC1091
  source "$CKS_ROOT/activate.sh" || true
fi

# 2. Override cks → target dataset (sourced LAST so these shell exports win).
# shellcheck disable=SC1091
source "$ENV_FILE"
echo "coding-agent: CKS_CONFIG=$CKS_CONFIG"

command -v claude >/dev/null 2>&1 || { echo "ERROR: 'claude' not on PATH" >&2; exit 1; }
cd "$CODE"
echo "coding-agent: launching Claude Code in $CODE"
exec claude "$@"
