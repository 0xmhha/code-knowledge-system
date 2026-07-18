# code-knowledge-system (cks)

Composes [`code-knowledge-graph`](https://github.com/0xmhha/code-knowledge-graph) (ckg) and
[`code-knowledge-vector`](https://github.com/0xmhha/code-knowledge-vector) (ckv) into a
token-budgeted, sanitized `EvidencePack` and exposes it through MCP for upper layers
(coding agent, external LLM clients).

## Status

R1′ — wired. cks composes ckv (vector/meaning) and ckg (graph/keyword) **in-process**
and exposes a 19-tool agent-facing MCP surface (`cks.context.*` + `cks.ops.*`) over stdio.
With no datasets configured it boots in a non-crashing degraded mode (Smart Dummy +
`cks.ops.health` reports `degraded`).

## Components

| Binary | Purpose | Phase |
|---|---|---|
| `cmd/cks-mcp`  | MCP server (stdio JSON-RPC) — exposes `cks.context.*`, `cks.ops.*` | C.5 |
| `cmd/cks-agent` | Coding agent CLI — vibe prompt → PR plan + diffs + tests | D |
| `cmd/cks-eval` | Evaluation harness — headless Claude via `cli-wrapper`, metric collection | E |
| `cmd/cks-glossary-gen` | Build the alias glossary that feeds the vocab resolver | domain |
| `cmd/cks-domain-sync` | Derive ckv/ckg policy views from verified domain entries | domain |
| `cmd/cks-domain-export` | Render verified entries → markdown corpus for `ckv build --docs` | domain |
| `cmd/cks-entry-verify` | Validate domain entries against schema + anchors | domain |
| `cmd/cks-inventory-check` | Cross-check domain-entry inventory vs coverage | domain |
| `cmd/cks-anchor-refresh` | Re-stamp entry code anchors against current HEAD | domain |
| `cmd/cks-promotion-worksheet` | Draft/needs_verification → verified promotion worksheet | domain |

## Architecture

```
external LLM client / coding agent
            │  MCP / HTTP loopback
            ▼
       ┌────────────┐
       │  cks       │  composer: intent → fan-out(ckv+ckg) → RRF
       │  (this     │            → graph 1-hop → token budget
       │   repo)    │            → sanitize → EvidencePack
       └─────┬──────┘
             │
       ┌─────┴─────┐
       ▼           ▼
     ckv         ckg
   (vector)   (graph+BM25)
```

No LLM calls inside cks itself; LLM is invoked only by `cks-agent` and `cks-eval`.

## Build

```
make build         # go build ./...
make test          # go test -race ./...
make test-short    # go test -short ./...
make lint          # golangci-lint run
make fmt           # gofmt -s -w .
make tidy          # go mod tidy
```

## Run as an MCP server

```
make build-bins                                 # -> ./bin/cks-mcp  (CGO required: sqlite-vec)
cp policies/cks.yaml.example ./cks.yaml          # edit backend paths / source_root / bge-m3 / ollama_url
./bin/cks-mcp -config ./cks.yaml                 # serves stdio; -config is the only flag
```

`-config` is optional — omitted, it falls back to `config.Default()` (dummy backends, dev
mode). Real backends activate when `backends.ckv.path` / `backends.ckg.path` point at built
ckv/ckg datasets and a live Ollama serves the `embed_model` (bge-m3, 1024-dim).

Register it with a client:

```
claude mcp add cks -- /abs/path/bin/cks-mcp -config /abs/path/cks.yaml
```

or in a `.mcp.json`:

```json
{ "mcpServers": { "cks": { "command": "/abs/path/bin/cks-mcp",
                           "args": ["-config", "/abs/path/cks.yaml"] } } }
```

Building the datasets the config points at:

```
cks-domain-sync …                                # derive ckv/ckg policy views from verified entries
ckv build --src <go-stablenet> --out <ckv-data> --embedder=ollama --model-name=bge-m3
ckg build --src <go-stablenet> --out <ckg-data> --policy-file policies/policy.yaml
```

Once warm, the agent keeps it fresh via `cks.ops.freshness` → `cks.ops.index` (which shells
the same `ckv`/`ckg` builds, forwarding `--policy-file` when `backends.ckg.policy_file` is set).

## Dependencies (wired)

- `github.com/0xmhha/code-knowledge-graph` — graph + BM25 backend (`pkg/store`, in-process); pinned at released **v0.1.0** (no `replace`)
- `github.com/0xmhha/code-knowledge-vector` — vector backend (`pkg/ckv`, in-process; sqlite-vec CGO); pinned at released **v0.1.0** (no `replace`)
- `github.com/mark3labs/mcp-go` — MCP server (v0.56.0)

## Layout (target)

```
cmd/
├── cks-mcp/        MCP entry
├── cks-agent/      Agent CLI
└── cks-eval/       Eval harness
pkg/
├── contract/       Public types (Citation, EvidencePack, Hit alias)
└── client/         ckg/ckv client wrappers (interface + real + fake)
internal/
├── envelope/       trace_id / run_id propagation
├── footprint/      structured JSONL logging
├── auditlog/       append-only audit
├── composer/       intent / planner / fuser / expand / budget / sanitize / pack
├── adapter/        mcp / http / cli
├── agent/          extractor / analyzer / splitter / codegen / tester / verify
└── eval/           scenario / runner / metrics / report
policies/           sanitization_rules.yaml, capability_policy.yaml
eval/
├── scenarios/      *.yaml (e.g. stablenet-pr70.yaml)
└── baselines/      *.diff, *.files.txt
```
