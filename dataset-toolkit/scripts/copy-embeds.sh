#!/usr/bin/env bash
# copy-embeds.sh — copy //go:embed-referenced assets into a pruned source tree so
# the build-participating packages keep their embedded files (contract artifacts,
# trusted_setup.json, web assets, …). Run AFTER build-pruned-src.sh; without it
# `go list ./...` over the pruned tree fails on missing embed targets.
#
# Generalized from knowledge-data/pr-14/copy-embeds.sh — drive via SRC/OUT.
#
# Usage:
#   SRC=/path/to/original/repo OUT=/path/to/dataset/_src  ./copy-embeds.sh
set -euo pipefail
SRC="${SRC:?set SRC=/abs/path/to/original/source/repo}"
OUT="${OUT:?set OUT=/abs/path/to/dataset/_src}"
[ -d "$OUT" ] || { echo "ERROR: OUT not found (run build-pruned-src.sh first): $OUT" >&2; exit 1; }

cd "$OUT"
# For every pruned .go file carrying an embed directive, resolve its patterns
# against the ORIGINAL tree and copy matches (files or dirs) into the pruned tree.
grep -rl '//go:embed' --include='*.go' . | while read -r gofile; do
  reldir="$(dirname "${gofile#./}")"
  # Collect patterns from all embed lines in this file (strip optional all: prefix, quotes).
  patterns=$(grep '//go:embed' "$gofile" | sed 's#.*//go:embed ##' | tr ' ' '\n' \
             | sed -e 's/^all://' -e 's/^"//' -e 's/"$//' | grep -v '^$')
  ( cd "$SRC/$reldir"
    for pat in $patterns; do
      for match in $pat; do            # shell-glob against real files
        [ -e "$match" ] || continue
        dst="$OUT/$reldir/$(dirname "$match")"
        mkdir -p "$dst"
        cp -R "$match" "$dst"/
      done
    done
  )
done
echo "embeds copied. re-validating with go list..."
cd "$OUT"
if go list ./... >/dev/null 2>list-err.txt; then
  echo "OK: go list ./... clean ($(go list ./... 2>/dev/null | wc -l | tr -d ' ') packages)"
  rm -f list-err.txt
else
  echo "remaining go list issues:"; cat list-err.txt
fi
