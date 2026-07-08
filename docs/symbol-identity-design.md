# Symbol Identity — single design principle for ckg / ckv / cks

Status: design (Phase 0). This document is the authoritative contract the three
backends (ckg graph, ckv vector, cks composer) and the domain-knowledge anchors
must conform to. Implementation is phased (§7).

> **Follow-up (2026-07-08)**: the `ckg_node_id` shared field is retired in favor of
> `canonical_id` as the sole cross-repo join key (finishes this ADR's migration).
> See [`retire-ckg-node-id.md`](./retire-ckg-node-id.md) for the cross-repo plan,
> the dead-code verdict, and the per-repo checklist.

## 1. Problem

"Symbol" is used inconsistently across the system, with two distinct defects:

1. **The identity is not unique.** ckg stores Go qualified names in *package-leaf*
   form (`validator.defaultSet.QuorumSize`, not the import path) and resolves them
   by **suffix match**. On go-stablenet (a geth fork) this is ambiguous twice over:
   - bare/leaf names flood: `String` ×169, `Close` ×71, `Size` ×25, `Call` ×9 …;
   - even the leaf *package* name collides — real duplicates at different import
     paths: `core` ↔ `consensus/wbft/core` ↔ `signer/core`; `types` ↔
     `core/types` ↔ `beacon/types`; also `params`, `common`, `bn256`, `test`,
     `utils`. So `core.Backend.Size` is ambiguous.
   Resolution then silently returns the first match (`defs[0]`), so a lookup can
   bind to an unrelated same-named symbol.

2. **The `symbol` field is overloaded.** A domain-knowledge anchor
   `{file, symbol, line}` sometimes means "the symbol *defined* at this line"
   (consistent) and sometimes "the *enclosing* function of a statement at this
   line" (inconsistent — e.g. `{symbol: TransitionDb, line: 505}` though
   TransitionDb is defined at 447). The same field carries two meanings.

## 2. Single design principle

> Every code element has one **canonical symbol identity** — globally unique,
> human-readable, and stable across line edits — and that identity, not a short
> name and not a line number, is what the system stores, keys, and resolves on.
> A **file:line** is a *locator* ("where it is right now"), never an identity.

Each value has exactly one role:

| Concept | Role | Stable across line edits? | Used for |
|---|---|---|---|
| **canonical id** | *who* — the identity | yes | node keys, cross-refs, dedup, exact resolution, definition anchors |
| **display name** | *human label* | yes | UI, snippets, FTS short-name search |
| **file:line (citation)** | *where now* — locator | no | jump-to-source, diffs, freshness, location/usage anchors |

## 3. Canonical id specification

**Go** — module-relative import path + receiver + member, receiver-pointer-aware:
- function: `core/vm.IntrinsicGas`
- method: `core/vm.(*EVM).Call`, `common.(Address).String` (keep `*` for pointer receiver)
- type: `consensus/wbft/validator.defaultSet`
- field: `core/types.WBFTExtra.PrevCommittedSeal`
- interface method: `consensus/wbft.ValidatorSet.QuorumSize` (distinct id from each
  concrete impl; linked by an `implements` edge, never merged)
- package-level const/var: `params.MainnetChainConfig` (these MUST be first-class
  symbols; today they are emitted as nodes but resolution drops them)
- generics: declared type-param names, not instantiations: `common/lru.NewBasicLRU[K,V]`

The module prefix `github.com/ethereum/go-ethereum/` is dropped (single module);
the package import path is module-relative.

**Solidity** — directory path (to separate versions) + contract + member +
parameter-type signature (to separate overloads):
- `systemcontracts/solidity/v2/GovMinter.mint(address,uint256)`
- `systemcontracts/solidity/abstracts/GovBase.approveProposal(uint256)`

**TS/proto** — module/file-path qualified analogously (`<module>.<Class>.<member>`,
`proto:<package>.<Message>`). They have no import-path concept, so the file/package
path is the qualifier.

**Edge cases the form encodes:** method vs func (receiver in id), pointer vs value
receiver (`*`), interface vs concrete (separate ids + `implements`), generics
(type-param names), embedded/promoted (index the real declarer once; promotion is
an edge), Solidity overloads & versions (param signature + dir).

**Resolution rule:** resolve by **exact** canonical id. A short/leaf name MAY be
offered as a convenience search, but a lookup that resolves to **more than one**
node is an **error** for the traversal family (find_callers/get_subgraph/
impact_analysis), never a silent `defs[0]`.

## 4. Two anchor kinds (domain knowledge)

Replace the overloaded `symbol` with an explicit kind:

- **definition anchor** (`kind: def`) — points at a symbol's declaration.
  `{ kind: def, symbol: <canonical id, resolves to exactly one node>, file, line }`.
  Invariant: `line` == the symbol's current declaration line. Refreshable: resolve
  `symbol` → current line (this is what `cks-anchor-refresh` does, now exact).
  Optional `signature_hash` detects semantic change independent of line moves.

- **location/usage anchor** (`kind: loc`) — points at a statement *inside* a symbol.
  `{ kind: loc, file, line, enclosing_symbol: <canonical id>, reason, snippet_hash? }`.
  No `line == definition` rule. Refresh = re-locate the enclosing symbol, then the
  statement (by `snippet_hash`); never repoint to the definition line. The
  `TransitionDb line:505` sender-check and the `"ValidateTransaction (Berlin gate)"`
  cases become these; descriptive strings move from `symbol` into `reason`.

`file` remains the only hard-required field (back-compat). `kind` defaults to `def`.

## 5. Per-module roles & responsibilities

- **ckg (produces identity).** Compute the canonical id at *both* definition-emit
  and call/edge-resolution **in lockstep** (use `*types.Object.Pkg().Path()`, not
  `Pkg().Name()`). Store it as a new `canonical_id` column; keep `qualified_name`
  (leaf) + `name` for FTS/display. Node ID derives from canonical id. Promote
  package const/var and interface methods to resolvable nodes. Expose canonical id
  on every node/citation payload.
- **ckv (consumes positionally).** Alignment to ckg stays **positional**
  (file + start/end line) — it never used names, so it never inherited the
  ambiguity. Add an **additive** `canonical_id` to Chunk + Hit (omitempty),
  populated from the aligned ckg node. Do **not** change the embed-text prefix →
  **no re-embed**. Keep file:line as the primary ckv↔ckg join key, with
  `ckg_node_id` as the machine cross-ref.
- **cks (resolves + anchors).** Resolve by exact canonical id; treat a multi-match
  as an error for the traversal family (drop the `defs[0]` fallback in
  `resolveQname`/`resolveNodeID`/`resolveSeedFile`). Fix the MCP tool docs (they
  advertise a `consensus.wbft.Finalize` form ckg does not store). Adopt the two
  anchor kinds in the entry schema; teach `cks-anchor-refresh` to enforce
  `line==def` for `def` and range-containment for `loc`; give inventory validation
  a ckg handle to assert each `def` symbol resolves uniquely; render per kind in
  `domainexport`. **Composer and `contract.Citation` stay file:line — unchanged.**

## 6. Migration

- **ckg:** changing the id that feeds `MakeID` churns every node ID → **full
  reindex** (graph rebuild is LLM-free, minutes) + schema migration (add
  `canonical_id`). Definition-emit and edge-resolution must change together or all
  `calls`/`implements`/`uses_type` edges silently drop.
- **ckv:** additive field, in-place via the existing migration runner + reparse;
  **no re-embed** if the embed prefix is untouched. Manifest stays additive.
- **cks / data:** migrate the 39 go-stablenet entries (146 symbol+line anchors;
  ~25 reclassify to `loc`/`enclosing_symbol`; descriptive symbol strings → `reason`).
  `pkg/contract.Citation`, the composer, and MCP response payloads are unaffected.

## 7. Phases (each: implement → impact-check → test → PR)

- **Phase 0** — this design doc (the shared contract).
- **Phase 1 — ckg canonical id.** Add `canonical_id` (import-path + receiver/sig
  aware) at emit + resolution in lockstep; exact resolution; promote const/var +
  interface methods; full reindex; keep leaf qname for display/FTS. Acceptance:
  `find_callees`/`find_callers` collisions gone (e.g. `core.Backend.Size` resolves
  to exactly one), edge counts stable or higher, existing resolver tests pass +
  new collision tests.
- **Phase 2 — ckv canonical field.** Additive `canonical_id` on Chunk + Hit via
  positional alignment; no re-embed. Acceptance: every aligned chunk carries the
  ckg canonical id; manifest/migration in place; vectors byte-identical.
- **Phase 3 — cks resolution + anchors.** Exact resolution + multi-match error; MCP
  doc fix; two-kind anchor schema; `cks-anchor-refresh`/inventory/domainexport
  updates; migrate the 39 entries. Acceptance: `cks-inventory-check` enforces unique
  `def` resolution; anchor refresh has zero false REVIEW for `def`, deterministic
  `loc` handling; composer outputs unchanged.

## 8. Non-goals / what stays the same
- `pkg/contract.Citation` (file:line) and the entire composer pipeline.
- ckv embeddings (no re-embed) and ckg↔ckv positional alignment.
- The short/leaf display name and the convenience suffix search remain — they just
  stop being the *resolution identity*.
