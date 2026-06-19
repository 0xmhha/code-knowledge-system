# Per-dataset build pipeline (pruned source + domain knowledge)

Reproducible recipe for building a **self-contained code-knowledge dataset** — a
ckg graph + ckv vector index — for one source snapshot, optionally scoped to the
code that actually ships in a binary, with a domain-knowledge corpus embedded and
made retrievable.

Generalized from `knowledge-data/pr-14/README.md`. Each dataset lives in its own
directory and is **independent** (its cks config never references another
dataset's DB). The `dataset-toolkit/scripts/*` here are the parameterized,
project-agnostic versions of that pipeline.

## Layout of one dataset directory

```
<DATASET>/
  _src/                 pruned source tree (what ckg/ckv actually index)
  ckg/graph.db          ckg graph
  ckv/                  ckv vector index + manifest.json
  logs/{footprint,audit}/
  cks-<name>.yaml       generated cks config (gen-dataset-config.sh)
  cks-<name>.env        generated shell exports
```

## Scope: index only build-participating code

Indexing the whole repo pulls in tools/examples/tests the target binary never
runs, bloating the graph and diluting retrieval. Scope to the dependency closure
of the build target(s):

```
go list -deps ./cmd/<binary> | grep '^<module>'
```

`build-pruned-src.sh` does exactly this — it copies only those in-module packages
(top-level, non-test files, including `//go:embed` assets) into `_src/`. Set
`BUILD_TARGETS=./...` to skip pruning and index the whole module.

## Pipeline (run in order)

| # | Command | Output |
|---|---------|--------|
| 1 | `SRC=<repo> OUT=<DATASET>/_src MODULE=<mod> BUILD_TARGETS=./cmd/<bin> scripts/build-pruned-src.sh` | pruned `_src/` (build-participating pkgs) |
| 2 | `SRC=<repo> OUT=<DATASET>/_src scripts/copy-embeds.sh` | embed assets in `_src/`; validates `go list ./...` |
| 3 | `ckg build --src <DATASET>/_src --out <DATASET>/ckg --lang go` | `ckg/` graph |
| 4 | *(optional)* `cks-domain-export` + stage README/docs into a corpus dir | doc dirs for `--docs` |
| 5 | `ckv build --embedder=ollama --model-name=bge-m3 --src <DATASET>/_src --out <DATASET>/ckv --lang go [--docs DIR ...]` | `ckv/` vector (code [+ docs]) |
| 6 | *(if you used `--docs`)* `SRC=<DATASET>/_src DOCS_DIRS=DIR1:DIR2 scripts/materialize-domain-into-src.sh` | docs placed under `_src/` so citations resolve |
| 7 | `DATASET=<DATASET> NAME=<name> scripts/gen-dataset-config.sh` | `cks-<name>.yaml`, `cks-<name>.env` |

### Why step 6 matters (the retrieval trick)

ckv verifies citations at query time via `os.Stat(src_root + "/" + chunk.File)`.
`--docs` chunks store paths relative to each docs dir, which live **outside**
`_src`, so every doc chunk is dropped from results unless the file also exists
under `src_root`. `materialize-domain-into-src.sh` mirrors each `--docs` dir into
`_src/` (preserving relative paths) so the already-embedded doc chunks light up.
Run it **after** ckv (running before would double-index the docs via the `--src`
walk).

To wire domain knowledge as a first-class cks **domain project** instead (policies
+ glossary + governance edges), set `DOMAIN_PROJECT_DIR` / `DOMAIN_CORPUS_DIR` /
`GLOSSARY_PATH` when running `gen-dataset-config.sh`. Datasets that keep domain
docs in ckv only can omit all three.

## Wiring + verification

- `gen-dataset-config.sh` produces `cks-<name>.{yaml,env}`. Point coding-agent at
  the dataset with `run-coding-agent.sh` (`CODE=<repo> ENV_FILE=<DATASET>/cks-<name>.env`).
- MCP config is read **once at session start** — start a new session to pick up changes.
- Verify the binding: cks `freshness.indexed_head` must equal
  `git -C <repo> rev-parse HEAD`. See `multi-project-setup.md` §4.
- Integrity: `ckg validate --graph <DATASET>/ckg --format json` (Issues = null/0)
  and `PRAGMA integrity_check` on both DBs.
- Retrieval smoke test: `cks.context.semantic_search` on a couple of known
  code/domain queries returns the expected files.

See `multi-project-setup.md` for switching the active dataset across sessions
without cross-binding.
