# go-stablenet Knowledge System — Context Map (for grounded, multi-perspective review)

**Purpose.** This document is the shared context that lets an LLM (and a human
reviewer) make *grounded, consistent, multi-perspective* decisions when changing
**go-stablenet** (a geth fork: WBFT BFT consensus, WKRC native stablecoin,
governance system contracts). It maps the five repos that together turn a
requirement into a verified change, inventories the domain knowledge available
for decisions, catalogs the review perspectives and where each one's knowledge
lives, and lists the gaps and risks that still undermine determinism.

Baseline: domain-knowledge anchors verified at go-stablenet commit `9978930ba` (#84);
live checkout HEAD is `d7cff3df9` (#86) — see Staleness, §6. (Note: `project.yaml`
records the baseline as `ec6a4e96b`, which does not resolve in the current checkout —
reconcile before trusting line numbers.)

---

## 1. The system at a glance

```
                 go-stablenet (target code: WBFT, WKRC, gov contracts)
                            │  parsed / embedded into datasets
        ┌───────────────────┼────────────────────┐
        ▼                   ▼                     ▼
   ckg (graph)         ckv (vector)         domain-knowledge
   AST→SQLite graph    bge-m3 embeddings    43 entries / 40 verified (A1–A14)
   symbols + relations semantic recall      invariants/pitfalls/anchors
        └───────────────────┴────────────────────┘
                            │  fused in-process
                            ▼
                    cks (composer / MCP)
        get_for_task → EvidencePack (budgeted, sanitized, integrity-stamped)
        + relational tools (find_callers/callees/subgraph/impact)
                            │  MCP (planner only)
                            ▼
                 coding-agent (Claude Code plugin)
   planner → implementer → evaluator → orchestrator (state.json, resumable)
                            │  evaluator drives
                            ▼
                    chainbench (test harness)
        boots a local WBFT network, runs consensus/regression/hardfork tests
```

**Design rule (coding-agent):** *Binary = deterministic, Session = LLM.* The
backends (ckg/ckv/cks/chainbench) do deterministic work; all reasoning lives in
the plugin's agents. The point of the datasets is to make that reasoning
*grounded* (retrieve over an unfamiliar fork instead of guessing) and
*resumable* (every stage writes a disk artifact).

---

## 2. The five components

### 2.1 ckg — code-knowledge-graph (structural / relational)
- **Role:** parses Go/Solidity/Proto/TS into a persistent **SQLite graph** (35 node
  types, 40 edge types; schema v1.10). go-stablenet graph ≈ **210K nodes / 708K
  edges**. Node ID = `sha256(qname|lang|startByte)[:16]` (deterministic).
- **Queries (via cks):** find_symbol, find_callers (reverse BFS), find_callees
  (forward), get_subgraph, impact_analysis (6 reverse buckets, depth≤5,
  deterministic sorted output), change_history (commit/hunk, file-level).
- **Key files:** `pkg/types/{node,edge,enums}.go`, `internal/persist/schema.sql`,
  `internal/buildpipe/pipeline.go`, `internal/parse/golang/resolve.go`, `pkg/impact/impact.go`.
- **Accuracy / gaps:** two-pass call resolution; **recently fixed** to qualify call
  targets by package+receiver type (forward-edge name collisions removed — PR #17).
  Still **V0**: unresolved cross-pkg/dynamic calls dropped (reverse-edge
  under-recall); **no data-flow edges** (control flow is statement-granular only);
  file-level (not line-level) blame; `awaits`/`overrides`/`retry_path` reserved but
  unemitted. Incremental builds skip lock/security enrichment (need `--no-cache`).

### 2.2 ckv — code-knowledge-vector (semantic)
- **Role:** embeds code + docs + domain corpus into `sqlite-vec`; serves semantic
  search. Production model **bge-m3 via Ollama, 1024-dim**. go-stablenet ≈ **26,015
  chunks** (22.1K symbol, 2.2K header, 1.3K doc, 207 convention, 163 invariant).
- **What's embedded:** one chunk per function/method/type, file headers, markdown/ADR
  doc sections, the rendered domain-knowledge corpus, optional PR/commit history;
  Anthropic contextual-prefix on chunks. Policy `policy/stablenet.yaml` attaches
  category + `also_review`/`required_tests`/`watch_out` guidance to every hit.
- **Key files:** `cmd/ckv/build.go`, `internal/store/sqlitevec/store.go`,
  `internal/chunk/prefix.go`, `internal/embed/registry/registry.go`, `policy/stablenet.yaml`.
- **Cost / gaps:** full bge-m3 embed ≈ **10 hours** (re-index is expensive and coupled
  to one Ollama model; vector dimension baked into the DDL). Eval fixture is tiny
  (N=10). 6 newer ckv tools not yet wired into cks; rerank is a stub.

### 2.3 cks — code-knowledge-system (composer / MCP hub)
- **Role:** fuses ckg + ckv + domain knowledge into a token-budgeted, sanitized,
  SHA-256-stamped **EvidencePack**, exposed over MCP. Calls no LLM itself.
- **`get_for_task` pipeline** (`internal/composer/composer.go`): intent classify
  (ckv embed + cosine anchors) → Stage 1 keyword extract (glossary vocab + ckv
  recall → ckg BM25 rerank) → Stage 2 citation search (ckg BM25 + FindSymbol + ckv
  list, fused by **RRF**, CkvWeight 5.0 > Symbol 1.5 > BM25 1.0) → Stage 3 graph
  expansion (ckg Neighbors, intent-shaped) → Stage 4 budget (8000 tok, ≤12
  citations) → Stage 5 sanitize (`policies/sanitization_rules.yaml`).
- **MCP surface (13):** `cks.context.*` — get_for_task, semantic_search, search_text,
  find_symbol, find_callers, find_callees, get_subgraph, impact_analysis,
  change_history, concurrency_impact; `cks.ops.*` — health, freshness, index.
- **Domain-knowledge subsystem:** `docs/domain-knowledge/` (schema
  `shared/entry.schema.yaml`, lifecycle `shared/STATUS_LIFECYCLE.md`). `cks-domain-sync`
  derives ckv/ckg policy views from **verified** entries; `internal/domainexport`
  renders entries → markdown corpus for `ckv build --docs`; `cks-glossary-gen` builds
  the alias glossary feeding the vocab resolver.
- **Gaps (from the 4-way eval, `eval/ckg-4way/`):** `get_for_task` (δ) has the best
  answer pass-rate but the **most fabricated line-range citations** (≈1 per 3
  questions) and weak location precision → consumers must validate citations against
  pack headers. Tuning constants are still a "Phase E baseline."

### 2.4 coding-agent — the Claude Code plugin (the only LLM-bearing piece)
- **Pipeline:** `TICKET_INTAKE → ANALYSIS → PLANNING → DESIGN → IMPLEMENTATION →
  EVALUATION → COMPLETION` (+ bugfix re-entry ≤3, code_review/release variants),
  state in `state.json` + artifacts; resume logic in `skills/state-machine`.
- **Agents:** `orchestrator` (state machine, PR, Jira, MCP pre-flight), `planner`
  (**sole cks consumer**, 4 modes), `implementer` (branch isolation, per-step commit,
  builds `gstable`), `evaluator` (4-stage gate, **sole chainbench consumer**).
- **Domain backstop:** `skills/stablenet-invariants` (always-on byzantine-fairness:
  equal-power quorum, instant finality / inert reorg, base-fee redistribution).
- **Consumption:** planner uses `cks_ops_health/freshness/index` + `get_for_task` +
  relational tools; evaluator drives chainbench at Stage 4; jira-gateway scrubs inbound
  Jira. Tool surface frozen in `contract/agent-mcp.schema.json`.
- **Status note:** HANDOFF lists "domain KB empty / 0 verified" as P0 — **this is
  stale**; the corpus is now 43 entries / 40 verified (§5). Real remaining items: a full
  end-to-end `/work`→`/merge` run has not been executed; retrieval silently degrades
  to a dummy embedder if Ollama/bge-m3 is absent.

### 2.5 chainbench — validation harness
- **Role:** boots a local WBFT network (default 4 validators + 1 endpoint, 1s blocks,
  static preset keys) and runs `tests/{basic,fault,stress,regression,hardfork}`;
  exposes ~38 MCP tools (lifecycle, node, test, consensus, network, tx/read).
- **Domain encoded:** BFT quorum `floor(2N/3)+1` (`lib/consensus_calc.sh`), WBFT health
  via `istanbul_*` RPCs, stablenet genesis (WKRC adapter + gov contracts 0x1000–0x1004,
  BLS keys), hardfork profiles (`bohoBlock`).
- **Gotchas (reproducibility footguns):** `profiles/regression.yaml` hardcodes an
  absolute `binary_path` from another machine (must override); the default profile funds
  only validators+1 endpoint, so regression test accounts are funded only under the
  regression profile; lifecycle still shells out from bash (timing flakiness).

---

## 3. End-to-end decision flow (how a requirement becomes a grounded change)

1. **Intake** (orchestrator): Jira/free-text → ticket.json (sensitive-info scrub).
2. **Analysis/Planning/Design** (planner): `cks_ops_health` → `get_for_task(prompt)` →
   EvidencePack (ckv semantic recall + ckg citations/graph + domain-knowledge guidance,
   budgeted & sanitized). Planner adds relational depth with `find_callers`/
   `impact_analysis` and consults `stablenet-invariants` so the design cannot violate
   byzantine-fairness. Output: `analysis.md`, `related-code.json`, `plan.md`, `design.md`.
3. **Implementation** (implementer): isolated branch, one commit per atomic step, builds
   `gstable`, records the binary path in `state.json`.
4. **Evaluation** (evaluator): unit/lint/security gates, then **chainbench** boots the
   built binary, watches block production, runs the relevant test suite, parses the JSON
   report; failures re-enter the bugfix cycle (≤3).
5. **Completion** (orchestrator): PR + Jira sync (merge is a separate, guarded command).

**Determinism levers along this path:** deterministic node IDs and sorted graph output
(prompt-cache-safe); verified domain entries that pin invariants/anchors; the always-on
invariants skill; citation budgets + integrity stamps. **Determinism leaks:** silent
degraded embedder, δ citation fabrication, anchor staleness, ckg under-recall on dynamic
dispatch (see §6).

---

## 4. Decision-perspective catalog (the lenses, and where each is grounded)

For every change, these are the perspectives a reviewer must apply. "Source" = where
the grounding knowledge lives today; "Gap" = what is missing for confident review.

| Perspective | Grounding source (today) | Gap |
|---|---|---|
| **Code flow / sequence** | ckg find_callers/callees/subgraph/impact; `wbft-consensus.md` | reverse-edge under-recall on dynamic dispatch; no data-flow edges |
| **Go language traits** (compiled, GC, concurrency) | A1.concurrency.core_lock_discipline; ckg concurrency edges (spawns/sends_to/acquires_lock); `concurrency_impact` | only lock discipline + reorg serialization; no channel/goroutine-lifecycle, context-cancellation, map-race, slice-aliasing entries |
| **Consensus safety & liveness** | A1 (×5), A14 theory (×9: 3f+1, FLP, equivocation, justification-locking), `stablenet-invariants` skill | no proposer-selection liveness / timeout-backoff entry; no backlog/future-message buffering entry |
| **Cryptography (BLS, sig schemes)** | A12 seals (BLS seal layout, feepayer sighash) | no BLS aggregation/verification math, key-gen, rogue-key/proof-of-possession, malleability entry |
| **Distributed networking & protocol** | A9 (istanbul subprotocol, RPC ref); ckg Endpoint/MessageType edges | no devp2p/RLPx handshake, peer discovery, gossip fan-out, eclipse/DoS entry |
| **EVM / state** | A5 (account-extra, native coin), A11 (blacklist check-points) | no EVM execution, gas metering internals, state-trie/commit/snapshot, precompile entry |
| **Gas / economics** | A6 (Anzeon tip, fee delegation), A14 base-fee redistribution | no full EIP-1559 base-fee math, tx ordering/priority, fee-market invariants |
| **Governance / system contracts** | A4 (addresses, GovMinter, GovMasterMinter/mint-proposal, GovValidator genesis+storage, storage-slot helpers), A10 codegen no-edit-zones; chainbench genesis | no GovCouncil / NativeCoinAdapter internals; no per-contract upgrade storage-slot pitfalls |
| **Security vulnerabilities** | A14 equivocation/justification; ckg Solidity security markers | no reorg-griefing, replay across fee-delegation, blacklist-bypass surface, gas/balance overflow entry |
| **Testing / validation** | chainbench suites + MCP; coding-agent evaluator §7; `A1.testing.consensus_change_validation` (needs_verification) | entry not yet verified → not shipped to the index; no separate regression-account / chainbench-harness reference entry |

---

## 5. Domain-knowledge corpus (the "learning" dataset)

- **43 entries** (40 `status: verified` + 3 `needs_verification`), under
  `code-knowledge-system/docs/domain-knowledge/projects/go-stablenet/entries/`.
  The 3 unverified (not yet shipped to the index): `A1.testing.consensus_change_validation`,
  `A4.gov_validator.storage_and_governance`, `A11.txpool.type_taxonomy_admission`.
- **Schema** (`shared/entry.schema.yaml`): id, subsystem (A1–A14), knowledge_type
  (B1 architecture, B2 data-structure, B3 algorithm/flow, B4 invariant, B5 pitfall,
  B6 procedure, B7 reference), title, summary, status, priority, plus code_anchors,
  code_keywords, korean/english_aliases, invariants, pitfalls, constants, related_concepts,
  last_verified_at, verified_by.
- **Lifecycle** (`shared/STATUS_LIFECYCLE.md`): needs_author → draft → needs_verification
  → verified; **only verified entries ship to the ckv index**; an anchor file changing
  reverts an entry to needs_verification.
- **Coverage** (strongest → thinnest): A14 protocol theory (9), A1 WBFT core (6) and
  A4 gov contracts (6) and A8 genesis (3) are strong; **A11 tx/state-transition (2),
  A9 p2p (2), A7 hardfork (1), A13 sealing (1)** are thin relative to their scope.
- **First-party authoritative docs** (cross-referenced, not duplicated): go-stablenet
  `CLAUDE.md` and `.claude/docs/{wbft-consensus, stablenet-features, system-contract-flow,
  build-source-files, review-guide, code-convention, dev-basics, ops-guide}.md`. (Note:
  these are real indexed files — citations to them are valid, not hallucinations.)

---

## 6. Cross-cutting risks to determinism (fix these to stop "wrong-direction" work)

1. **Anchor staleness (highest priority).** Entries verified @`9978930ba` (#84); HEAD is
   `d7cff3df9` (#86). The recorded baseline also conflicts across files (`project.yaml`
   `ec6a4e96b` is unresolvable; dashboards cite `9978930ba`; the ckg/ckv DBs were built
   @`c051d50b` #85). Per the lifecycle's own rule, any touched anchor file makes a
   "verified" entry potentially wrong. **Re-verify anchors against current HEAD before
   trusting line numbers**, and re-stamp `code_commit`.
2. **ckg reverse-edge under-recall.** Suffix-match resolution drops unresolved
   cross-package/dynamic-dispatch calls → `impact_analysis` can miss affected sites
   (the change-impact study measured ~0.77 recall after the call-target fix; better than
   grep's 0.40 but not complete). No data-flow edges → semantic/data-coupling impacts
   (e.g. account-extra bit readers) are invisible to all retrieval modes.
3. **δ (`get_for_task`) citation fabrication.** Best answer quality, but invents ~1
   out-of-range line citation per 3 questions. Validate citations against pack headers;
   prefer `find_callers`/`impact_analysis` for "what breaks if I change X".
4. **Silent degraded mode.** If Ollama/bge-m3 is absent, ckv falls back to a dummy
   embedder and the pipeline continues at lower quality (`cks_ops_health: degraded`).
   Treat `degraded` as a hard stop for high-stakes changes.
5. **ckv re-index cost (~10h).** Adding domain entries only becomes semantically
   searchable after a full bge-m3 re-embed; the ckg policy view updates cheaply. Plan
   batched re-indexing.
6. **chainbench reproducibility footguns.** Hardcoded `binary_path` in
   `profiles/regression.yaml`; profile-scoped account funding.

---

## 7. Phase 2 backlog — domain-knowledge gap-fill ("learning"), prioritized

Authoring order chosen so the highest-leverage, most decision-blocking knowledge lands
first. Each new entry follows the schema (B-type, anchors against **current HEAD**,
invariants/pitfalls, aliases) and must reach `verified` before it ships to the index.

**P0 — integrity first**
- Re-verify the 43 existing entries' anchors against current HEAD; re-stamp
  `code_commit`. (Knowledge you already trust must be correct before adding more.) Also
  resolve the 3 `needs_verification` entries (see §5) and the baseline-commit conflict
  (project.yaml `code_commit` does not resolve in the current checkout).

**P0 — fill decision-blocking gaps**
- A11 tx/state-transition: tx-type taxonomy & mempool admission *(drafted — `A11.txpool.type_taxonomy_admission`, needs_verification)*; still missing state-transition gas
  flow; intrinsic gas (incl. EIP-3860). (Most under-covered vs scope.)
- Testing/validation: a B6 entry on validating consensus/state changes with chainbench
  *(drafted — `A1.testing.consensus_change_validation`, needs_verification)*.
- Governance: GovValidator + GovMasterMinter *(now covered, A4)*; still missing GovCouncil,
  NativeCoinAdapter internals + per-contract upgrade storage-slot pitfalls.

**P1 — perspective completeness**
- Cryptography: BLS aggregation/verification & rogue-key/PoP defense.
- Distributed networking: devp2p/RLPx handshake, discovery, broadcast fan-out, eclipse/DoS.
- Consensus liveness: proposer-selection/timeout backoff; backlog/future-message buffering.
- Gas/economics: EIP-1559 base-fee update math; tx ordering/priority.
- Go concurrency: channel/goroutine lifecycle, context cancellation, map-race, slice-aliasing.

**P2 — security depth**
- Reorg-griefing, replay across fee-delegation, blacklist-bypass surface, gas/balance overflow.

> Activation: new/updated entries → `cks-domain-sync` (ckv/ckg policy views) →
> `domainexport` corpus → ckv re-embed (batched, ~10h) + ckg policy refresh.
</content>
