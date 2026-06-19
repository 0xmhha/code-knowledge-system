# dataset-toolkit

Project-agnostic tools + guides for **building and binding code-knowledge
datasets** (ckg graph + ckv vector) for any Go source snapshot — not just
go-stablenet. Generalized from the per-PR dataset builds under
`knowledge-data/{pr-14,pr-58,pr-77}/`, with hardcoded module / path / domain
specifics lifted out into env vars.

Use this when you need to stand up a *new* dataset (a different repo, a PR
snapshot, a pruned build scope) and point coding-agent's cks MCP at it. For the
production go-stablenet dataset, keep using `scripts/build-stablenet-dataset.sh`
and `scripts/gen-cks-config.sh` at the repo root — this toolkit is the reusable
generalization of those.

## Contents

### `scripts/` — the build pipeline (run in order; all driven by env vars)

| script | what it does |
|--------|--------------|
| `build-pruned-src.sh` | copy only build-participating packages (dep closure of `BUILD_TARGETS`) into `_src/` |
| `copy-embeds.sh` | copy `//go:embed` assets into `_src/` so the pruned tree type-checks |
| `materialize-domain-into-src.sh` | mirror `ckv --docs` dirs into `_src/` so doc/domain chunks pass ckv's citation check |
| `gen-dataset-config.sh` | emit per-dataset `cks-<name>.yaml` + `.env` (absolute paths resolved at run time) |
| `run-coding-agent.sh` | launch Claude Code with cks MCP bound to a specific dataset (shell env wins over settings.json) |

### `docs/`

- `dataset-pipeline.md` — the reproducible step-by-step recipe (what each script feeds).
- `multi-project-setup.md` — how to bind coding-agent / cks / ckg / ckv to
  different datasets across sessions without cross-contamination (the two
  bindings, `.claude/settings.local.json` vs global, restart + verify procedure).

## Quick start

```bash
DATASET=/abs/knowledge-data/myproj
REPO=/abs/path/to/source/repo
MODULE=github.com/acme/app        # from go.mod

# 1-2  pruned source + embeds
SRC=$REPO OUT=$DATASET/_src MODULE=$MODULE BUILD_TARGETS=./cmd/app \
  dataset-toolkit/scripts/build-pruned-src.sh
SRC=$REPO OUT=$DATASET/_src dataset-toolkit/scripts/copy-embeds.sh

# 3    ckg graph
ckg build --src $DATASET/_src --out $DATASET/ckg --lang go

# 5    ckv vector (add --docs DIR for domain corpora)
CKV_OLLAMA_ENDPOINT=http://localhost:11434 \
  ckv build --embedder=ollama --model-name=bge-m3 --src $DATASET/_src --out $DATASET/ckv --lang go

# 6    (only if you used --docs) make those docs retrievable
# SRC=$DATASET/_src DOCS_DIRS=/abs/corpus:/abs/readme dataset-toolkit/scripts/materialize-domain-into-src.sh

# 7    config + env
DATASET=$DATASET NAME=myproj dataset-toolkit/scripts/gen-dataset-config.sh

# run coding-agent against it
CODE=$REPO ENV_FILE=$DATASET/cks-myproj.env dataset-toolkit/scripts/run-coding-agent.sh
```

See `docs/dataset-pipeline.md` for the full table and the citation-trick rationale.
