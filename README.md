# code-knowledge-system (cks)

Composes [`code-knowledge-graph`](https://github.com/0xmhha/code-knowledge-graph) (ckg) and
[`code-knowledge-vector`](https://github.com/0xmhha/code-knowledge-vector) (ckv) into a
token-budgeted, sanitized `EvidencePack` and exposes it through MCP for upper layers
(coding agent, external LLM clients).

## Status

Pre-α (Phase 0 scaffold).

## Components

| Binary | Purpose | Phase |
|---|---|---|
| `cmd/cks-mcp`  | MCP server (stdio JSON-RPC) — exposes `cks.context.*`, `cks.ops.*` | C.5 |
| `cmd/cks-agent` | Coding agent CLI — vibe prompt → PR plan + diffs + tests | D |
| `cmd/cks-eval` | Evaluation harness — headless Claude via `cli-wrapper`, metric collection | E |

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

## Dependencies (planned, not yet wired)

- `github.com/0xmhha/code-knowledge-graph` — graph + BM25 backend (Reader API)
- `github.com/0xmhha/code-knowledge-vector` — vector backend (VectorStore API)
- `github.com/0xmhha/cli-wrapper` — headless Claude wrapper (eval only)
- `github.com/mark3labs/mcp-go` — MCP server (Phase C.5)

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
