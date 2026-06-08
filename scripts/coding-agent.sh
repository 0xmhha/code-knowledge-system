#!/usr/bin/env bash
# coding-agent.sh — launch Claude Code for the coding-agent pipeline with
# autonomous (no-prompt) execution guaranteed.
#
#   1. ensure go-stablenet/.claude/settings.local.json has bypassPermissions
#      (enable-autopilot.sh — idempotent: pass if already set)
#   2. cd into go-stablenet so the project-local settings load at session start
#   3. exec `claude` (passes through any args, e.g. a slash command)
#
# Usage:
#   ./scripts/coding-agent.sh                       # open Claude Code in go-stablenet (autonomous)
#   ./scripts/coding-agent.sh /coding-agent:work STABLE-1234
#
# Why a launcher (not a hook): the permission mode is fixed at session start,
# so the settings must exist BEFORE claude launches. A SubagentStart/hook would
# only affect the NEXT session and cannot escalate permissions anyway.
set -euo pipefail

abs() { ( cd "$1" 2>/dev/null && pwd ); }
HERE="$(abs "$(dirname "${BASH_SOURCE[0]}")")"
CKS_ROOT="$(abs "$HERE/..")"
GSN="$(abs "${GO_STABLENET_ROOT:-$CKS_ROOT/../go-stablenet}")"
[ -n "$GSN" ] && [ -d "$GSN" ] || { echo "ERROR: go-stablenet not found (set GO_STABLENET_ROOT)" >&2; exit 1; }

"$HERE/enable-autopilot.sh"
command -v claude >/dev/null 2>&1 || { echo "ERROR: 'claude' not on PATH" >&2; exit 1; }
cd "$GSN"
echo "coding-agent: launching Claude Code in $GSN (autonomous)"
exec claude "$@"
