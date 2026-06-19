#!/usr/bin/env bash
# build-pruned-src.sh — produce a pruned copy of a Go source tree containing ONLY
# the code that participates in a given build target (the dependency closure of
# BUILD_TARGETS, restricted to MODULE). Indexing this pruned tree instead of the
# whole repo keeps ckg/ckv focused on code that actually ships, dropping
# unrelated tools/examples and shrinking the graph + vector index.
#
# Generalized from knowledge-data/pr-14/build-pruned-src.sh (was hardcoded to
# go-ethereum / ./cmd/gstable). Drive it entirely from env vars / flags.
#
# Excludes _test.go files and non-package dirs (testdata, etc.) so the tree is
# only build-participating code. Keeps //go:embed assets via copy-embeds.sh
# (run that next).
#
# Usage:
#   SRC=/path/to/repo OUT=/path/to/dataset/_src MODULE=github.com/acme/app \
#     BUILD_TARGETS=./cmd/app  ./build-pruned-src.sh
#
#   # BUILD_TARGETS may be space-separated for several binaries:
#   BUILD_TARGETS="./cmd/app ./cmd/tool"  ...
set -euo pipefail

SRC="${SRC:?set SRC=/abs/path/to/source/repo}"
OUT="${OUT:?set OUT=/abs/path/to/dataset/_src}"
MODULE="${MODULE:?set MODULE=the/go/module/path (see 'module' line in go.mod)}"
BUILD_TARGETS="${BUILD_TARGETS:-./...}"   # default: whole module (no pruning)

command -v go >/dev/null 2>&1 || { echo "ERROR: 'go' not on PATH" >&2; exit 1; }
[ -d "$SRC" ] || { echo "ERROR: SRC not a directory: $SRC" >&2; exit 1; }

cd "$SRC"
rm -rf "$OUT"
mkdir -p "$OUT"

# Dependency closure of the build target(s), restricted to this module.
# go list -deps prints the package itself plus all transitive deps.
mapfile -t PKGS < <(go list -deps $BUILD_TARGETS 2>/dev/null | grep "^$MODULE")
[ "${#PKGS[@]}" -gt 0 ] || { echo "ERROR: 0 in-module packages for '$BUILD_TARGETS' under '$MODULE'" >&2; exit 1; }

echo "build-participating in-module packages: ${#PKGS[@]}"

# Copy go.mod / go.sum so the pruned tree stays module-coherent for go/types.
cp go.mod go.sum "$OUT"/ 2>/dev/null || true

count_files=0
for pkg in "${PKGS[@]}"; do
  rel="${pkg#"$MODULE"/}"
  [ "$pkg" = "$MODULE" ] && rel="."          # module root package, if any
  srcdir="$SRC/$rel"
  dstdir="$OUT/$rel"
  mkdir -p "$dstdir"
  # Copy top-level package files only (Go packages are flat). Exclude tests.
  # Keep ALL non-test files (incl. //go:embed assets, .s, .c) for fidelity.
  shopt -s nullglob dotglob
  for f in "$srcdir"/*; do
    [ -f "$f" ] || continue                  # skip subdirs (separate packages)
    case "$(basename "$f")" in
      *_test.go) continue ;;                 # tests don't participate in build
    esac
    cp "$f" "$dstdir"/
    count_files=$((count_files+1))
  done
  shopt -u nullglob dotglob
done

echo "copied files: $count_files"
echo "pruned tree at: $OUT"
echo "--- go file count ---"
find "$OUT" -name '*.go' | wc -l
echo "--- size ---"
du -sh "$OUT"
echo "next: SRC=$SRC OUT=$OUT ./copy-embeds.sh"
