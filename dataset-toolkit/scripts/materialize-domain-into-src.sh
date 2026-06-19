#!/usr/bin/env bash
# materialize-domain-into-src.sh — make ckv "domain"/doc chunks RETRIEVABLE.
# -----------------------------------------------------------------------------
# Why this exists (the trick worth keeping):
#
# When ckv build is given `--docs DIR`, each doc's chunk stores a path RELATIVE
# to DIR (e.g. "entries/A14.md", "shared/SCHEMA.md", "README.md"). At query time
# ckv enforces citations via os.Stat(manifest.src_root + "/" + chunk.File). The
# doc files do NOT live under src_root (the pruned _src), so every doc chunk is
# dropped as an unverifiable citation — semantic_search never returns them.
#
# verifyCitation only checks file EXISTENCE + line ordering (not content), so we
# just place each doc at the exact path its chunk stored, UNDER _src. No
# re-embedding: the already-embedded doc chunks light up.
#
# Generic operation: for each docs dir that was passed to `ckv build --docs`,
# mirror its contents into _src preserving relative paths (cp -R DIR/. _src/).
#
# Run this ONLY AFTER ckv build completes — running it before would make the
# `--src` walk double-index these files as code. Idempotent.
#
# Usage:
#   SRC=/path/to/dataset/_src \
#   DOCS_DIRS="/abs/corpus:/abs/domain-knowledge:/abs/readme-dir" \
#     ./materialize-domain-into-src.sh
#
#   DOCS_DIRS is a colon-separated list of the SAME dirs you passed to
#   `ckv build --docs ...`, in the same order.
set -euo pipefail
SRC="${SRC:?set SRC=/abs/path/to/dataset/_src}"
DOCS_DIRS="${DOCS_DIRS:?set DOCS_DIRS=colon-separated list of the --docs dirs used at ckv build}"
[ -d "$SRC" ] || { echo "ERROR: SRC not found: $SRC" >&2; exit 1; }

IFS=':' read -r -a DIRS <<< "$DOCS_DIRS"
copied=0
for d in "${DIRS[@]}"; do
  [ -n "$d" ] || continue
  if [ ! -d "$d" ]; then echo "WARN: docs dir not found, skipping: $d" >&2; continue; fi
  # Mirror DIR's contents into _src, preserving the relative paths ckv stored.
  cp -R "$d"/. "$SRC"/
  n=$(find "$d" -type f | wc -l | tr -d ' ')
  copied=$((copied + n))
  echo "materialized $n file(s) from $d -> $SRC"
done

echo "done: $copied doc file(s) placed under $SRC (paths now resolve for ckv citation checks)"
echo "verify (expect 0 missing): compare ckv 'category=doc/domain' chunk paths against \$SRC"
