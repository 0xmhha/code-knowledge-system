#!/usr/bin/env bash
# build-stablenet-dataset.sh
# ---------------------------------------------------------------------------
# Reproducibly build the go-stablenet code-knowledge dataset:
#   ckg graph index  +  ckv bge-m3 vector index  +  cks domain corpus/policies
#   +  a wired cks.yaml.
#
# Idempotent: safe to re-run. Each stage rebuilds its own artifact.
# Run from the code-knowledge-system repo root.
#
# Usage:
#   ./scripts/build-stablenet-dataset.sh            # full build
#   STAGE=ckg ./scripts/build-stablenet-dataset.sh  # only the ckg stage
#   SKIP_CKV=1 ./scripts/build-stablenet-dataset.sh # everything except the ~10h ckv embed
#
# Env (override as needed):
#   GO_STABLENET_ROOT  go-stablenet checkout            (default: ../go-stablenet)
#   CKG_REPO           code-knowledge-graph checkout    (default: ../code-knowledge-graph)
#   CKV_REPO           code-knowledge-vector checkout   (default: ../code-knowledge-vector)
#   EMBED_MODEL        ollama embedding model           (default: bge-m3)
#   OLLAMA_URL         ollama daemon endpoint           (default: http://localhost:11434)
# ---------------------------------------------------------------------------
set -euo pipefail

CKS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$CKS_ROOT"

GO_STABLENET_ROOT="${GO_STABLENET_ROOT:-$(cd "$CKS_ROOT/../go-stablenet" 2>/dev/null && pwd || echo "")}"
CKG_REPO="${CKG_REPO:-$CKS_ROOT/../code-knowledge-graph}"
CKV_REPO="${CKV_REPO:-$CKS_ROOT/../code-knowledge-vector}"
EMBED_MODEL="${EMBED_MODEL:-bge-m3}"
OLLAMA_URL="${OLLAMA_URL:-http://localhost:11434}"
STAGE="${STAGE:-all}"
SKIP_CKV="${SKIP_CKV:-0}"

CKG_BIN="$CKG_REPO/bin/ckg"
CKV_BIN="$CKV_REPO/bin/ckv"

# Artifact layout (under cks repo root)
PROJECT_DIR="docs/domain-knowledge/projects/go-stablenet"
CORPUS_DIR="generated/domain-corpus/go-stablenet"
POLICY_DIR="generated/policies"
CKG_POLICY="$POLICY_DIR/stablenet-ckg-policy.yaml"
CKV_POLICY="$POLICY_DIR/stablenet-ckv.yaml"
CKG_OUT="data/ckg-stablenet"
CKV_OUT="data/ckv-stablenet"

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
die() { printf '\033[1;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }
run_stage() { [ "$STAGE" = "all" ] || [ "$STAGE" = "$1" ]; }

[ -n "$GO_STABLENET_ROOT" ] && [ -d "$GO_STABLENET_ROOT" ] || die "GO_STABLENET_ROOT not found: '$GO_STABLENET_ROOT'"
export GO_STABLENET_ROOT

log "config"
cat <<EOF
  CKS_ROOT          = $CKS_ROOT
  GO_STABLENET_ROOT = $GO_STABLENET_ROOT
  CKG_BIN           = $CKG_BIN
  CKV_BIN           = $CKV_BIN
  EMBED_MODEL       = $EMBED_MODEL
  OLLAMA_URL        = $OLLAMA_URL
  STAGE             = $STAGE   SKIP_CKV=$SKIP_CKV
EOF

# ── Stage 0: binaries ──────────────────────────────────────────────────────
if run_stage bins; then
  log "Stage: build binaries"
  ( cd "$CKG_REPO" && make build-no-viewer )
  ( cd "$CKV_REPO" && make build )
  make build-bins
  go build -o bin/cks-domain-export ./cmd/cks-domain-export
  go build -o bin/cks-domain-sync   ./cmd/cks-domain-sync
fi
[ -x "$CKG_BIN" ] || die "ckg binary missing: $CKG_BIN (run STAGE=bins)"
[ -x "$CKV_BIN" ] || die "ckv binary missing: $CKV_BIN (run STAGE=bins)"

# ── Stage 1: domain corpus (Channel ②) ─────────────────────────────────────
if run_stage domain || run_stage all; then
  log "Stage: domain corpus export"
  mkdir -p "$CORPUS_DIR"
  ./bin/cks-domain-export -project "$PROJECT_DIR" -out "$CORPUS_DIR"
fi

# ── Stage 2: policy sync (ckv + ckg views) ─────────────────────────────────
if run_stage policy || run_stage all; then
  log "Stage: policy sync"
  mkdir -p "$POLICY_DIR"
  ./bin/cks-domain-sync -entries "$PROJECT_DIR" \
    -ckv-out "$CKV_POLICY" -ckg-out "$CKG_POLICY"
fi

# ── Stage 3: ckg graph index (LLM-free) ────────────────────────────────────
if run_stage ckg || run_stage all; then
  log "Stage: ckg build (graph)"
  rm -rf "$CKG_OUT"; mkdir -p "$CKG_OUT"
  "$CKG_BIN" build --src "$GO_STABLENET_ROOT" --out "$CKG_OUT" \
    --lang go --policy-file "$CKG_POLICY"
  "$CKG_BIN" validate --graph "$CKG_OUT" --format json >/dev/null \
    && echo "ckg validate: OK" || echo "ckg validate: WARN (non-fatal)"
fi

# ── Stage 4: ckv vector index (bge-m3 via Ollama) — LONG (~10h) ─────────────
if { run_stage ckv || run_stage all; } && [ "$SKIP_CKV" != "1" ]; then
  log "Stage: ckv build (bge-m3 embed — this can take hours)"
  curl -fsS "$OLLAMA_URL/api/version" >/dev/null 2>&1 || die "Ollama not reachable at $OLLAMA_URL (run: ollama serve)"
  ollama list 2>/dev/null | grep -q "$EMBED_MODEL" || die "model '$EMBED_MODEL' not pulled (run: ollama pull $EMBED_MODEL)"
  rm -rf "$CKV_OUT"; mkdir -p "$CKV_OUT"
  CKV_OLLAMA_ENDPOINT="$OLLAMA_URL" "$CKV_BIN" build \
    --embedder=ollama --model-name="$EMBED_MODEL" \
    --src "$GO_STABLENET_ROOT" --out "$CKV_OUT" \
    --policy "$CKV_POLICY" --docs "$CORPUS_DIR"
else
  log "Stage: ckv build SKIPPED (SKIP_CKV=1 or STAGE!=ckv/all)"
fi

# ── Stage 5: generate config + env from current paths ──────────────────────
if run_stage config || run_stage all; then
  log "Stage: generate cks config + env (resolved paths)"
  GO_STABLENET_ROOT="$GO_STABLENET_ROOT" CKG_REPO="$CKG_REPO" CKV_REPO="$CKV_REPO" \
    CKV_OLLAMA_ENDPOINT="$OLLAMA_URL" EMBED_MODEL="$EMBED_MODEL" \
    "$CKS_ROOT/scripts/gen-cks-config.sh"
fi

log "done — artifacts:"
ls -la "$CKG_OUT"/graph.db 2>/dev/null || true
ls -la "$CKV_OUT"/vector.db 2>/dev/null || true
echo "Next: source ./cks.env  &&  ./bin/cks-mcp -config ./cks-stablenet.yaml"
