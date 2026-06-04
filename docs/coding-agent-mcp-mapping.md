# coding-agent MCP integration: mapping from HLD virtual signatures to live CKS tools

> This document is the contract between the coding-agent plugin (separate repo) and the CKS MCP server it talks to. It exists because the coding-agent HLD (`docs/superpowers/specs/phase3-cks-mcp-ckv.md` and `phase4-cks-mcp-ckg.md`) was written before CKS was implemented; the HLD describes virtual tool signatures (`ckv_search`, `ckg_query`, etc.) that do not match the actual tool names CKS now exposes. Anyone wiring the coding-agent to CKS should read the mapping here first — guessing the names by analogy will yield "tool not found" errors.

**Authoritative source**: `internal/mcp/*.go` in this repo. When wire names or schemas diverge between this doc and the code, the code wins.

## 1. Architecture in one paragraph

The coding-agent never talks to CKV or CKG directly. It opens a single MCP stdio session against `cks-mcp`. `cks-mcp` exposes 13 tools (table below); under the hood, those tools route to `internal/ckvclient` and `internal/ckgclient`, which open the real ckv/ckg backends **in-process** (no subprocess) or fall back to a Smart Dummy that returns LLM-actionable instructions instead of fake data. From the coding-agent's point of view there is exactly one MCP server to install.

```
coding-agent  ─stdio MCP─▶  cks-mcp  ─Go import─▶  ckvclient ─pkg/ckv (in-process)──▶  ckv
                                       ─Go import─▶  ckgclient ─pkg/store (in-process)─▶  ckg
```

Beyond the original set, **`cks.context.concurrency_impact`** (goroutine/channel/lock blast radius) and **`cks.ops.index`** (agent-triggered ckv+ckg reindex) are now live. The authoritative list is `internal/mcp/*.go`, pinned against `internal/mcp/testdata/agent-mcp.schema.json` by `internal/mcp/schema_golden_test.go`.

## 2. Live tool catalog (13 tools)

Wire names are the strings the MCP client sends; constants live in `internal/mcp/*.go`.

| Wire name | Constant | Required input | Optional input | Purpose |
|---|---|---|---|---|
| `cks.context.get_for_task` | `ToolNameGetForTask` | `prompt` (string) | — | Run the full composer pipeline over a vibe prompt; returns a stamped, sanitized EvidencePack. The one-shot entry the coding-agent should default to. |
| `cks.context.semantic_search` | `ToolNameSemanticSearch` | `query` (string) | `k` (number), `language` (string), `path_glob` (string), `kinds` (string) | CKV semantic search (vector). Returns hits without the composer's intent/sanitize steps. |
| `cks.context.search_text` | `ToolNameSearchText` | `query` (string) | `k` (number), `language` (string), `path_glob` (string) | CKG BM25 text search. Use when the caller already knows an identifier or keyword. |
| `cks.context.find_symbol` | `ToolNameFindSymbol` | `name` (string) | `language` (string), `kinds` (string), `path_glob` (string) | CKG symbol lookup by name. Returns definition site(s). |
| `cks.context.find_callers` | `ToolNameFindCallers` | `symbol` (string) | `depth` (number), `max_total` (number) | Reverse call graph. |
| `cks.context.find_callees` | `ToolNameFindCallees` | `symbol` (string) | `depth` (number), `max_total` (number) | Forward call graph. |
| `cks.context.get_subgraph` | `ToolNameGetSubgraph` | `symbol` (string) | `depth` (number), `max_total` (number) | Bi-directional subgraph rooted at `symbol`. |
| `cks.context.impact_analysis` | `ToolNameImpactAnalysis` | `symbol` (string) | `depth` (number), `max_total` (number) | Same shape as `find_callers` but surfaced as "what would break if I change this". |
| `cks.context.change_history` | `ToolNameChangeHistory` | — | `intent` (string), `symbol` (string), `k` (number), `max_count` (number) | PR-breadcrumb history. Either `intent` or `symbol` must be supplied (handler-level check). |
| `cks.ops.health` | `ToolNameHealth` | — | — | Aggregate backend health: `ok` / `degraded` / `down`. |
| `cks.ops.freshness` | `ToolNameFreshness` | — | — | CKV index freshness vs working tree. Use before relying on `semantic_search` after a long indexing pause. |

## 3. Mapping from HLD virtual signatures

The coding-agent HLD assumes the following shapes. Each row says how to fulfil that call against the live tools.

### 3.1 phase3 (CKV): `ckv_search`

HLD signature (phase3-cks-mcp-ckv.md §7.1):

```ts
ckv_search({
  query: string,
  top_k?: number,
  filters?: { package?, file_pattern?, symbol_type?, modified_since? },
  include_history?: boolean,
  rerank?: boolean,
})
```

Live equivalent — pick **one** of these two depending on the caller's intent:

```jsonc
// (a) Vector path — the HLD's default behaviour.
{ "name": "cks.context.semantic_search",
  "arguments": {
    "query":     "<query>",
    "k":         <top_k>,
    "language":  "go",                // optional; omit to match all
    "path_glob": "<filters.file_pattern>",
    "kinds":     "<filters.symbol_type, comma-separated>"
  } }

// (b) Keyword path — when the caller has a specific identifier in mind.
{ "name": "cks.context.search_text",
  "arguments": {
    "query":     "<query>",
    "k":         <top_k>,
    "language":  "go",
    "path_glob": "<filters.file_pattern>"
  } }
```

Mapping notes:

- `top_k` → `k` (rename only).
- `filters.file_pattern` → `path_glob`. The glob syntax matches `filepath.Match` semantics.
- `filters.symbol_type` → `kinds`. Comma-separated string (e.g. `"function,method"`), not an array.
- `filters.package`, `filters.modified_since`: no direct equivalent. For package, encode it in the path_glob (`consensus/wbft/**`). For modified_since, use `cks.context.change_history` separately.
- `include_history`: not part of `semantic_search`. Fetch with a follow-up `cks.context.change_history` call, or use `cks.context.get_for_task` which folds history into the composer's EvidencePack.
- `rerank`: not exposed as a per-call flag. The composer (`get_for_task`) reranks by RRF over BM25 + symbol lists; `semantic_search` returns raw CKV hits.

### 3.2 phase3 (CKV): `ckv_index`

```ts
ckv_index({ mode, project_root, exclude_patterns? })
```

**Now exposed as `cks.ops.index`** (G8/S2). The agent calls `cks.ops.index{mode:"incremental"|"full", since_commit?}` after `cks.ops.freshness` reports staleness; it shells the same `ckv build`/`ckv reindex` + `ckg build` the operator would run out-of-band (an explicit, infrequent maintenance op, not the hot retrieval path). The query surface remains read-only; only this one maintenance tool can refresh the index, and it is disabled unless `backends.{ckv,ckg}.binary_path` are configured.

To check whether the index is stale after an out-of-band edit, use `cks.ops.freshness`.

### 3.3 phase4 (CKG): `ckg_query`

HLD signature (phase4-cks-mcp-ckg.md §7.1):

```ts
ckg_query({
  symbols: string[],
  depth?: number,
  relation_types?: string[],
  include_history?: boolean,
  include_concurrency?: boolean,
  max_nodes?: number,
})
```

Live equivalent — compose **multiple** calls depending on the relation_types requested:

| HLD `relation_types[]` | Live tool | Notes |
|---|---|---|
| `["calls"]` (forward) | `cks.context.find_callees` | One call per symbol. |
| `["calls"]` (reverse) | `cks.context.find_callers` | One call per symbol. |
| `["calls", ...]` (both + others) | `cks.context.get_subgraph` | Single call per symbol, returns bidirectional. |
| (none specified) | `cks.context.get_subgraph` | Default; widest shape. |

Mapping notes:

- `symbols[]`: live tools accept a single symbol per call. The coding-agent should fan out (one call per symbol) and merge client-side. The composer's `get_for_task` does this fan-out internally based on the prompt's extracted symbols.
- `depth` → `depth` (same).
- `max_nodes` → `max_total` (rename).
- `include_history`: orthogonal — fetch via `cks.context.change_history` and join on node ids client-side.
- `include_concurrency`: **not currently exposed**. The HLD's concurrency analyzer is a CKG roadmap item; for now the field has no live equivalent and should be treated as "false" by callers.

### 3.4 phase4 (CKG): `ckg_impact`

```ts
ckg_impact({ symbol, change_type })
```

Live equivalent:

```jsonc
{ "name": "cks.context.impact_analysis",
  "arguments": {
    "symbol":    "<symbol>",
    "depth":     2,
    "max_total": 200
  } }
```

`change_type` (`signature` / `logic` / `delete`) has no direct CKG counterpart; the live impact_analysis returns the reverse call graph regardless. Risk-level scoring lives client-side (the coding-agent's planner can weight differently per change_type).

For the per-PR history side of impact ("what changed near this code recently"), pair with:

```jsonc
{ "name": "cks.context.change_history",
  "arguments": { "symbol": "<symbol>", "k": 5 } }
```

### 3.5 phase4 (CKG): `ckg_index`

Now folded into `cks.ops.index` (see §3.2): a single tool refreshes both ckv and ckg. The ckg leg shells `ckg build --src --out` (plus `--policy-file` when `backends.ckg.policy_file` is set, rebuilding governed_by edges).

## 4. Composition example — Planner ANALYSIS step

The phase3 HLD §8 / phase4 HLD §8 describes a five-step retrieval pattern. Here it is using live tools.

Step 1 — semantic search. The coding-agent has the planner's vibe prompt and wants the related-code shortlist:

```jsonc
{ "name": "cks.context.semantic_search",
  "arguments": { "query": "staking reward overflow guard", "k": 10, "language": "go" } }
```

Step 2 — pick symbols of interest from the hits (client-side).

Step 3 — graph expansion for each:

```jsonc
{ "name": "cks.context.get_subgraph",
  "arguments": { "symbol": "governance.CalcReward", "depth": 2, "max_total": 200 } }
```

Step 4 — impact + history:

```jsonc
{ "name": "cks.context.impact_analysis",
  "arguments": { "symbol": "governance.CalcReward", "depth": 2 } }

{ "name": "cks.context.change_history",
  "arguments": { "symbol": "governance.CalcReward", "k": 5 } }
```

Step 5 — synthesize into an EvidencePack client-side, **or** replace all four steps with a single composer call:

```jsonc
{ "name": "cks.context.get_for_task",
  "arguments": { "prompt": "staking reward overflow guard" } }
```

`get_for_task` is the recommended default for ANALYSIS. The split-call form is only needed when the coding-agent wants per-step control (e.g. to apply its own intent classifier before retrieval).

## 5. Differences worth flagging to the coding-agent

| HLD assumed | Live reality |
|---|---|
| Tools named `ckv_*` / `ckg_*` | Tools named `cks.context.*` / `cks.ops.*`. Wire names are dotted; underscores are inside the last segment only. |
| Single batched `ckg_query` accepts `symbols[]` | Live tools take a single `symbol` (or `name`). Caller fans out. |
| Indexing exposed as `ckv_index` / `ckg_index` MCP tools | One maintenance tool `cks.ops.index{mode,since_commit?}` refreshes both ckv+ckg (shells the builds); the query surface stays read-only. |
| `include_concurrency` field on `ckg_query` | Replaced by a dedicated tool `cks.context.concurrency_impact{symbol,depth,max_total}` (goroutine/channel/lock blast radius). |
| `change_type` on `ckg_impact` controls risk weighting server-side | Not implemented. Coding-agent owns the change-type → risk-weight mapping client-side. |
| `rerank` toggle on `ckv_search` | Reranking lives inside `get_for_task` (RRF over BM25 + FindSymbol lists). `semantic_search` returns raw vector hits. |
| `top_k` parameter | Renamed `k` everywhere. |
| `file_pattern` filter | Renamed `path_glob`. Uses `filepath.Match` syntax. |

## 6. Health gating

Before issuing retrieval calls in a long-lived session, the coding-agent should:

```jsonc
{ "name": "cks.ops.health", "arguments": {} }
```

A `degraded` rollup (ckv unreachable but ckg ok) is still useful: the coding-agent can fall back to `search_text` / `find_symbol` / `find_callers` and skip `semantic_search`. A `down` rollup (ckg unreachable) means retrieval cannot proceed; the planner should surface the error rather than silently degrade.

`cks.ops.freshness` is a soft check — returns the CKV index's recorded last-built commit vs the working tree. The coding-agent should warn the user (not block) when freshness is stale.

## 7. Cross-references

- Live tool source: `internal/mcp/server.go`, `internal/mcp/graph.go`, `internal/mcp/search.go`, `internal/mcp/analysis.go`, `internal/mcp/freshness.go`, `internal/mcp/health.go`, `internal/mcp/get_for_task.go`.
- coding-agent HLDs (separate repo): `docs/superpowers/specs/phase3-cks-mcp-ckv.md`, `docs/superpowers/specs/phase4-cks-mcp-ckg.md`.
- Composer pipeline (the engine behind `get_for_task`): `internal/composer/` and `docs/composer/`.
- Smart Dummy backends (returned when no real ckv/ckg path is configured): `internal/ckvclient/dummy.go`, `internal/ckgclient/dummy.go`.

## 8. Maintaining this doc

This file should be re-checked whenever any of the following changes:

1. `internal/mcp/*.go` adds, renames, or changes a tool's schema. The matching row in §2 and any §3 mapping line updates here.
2. The coding-agent HLD evolves a virtual signature (phase3 §7.x or phase4 §7.x). Add a new mapping row in §3 or amend the existing one.
3. A roadmap item from §5 lands ("rerank" toggle, `include_concurrency`, etc.). Move the row out of §5 and into §3.

When uncertain, run `cks-mcp` against the tools/list MCP method and compare the names there with §2. The wire names are the contract.
