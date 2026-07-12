#!/usr/bin/env bash
#
# reindex-dataset.sh — coordinated CKG+CKV reindex orchestrator (reindex P1-3).
#
# Implements the 3-party agreed pipeline (ckv docs/reindex-migration-design
# §3.2/§4/§5 + consensus §10.6):
#
#   lock → build CKG → version dir <family>@<short-commit>-<digest8>
#        → build/align CKV (sources ledger) → verification gate
#        → atomic promote (<family>@current symlink) → [restart serving]
#
# The serving instance is never touched in place: new artifacts are built in an
# immutable sibling version directory and only the `current` symlink moves.
# cks-mcp resolves the symlink ONCE at boot (pinned identity for its lifetime),
# so promotion takes effect on the next instance (re)start — by agreement,
# transitions happen at session/bench boundaries (or via instance blue-green,
# see docs/ops-blue-green-reindex.md).
#
# Usage:
#   scripts/reindex-dataset.sh run     FAMILY=<name> SRC=/abs/checkout [opts]
#   scripts/reindex-dataset.sh gate    VERDIR=/abs/<family>@<ver>   # verify only
#   scripts/reindex-dataset.sh promote VERDIR=/abs/<family>@<ver>  # flip symlink
#   scripts/reindex-dataset.sh status  FAMILY=<name>
#   scripts/reindex-dataset.sh plan    FAMILY=<name> SRC=...       # print, no exec
#
# Options (env):
#   KD           knowledge-data root        (default: <cks>/../knowledge-data)
#   CKG_BIN      ckg binary                 (default: ../code-knowledge-graph/bin/ckg)
#   CKV_BIN      ckv binary                 (default: ../code-knowledge-vector/bin/ckv)
#   LANGS        ckg/ckv languages          (default: auto)
#   CKG_POLICY   ckg --policy-file          (optional)
#   CKV_POLICY   ckv --policy               (optional)
#   CKV_DOCS     comma-separated --docs dirs(optional)
#   CKV_FLOW     ckv --flow-corpus jsonl    (optional)
#   CKV_PR=1     ckv --include-pr-history   (optional)
#   FILES_FROM   include/exclude JSON for BOTH builders (optional)
#   EMBEDDER     ckv embedder               (default: ollama)
#   MODEL        ckv model name             (default: bge-m3)
#   SKIP_LIVE_GATE=1  skip the B7 canonical match-rate gate (needs ollama + cks repo)
#   RESTART=1    after promote: regenerate config to @current + restart serving
#
# Gate (§5.1, all must pass before promote):
#   1. ckg validate           (integrity: dangling edges, schema invariants)   HARD
#   2. manifest alignment     (src_commit equal, schema >= 1.19, digest when
#                              both sides publish one)                          HARD
#   3. counts                 (ckv chunk_count > 0)                             HARD
#   4. B7 canonical match-rate >= 90% (cks live fixture)                        HARD
#      (SKIP_LIVE_GATE=1 downgrades 4 to a logged skip — e.g. no ollama)
#   5. ckg audit              (file-set comparison)                             SOFT
set -euo pipefail

CKS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KD="${KD:-$(cd "$CKS_ROOT/../knowledge-data" && pwd)}"
CKG_BIN="${CKG_BIN:-$CKS_ROOT/../code-knowledge-graph/bin/ckg}"
CKV_BIN="${CKV_BIN:-$CKS_ROOT/../code-knowledge-vector/bin/ckv}"
LANGS="${LANGS:-auto}"
EMBEDDER="${EMBEDDER:-ollama}"
MODEL="${MODEL:-bge-m3}"

log()  { echo "[reindex] $*"; }
die()  { echo "[reindex] ERROR: $*" >&2; exit 1; }

json_get() { # json_get <file> <python-expr over d>
  python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(eval(sys.argv[2]) or '')" "$1" "$3" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# lock — mkdir is atomic and portable (macOS has no flock(1)).
# ---------------------------------------------------------------------------
LOCKDIR=""
acquire_lock() {
  LOCKDIR="$KD/.${FAMILY}.build.lock.d"
  if ! mkdir "$LOCKDIR" 2>/dev/null; then
    die "another build holds the lock: $LOCKDIR (pid $(cat "$LOCKDIR/pid" 2>/dev/null || echo '?')) — remove it only if that build is dead"
  fi
  echo $$ > "$LOCKDIR/pid"
  trap 'release_lock' EXIT
}
release_lock() { [ -n "$LOCKDIR" ] && rm -rf "$LOCKDIR"; }

# ---------------------------------------------------------------------------
# gate — verify a version dir (graph-db/ + vector-db/) before promote.
# ---------------------------------------------------------------------------
run_gate() {
  local ver_dir="$1"
  local graph_dir="$ver_dir/graph-db" vector_dir="$ver_dir/vector-db"
  [ -f "$graph_dir/graph.db" ]        || die "gate: missing $graph_dir/graph.db"
  [ -f "$vector_dir/manifest.json" ]  || die "gate: missing $vector_dir/manifest.json"

  log "gate 1/5: ckg validate"
  "$CKG_BIN" validate --graph "$graph_dir" || die "gate: ckg validate failed"

  log "gate 2/5: manifest alignment (src_commit / schema>=1.19 / digest)"
  python3 - "$graph_dir/manifest.json" "$vector_dir/manifest.json" <<'PY' || exit 1
import json, sys
ckg = json.load(open(sys.argv[1])); ckv = json.load(open(sys.argv[2]))
errs = []
g_commit = ckg.get("src_commit","")
led = (ckv.get("sources") or {}).get("ckg") or {}
v_commit = led.get("src_commit") or ckv.get("src_commit") or ckv.get("indexed_head") or ""
if g_commit and v_commit and g_commit != v_commit:
    errs.append(f"src_commit mismatch: ckg={g_commit[:9]} ckv={v_commit[:9]}")
schema = str(ckg.get("schema_version",""))
try:
    maj, minor = (int(x) for x in schema.split(".")[:2])
    if (maj, minor) < (1, 19):
        errs.append(f"ckg schema {schema} < 1.19 (canonical_id unpopulated)")
except ValueError:
    errs.append(f"unparsable ckg schema_version: {schema!r}")
g_dig = ckg.get("graph_digest",""); v_dig = led.get("graph_digest","")
if g_dig and v_dig and g_dig != v_dig:
    errs.append(f"graph_digest mismatch: ckg={g_dig[:12]} ckv={v_dig[:12]}")
if not led:
    print("  note: ckv sources.ckg ledger absent (pre-P1 index) — compared top-level src_commit")
if errs:
    print("ALIGNMENT GATE FAILED:", "; ".join(errs), file=sys.stderr); sys.exit(1)
print(f"  aligned: commit={v_commit[:9]} schema={schema} digest={'✓' if g_dig and v_dig else '(pending CKG publication)'}")
PY

  log "gate 3/5: counts"
  local chunks
  chunks="$(json_get "$vector_dir/manifest.json" d "d.get('chunk_count',0)")"
  [ -n "$chunks" ] && [ "$chunks" -gt 0 ] 2>/dev/null || die "gate: ckv chunk_count=$chunks"
  log "  chunk_count=$chunks"

  if [ "${SKIP_LIVE_GATE:-0}" = "1" ]; then
    log "gate 4/5: B7 canonical match-rate — SKIPPED (SKIP_LIVE_GATE=1)"
  else
    log "gate 4/5: B7 canonical match-rate >= 90% (live, needs ollama)"
    ( cd "$CKS_ROOT" && \
      CKS_B7_LIVE_CKG="$graph_dir/graph.db" CKS_B7_LIVE_CKV="$vector_dir" \
        go test ./internal/ckgclient/ -run TestLive_B7 -count=1 >/dev/null ) \
      || die "gate: B7 canonical match-rate below target"
    log "  B7 gate passed"
  fi

  log "gate 5/5: ckg audit (soft)"
  if ! "$CKG_BIN" audit --graph "$graph_dir" ${SRC:+--src "$SRC"} 2>/dev/null; then
    log "  WARN: ckg audit reported differences (soft gate — inspect before relying on file-set completeness)"
  fi
  log "gate: ALL PASSED for $ver_dir"
}

# ---------------------------------------------------------------------------
# promote — atomically point <family>@current at a gated version dir.
# ---------------------------------------------------------------------------
do_promote() {
  local ver_dir="$1"
  [ -d "$ver_dir" ] || die "promote: no such version dir: $ver_dir"
  [ -f "$ver_dir/.building" ] && die "promote: $ver_dir still has a .building sentinel (gate not completed)"
  local family_base; family_base="$(basename "$ver_dir")"; family_base="${family_base%%@*}"
  local link="$KD/${family_base}@current"
  case "$(uname -s)" in
    Darwin) ln -sfh "$ver_dir" "$link" ;;
    *)      ln -sfn "$ver_dir" "$link" ;;
  esac
  log "promoted: $link -> $ver_dir"
  log "rollback: re-run promote with the previous version dir (old versions are preserved)"
}

# ---------------------------------------------------------------------------
cmd="${1:-}"; shift || true
case "$cmd" in

plan|run)
  FAMILY="${FAMILY:?set FAMILY=<dataset family, e.g. pr-77-2>}"
  SRC="${SRC:?set SRC=/abs/path/to/indexed source checkout}"
  SRC="$(cd "$SRC" && pwd)"
  [ -x "$CKG_BIN" ] || die "ckg binary not executable: $CKG_BIN"
  [ -x "$CKV_BIN" ] || die "ckv binary not executable: $CKV_BIN"

  COMMIT="$(git -C "$SRC" rev-parse HEAD)"
  DIRTY="$(git -C "$SRC" status --porcelain | wc -l | tr -d ' ')"
  [ "$DIRTY" = "0" ] || log "WARN: source checkout has $DIRTY local changes — determinism (ADR-0002) expects detached+clean"

  log "family=$FAMILY  src=$SRC  commit=${COMMIT:0:9}  langs=$LANGS  embedder=$EMBEDDER/$MODEL"
  if [ "$cmd" = "plan" ]; then
    log "plan: lock -> ckg build -> version dir @<commit8>-<digest8> -> ckv build(--ckg align) -> gate(5) -> promote @current${RESTART:+ -> restart}"
    exit 0
  fi

  acquire_lock
  STAGING="$KD/${FAMILY}@staging-$$"
  mkdir -p "$STAGING/graph-db"
  touch "$STAGING/.building"

  log "step 1/4: ckg build"
  "$CKG_BIN" build --src "$SRC" --out "$STAGING/graph-db" --lang "$LANGS" \
    ${CKG_POLICY:+--policy-file "$CKG_POLICY"} \
    ${FILES_FROM:+--files-from "$FILES_FROM"}

  DIGEST="$(json_get "$STAGING/graph-db/manifest.json" d "d.get('graph_digest','')")"
  if [ -n "$DIGEST" ]; then VER="${COMMIT:0:8}-${DIGEST:0:8}"
  else VER="${COMMIT:0:8}-$(date -u +%Y%m%d%H%M%S)"; log "note: ckg graph_digest not published yet — interim timestamped version"
  fi
  VER_DIR="$KD/${FAMILY}@${VER}"
  [ -e "$VER_DIR" ] && die "version dir already exists (immutable): $VER_DIR"
  mv "$STAGING" "$VER_DIR"
  log "version dir: $VER_DIR"

  log "step 2/4: ckv build + ckg align (sources ledger)"
  docs_flags=()
  if [ -n "${CKV_DOCS:-}" ]; then
    IFS=',' read -ra _docs <<< "$CKV_DOCS"
    for d in "${_docs[@]}"; do docs_flags+=(--docs "$d"); done
  fi
  ${OLLAMA_URL:+export CKV_OLLAMA_ENDPOINT="$OLLAMA_URL"}
  "$CKV_BIN" build --src "$SRC" --out "$VER_DIR/vector-db" \
    --ckg "$VER_DIR/graph-db" \
    --embedder "$EMBEDDER" --model-name "$MODEL" \
    ${CKV_POLICY:+--policy "$CKV_POLICY"} \
    ${CKV_FLOW:+--flow-corpus "$CKV_FLOW"} \
    ${FILES_FROM:+--files-from "$FILES_FROM"} \
    ${CKV_PR:+--include-pr-history} \
    "${docs_flags[@]}"

  log "step 3/4: verification gate"
  run_gate "$VER_DIR"
  rm -f "$VER_DIR/.building"

  log "step 4/4: promote"
  do_promote "$VER_DIR"

  if [ "${RESTART:-0}" = "1" ]; then
    log "restart: regenerating config to @current + restarting serving instance"
    CKS_DATASET_DIR="$KD/${FAMILY}@current" GO_STABLENET_ROOT="$SRC" \
      "$CKS_ROOT/scripts/gen-cks-config.sh"
    "$CKS_ROOT/scripts/serve-cks-http.sh" restart
  else
    log "next: CKS_DATASET_DIR=$KD/${FAMILY}@current GO_STABLENET_ROOT=$SRC scripts/gen-cks-config.sh && scripts/serve-cks-http.sh restart"
  fi
  ;;

gate)
  VERDIR="${VERDIR:?set VERDIR=/abs/<family>@<ver> (must contain graph-db/ + vector-db/)}"
  run_gate "$(cd "$VERDIR" && pwd)"
  ;;

promote)
  VERDIR="${VERDIR:?set VERDIR=/abs/<family>@<ver>}"
  do_promote "$(cd "$VERDIR" && pwd)"
  ;;

status)
  FAMILY="${FAMILY:?set FAMILY=<dataset family>}"
  link="$KD/${FAMILY}@current"
  if [ -L "$link" ]; then log "current -> $(readlink "$link")"
  else log "no @current symlink for $FAMILY (legacy flat layout still in use?)"; fi
  ls -d "$KD/${FAMILY}"@* 2>/dev/null | grep -v '@current$' | sed 's/^/[reindex]   version: /' || log "  (no version dirs yet)"
  [ -d "$KD/.${FAMILY}.build.lock.d" ] && log "LOCK HELD by pid $(cat "$KD/.${FAMILY}.build.lock.d/pid" 2>/dev/null)" || true
  ;;

*)
  echo "usage: $0 {run|plan|gate|promote|status}  (see header comments for env options)"
  exit 2
  ;;
esac
