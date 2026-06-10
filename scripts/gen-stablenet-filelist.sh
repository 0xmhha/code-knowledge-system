#!/usr/bin/env bash
# gen-stablenet-filelist.sh
# ---------------------------------------------------------------------------
# Given only the go-stablenet project path, produce the file list that should
# drive index creation:
#   = Go files in the `gstable` build dependency closure
#   + Solidity contract sources and libraries embedded into the build
#   - consensus engines that are registered but never used (clique, ethash)
#
# Output:
#   <out>/build_files.txt   one repo-relative file per line
#   <out>/files-from.json   {"include":[...]} consumable by ckg/ckv --files-from
#
# Usage:
#   ./scripts/gen-stablenet-filelist.sh [GO_STABLENET_ROOT] [OUT_DIR]
# Optional env:
#   ENTRY=./cmd/gstable               build entry point (closure root)
#   EXCLUDE_PKGS="consensus/clique consensus/ethash"   package prefixes to drop
#   INCLUDE_TESTS=1                   include _test.go of in-scope packages
#                                     (for usage-oriented queries; default 1)
# ---------------------------------------------------------------------------
set -euo pipefail

ROOT="${1:-${GO_STABLENET_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../go-stablenet" 2>/dev/null && pwd)}}"
OUT="${2:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/generated/stablenet-filelist}"
ENTRY="${ENTRY:-./cmd/gstable}"
EXCLUDE_PKGS="${EXCLUDE_PKGS:-consensus/clique consensus/ethash}"
INCLUDE_TESTS="${INCLUDE_TESTS:-1}"

[ -d "$ROOT" ] || { echo "ERROR: go-stablenet root not found: '$ROOT'" >&2; exit 1; }
command -v go >/dev/null || { echo "ERROR: go toolchain required" >&2; exit 1; }
mkdir -p "$OUT"
ROOT="$(cd "$ROOT" && pwd)"

echo "==> go-stablenet: $ROOT"
echo "==> entry: $ENTRY  exclude_pkgs: [$EXCLUDE_PKGS]  include_tests: $INCLUDE_TESTS"

# ── 1) Go: source files in the gstable build closure ────────────────────────
# Resolve the dependency closure of the entry point with `go list -deps`, then
# expand each package's .go (and _test.go) files to repo-relative paths.
# Excluded package prefixes are filtered out.
go_tmpl='{{$d:=.Dir}}{{range .GoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}'
[ "$INCLUDE_TESTS" = "1" ] && go_tmpl="$go_tmpl"'{{range .TestGoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}'

( cd "$ROOT" && go list -deps -f "$go_tmpl" "$ENTRY" ) 2>/dev/null \
  | sed "s#^$ROOT/##" \
  | grep -E '^[a-z]' \
  | { ex=""; for p in $EXCLUDE_PKGS; do ex="$ex -e ^$p/"; done; [ -n "$ex" ] && grep -vE $ex || cat; } \
  | sort -u > "$OUT/go_files.txt"

# ── 2) Solidity: contract sources (and libraries) embedded into the build ────
# The build embeds only the compiled bytecode artifacts (//go:embed
# artifacts/vN/<Name>), so the .sol sources must be added explicitly or they
# would be missed. Recover the used contracts from the embed directives and
# include the matching solidity/vN/<Name>.sol plus the shared libraries.
: > "$OUT/sol_files.txt"
CONTRACTS_GO="$ROOT/systemcontracts/contracts.go"
if [ -f "$CONTRACTS_GO" ]; then
  grep -oE 'artifacts/v[0-9]+/[A-Za-z0-9_]+' "$CONTRACTS_GO" | sort -u | while read -r art; do
    rel="systemcontracts/solidity/${art#artifacts/}.sol"   # artifacts/v1/Gov -> solidity/v1/Gov.sol
    [ -f "$ROOT/$rel" ] && echo "$rel"
  done >> "$OUT/sol_files.txt"
fi
# Shared libraries imported by the contracts (test mocks are excluded).
if [ -d "$ROOT/systemcontracts/solidity/libraries" ]; then
  ( cd "$ROOT" && find systemcontracts/solidity/libraries -name '*.sol' ) >> "$OUT/sol_files.txt"
fi
sort -u -o "$OUT/sol_files.txt" "$OUT/sol_files.txt"

# ── 3) Combine and emit files-from.json ─────────────────────────────────────
cat "$OUT/go_files.txt" "$OUT/sol_files.txt" | sort -u > "$OUT/build_files.txt"

python3 - "$OUT/build_files.txt" "$OUT/files-from.json" <<'PY'
import json, sys
files=[l.strip() for l in open(sys.argv[1]) if l.strip()]
json.dump({"include": files, "exclude": []}, open(sys.argv[2],"w"), ensure_ascii=False, indent=0)
print(f"files-from.json: {len(files)} include entries")
PY

ngo=$(wc -l < "$OUT/go_files.txt" | tr -d ' ')
nsol=$(wc -l < "$OUT/sol_files.txt" | tr -d ' ')
ntest=$(grep -c '_test.go' "$OUT/go_files.txt" || true)
echo "==> result:"
echo "    Go files      : $ngo (of which _test.go: $ntest)"
echo "    Solidity files: $nsol"
echo "    total         : $(( ngo + nsol ))"
echo "    clique/ethash remaining: $(grep -cE 'consensus/(clique|ethash)/' "$OUT/build_files.txt" || true)"
echo "    output: $OUT/{build_files.txt, files-from.json}"
