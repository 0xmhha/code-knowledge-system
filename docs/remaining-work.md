# Remaining Work — consolidated, code-verified

> Tier 2 (living index). Single source of truth for what is left. Supersedes the two
> dated snapshots that drifted apart:
> - [`remaining-work-2026-07-10.md`](./remaining-work-2026-07-10.md) (audit E1–E5 / M1–M7)
> - [`session-handoff-2026-07-10.md`](./session-handoff-2026-07-10.md) §4 (P0–P4 priorities)
>
> Those two disagreed on M2 (one said "ready now", the other "blocked by degraded
> serving"); this file resolves the conflict against the code. Keep this file current;
> let the dated files stand as historical record.
>
> **Verified**: 2026-07-12 on branch `docs/retire-ckg-node-id`, cks HEAD `0ee9ceb`.
> Re-confirm each item's evidence before starting — the tree changes fast.

---

## Branch-merge gate — read before merging `docs/retire-ckg-node-id` to main

Two items MUST be resolved (or consciously deferred) before this branch merges:

1. **M1′ — committed local `replace` (`go.mod:41`)**: builds bind to a local ckv path.
   The ckv branch is now pushed (reproducibility restored), but the `replace` +
   local pin must be removed and a proper module pin restored, or CI / other machines
   build against the wrong ckv. **Merge-blocker.**
2. **M6 — the branch's namesake retirement is not in the code.** cks still carries
   11 `ckg_node_id`/`CKGNodeID` references (8 non-test). What actually landed on this
   branch is reindex / alignment / docs — not the retirement. The retirement is gated
   on ckv removing its column first (see [`retire-ckg-node-id.md`](./retire-ckg-node-id.md)).
   Decide explicitly: finish M6 on this branch, or rename the branch's intent and
   land M6 separately.

---

## Consolidated task table

Severity: `[중요]` high / `[권장]` recommended. Status verified against code on 2026-07-12.

| ID | Task | Severity | Status (verified) | Gate / prerequisite |
|---|---|---|---|---|
| **P0** | Reindex `pr-77-2` to recover serving (`reindex-dataset.sh run`, FAMILY=pr-77-2, SRC=vector-db-5). One pass also closes E2, lays down the versioned layout, and activates dual-side digest compare. | [중요] | Not done — serving degraded (vector index removed, per session-handoff §3.5). ckv full build = hours. | Coordinate who runs it (CKV rebuild may be in another session). |
| **M6** | Retire `ckg_node_id` (cks side): drop `Hit.CKGNodeID`, `real.go` mapping, 3 comment sites, JSON-contract note, reflect in `symbol-identity-design.md`. | [권장] | Not done — 11 refs remain. | **ckv column removal first** (retire checklist). |
| **M1′** | Remove committed `replace ckv => ../` and restore a proper module pin. | [중요] | Partial — ckv branch pushed (reproducibility restored); `replace` still at `go.mod:41`. | **Required before main merge**, after M6 stabilizes. |
| **M2** | Run the cks (combined) bench arm — last of the 5 arms. | [권장] | Not done. | **P0 first** (cannot measure a degraded instance). |
| **E4** | `symbol-identity-design.md` §7 — mark Phase 1/2 complete; only remaining is M7. | [권장] | Not done (stale). | Ready now. |
| **E5** | `coordination-response-cks-2026-06-29.md` T1 — note the 2 methods await CKV release. | [권장] | Not done (stale). | Ready now. |
| **M7** | Domain-knowledge anchor `kind:` migration. | [권장] | Not done — 2/43 entry files carry `kind:`, 41 remain (back-compat working). | Ready now (minor). |
| **M3** | T7 — composer causal orchestration (multi-hop `expand_flow`). | [권장] | Not started. | Avoid clashing with M2 measurement freeze. |
| **M4** | Embedding-dimension measurement. | [권장] | Waiting. | External: reindex-B (qwen3) index, CKV-owned. |
| **M5** | Expose `find_invariants` / `get_conventions` as dedicated tools. | [권장] | Partly mitigated (knowledge quota already routes the chunks into the pack). | External: awaiting ckv Engine release. |

**Resolved (no rework):** E1 (source_root corrected), E2 (resolution path fixed),
E3 (instance restarted), M1 (deps resolved via local replace).

**Recommended order:** `P0 → (E4·E5·M7 in parallel) → M2 → M6 + M1′ (before merge) → M3 → (M4·M5 external wait)`.

---

## Evidence pointers (re-verify before acting)

- M6 refs: `internal/ckvclient/real.go:130,135,150`, `pkg/contract/hit.go:27,30,34,44`,
  `pkg/contract/retrievaltrace.go:67` — full retirement checklist in
  [`retire-ckg-node-id.md`](./retire-ckg-node-id.md).
- M1′: `go.mod:41` `replace github.com/0xmhha/code-knowledge-vector => ../code-knowledge-vector`.
- M7: `docs/domain-knowledge/projects/go-stablenet/entries/*.yaml` (43 files, 2 with `kind:`).
- P0 / serving state: [`session-handoff-2026-07-10.md`](./session-handoff-2026-07-10.md) §3.5,
  [`ops-blue-green-reindex.md`](./ops-blue-green-reindex.md).
- Quick resume checks:
  ```bash
  scripts/serve-cks-http.sh status                                 # instance up?
  # then cks.ops.health → serviceable / alignment.ok / builder_version
  FAMILY=pr-77-2 scripts/reindex-dataset.sh status                 # version / lock
  git -C ../code-knowledge-vector status -sb                       # M1′ (ahead?)
  grep -rn "ckg_node_id\|CKGNodeID" --include='*.go' .             # M6 (→ 0 when done)
  ```
