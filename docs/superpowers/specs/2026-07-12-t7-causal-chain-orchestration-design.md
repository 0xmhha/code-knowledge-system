# T7 — Causal-Chain Orchestration (produce→store→consume)

Status: design (approved 2026-07-12). Implementation pending its own plan.
Scope: `code-knowledge-system` only. Prereq: FlowClient (ExpandFlow/GetFlow/
FindBranches) already live (M5). Consumer: coding-agent root-cause-lifecycle
(diagnose).

## 1. Problem & goal

cks synthesizes a token-budgeted EvidencePack but cannot yet answer the
diagnose question at the heart of root-cause-lifecycle: **"where is this value
produced, where is it stored, where is it consumed — so which edge is broken?"**
The flow corpus (via `expand_flow`/`get_flow`) exposes per-step `Reads`/
`Writes`/`Emits`, but nothing assembles those steps into an ordered
produce→store→consume chain. T7 adds that assembly.

Non-goals: precise dataflow analysis (SSA/taint); symptom→seed mapping (already
`find_branches`); any change to ckv/ckg. T7 assembles over the flow corpus's
existing granularity; it does not compute new dataflow.

## 2. Design principle

A **causal chain** is an ordered sequence of flow steps linked by a shared
traced *value*, where a step that `Writes`/`Emits` the value **produces** it, a
step that persists it to durable state **stores** it, and a step that `Reads` it
**consumes** it. Assembly is anchored in the flow corpus (a step/flow), walks
`expand_flow` multi-hop, and matches on the corpus's `Reads`/`Writes`/`Emits`
tokens — best-effort, string-level, explicitly not SSA-precise.

## 3. Seed resolution (a hard constraint)

The flow API resolves a seed only by `flow_id` / `entry_point` / `invariant_id`
(via `GetFlow`) or by `step_id` (via `ExpandFlow`). There is **no reverse index
from an arbitrary code symbol to a flow step.** Therefore:

- Accepted seeds: `step_id`, `entry_point`, `invariant_id`, `flow_id`.
- An arbitrary code symbol reaches a chain only by first mapping to a seed via
  `find_branches` (symptom→branch→step) or an entry point. The builder does NOT
  attempt symbol→step resolution itself.

## 4. Data model (ckvclient-local, mirroring the flow types)

```go
// CausalChain is an ordered produce→store→consume trace of one value across
// flow steps, assembled from the flow corpus.
type CausalChain struct {
    Seed      string       `json:"seed"`             // the entry/invariant/step/flow it was built from
    Value     string       `json:"value,omitempty"`  // the traced Reads/Writes token
    Links     []CausalLink `json:"links"`            // ordered; producer(s) first, consumer(s) last
    Truncated bool         `json:"truncated,omitempty"` // hit max_hops / max_steps
}

// CausalLink is one step in the chain, tagged by its role for the traced value.
type CausalLink struct {
    Role     string            `json:"role"`             // "produce" | "store" | "consume"
    StepID   string            `json:"step_id"`
    Symbol   string            `json:"symbol,omitempty"` // join to ckg via Symbol
    Citation contract.Citation `json:"citation"`
    Reads    string            `json:"reads,omitempty"`
    Writes   string            `json:"writes,omitempty"`
    Emits    string            `json:"emits,omitempty"`
    Relation string            `json:"relation,omitempty"` // "calls" | "called_by" vs the previous link
}
```

Role inference for the traced value `v`: `Writes`/`Emits` contains `v` → the
step **produces** it; a produce whose write target is durable/persistent state
(heuristic: write token names state/store/db/persist) → **store**; `Reads`
contains `v` → **consumes** it. A step matching neither is not a link.

## 5. Algorithm

`internal/composer/causal` package, one exported entry:

```go
func Assemble(ctx, fc ckvclient.FlowClient, req Request) (CausalChain, error)
type Request struct {
    Seed      string  // step_id | entry_point | invariant_id | flow_id
    SeedKind  string  // "step" | "entry" | "invariant" | "flow" (explicit; no guessing)
    Value     string  // optional; when empty, derived from the seed step's Writes/Emits
    Direction string  // "forward" (produce→consume, default) | "backward" | "both"
    MaxHops   int     // default 3
    MaxSteps  int     // default 24 (chain length cap)
}
```

1. **Resolve seed → starting step(s).** `flow/entry/invariant` → `GetFlow` →
   pick the step matching the seed (or the entry step). `step` → use directly.
2. **Choose the traced value.** If `Value` empty, take the seed step's
   `Writes` (else `Emits`). If still empty → return an empty chain (nothing to
   trace) with no error.
3. **Forward trace (produce→consume).** BFS from the producing step via
   `ExpandFlow(stepID, "down", 1)` up to `MaxHops`. A neighbor whose `Reads`
   contains the value → append a `consume` link (with its `Relation`). Continue
   from consumers to find downstream re-produces/consumes.
4. **Backward trace** (`backward`/`both`): `ExpandFlow(stepID, "up", 1)` to find
   the producer of a value the seed consumes.
5. **Assemble & bound.** Order links producer→…→consumer. Track visited
   `step_id`s (cycle-safe). Stop at `MaxHops`/`MaxSteps`; set `Truncated`.
   Deduplicate links by `step_id`.

Failure modes: `FlowClient` returns `ErrFlowUnsupported` (Dummy) → empty chain,
no error (diagnose proceeds, matching the knowledge-tool degradation policy).
A resolve miss (unknown seed) → empty chain + a `reason` on the wire, not an error.

## 6. Interfaces (both, per approval)

**6a. Dedicated MCP tool** — `cks.context.get_causal_chain`:
- args: `seed` (required), `seed_kind` (required; step|entry|invariant|flow),
  `value` (optional), `direction` (optional), `max_hops` (optional).
- handler mirrors `registerGetInvariantEnforcement`: `flowClient(d, name)` →
  `causal.Assemble(...)` → `NewToolResultStructured(chain)`.
- registered in `internal/mcp/server.go`; added to the golden SSoT fixture
  (cks 19 → 20 tools) and the coding-agent C1 schema + analyzer grant
  (coding-agent side, its own PR).

**6b. Composer integration** — diagnose/bugfix intent only:
- a lightweight stage after Stage 3 (or an extension invoked from the pipeline)
  that, for the relevant intents, takes seeds already surfaced (Stage-2 symbols
  that resolve to flow entries, or a `find_branches` result) and calls
  `causal.Assemble` for the top seed(s), capped (e.g. ≤2 chains).
- attaches results to a new `EvidencePack.CausalChains []CausalChain` field
  (omitempty; unchanged for non-diagnose intents).
- respects the token budget: chains are compact (step ids + citations, no
  bodies); the budget allocator counts them like other structured evidence.

## 7. Boundaries & isolation

- `internal/composer/causal` depends only on `ckvclient.FlowClient` +
  `pkg/contract` — testable with the existing `ckvclient.Fake`, no live backend.
- The MCP handler and the composer stage are thin adapters over `Assemble`.
- Value-matching is string containment over the corpus's Reads/Writes/Emits;
  this is documented as best-effort. No new dataflow computation.

## 8. Testing

- `causal` unit tests (Fake FlowClient, canned flows): forward produce→consume
  linking; backward; role inference (produce/store/consume); cycle-safety;
  `MaxHops`/`MaxSteps` truncation; empty-value and unknown-seed → empty chain,
  no error; Dummy → empty, no error.
- MCP handler happy-path + degraded (Fake/Dummy) test.
- Composer stage test: diagnose intent attaches a chain; non-diagnose intent
  leaves `CausalChains` empty; budget accounting unchanged.
- Golden SSoT fixture updated (20 tools); full `go build`/`go test` green.

## 9. Acceptance

- `cks.context.get_causal_chain(seed, seed_kind)` returns an ordered
  produce→consume chain over a live flow; degraded backends return empty + no
  error.
- A diagnose get_for_task attaches `CausalChains` to the EvidencePack.
- Coding-agent analyzer can call the tool (grant + C1 schema updated in the
  coding-agent repo).
- Documented limitation: string-level value matching, flow-corpus granularity,
  seeds must be flow-anchored (symbols via find_branches).

## 10. Out of scope / follow-ups

- SSA/taint-precise dataflow. Symbol→step reverse index (would need a ckv/ckg
  change). Cross-repo flow linking beyond `expand_flow`'s existing cross-flow
  links. Any measurement of the feature (a separate bench task).
