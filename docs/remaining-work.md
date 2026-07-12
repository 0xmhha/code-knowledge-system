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
> **Verified**: 2026-07-12 on branch `docs/retire-ckg-node-id`.
> Re-confirm each item's evidence before starting — the tree changes fast.

---

## Branch-merge gate — read before merging `docs/retire-ckg-node-id` to main

Both former merge-blockers are now resolved on this branch (2026-07-12):

1. **M6 ✅ — retirement is in the code.** ckv `origin/main` (`7f62683`) already dropped
   the `CKGNodeID` field; cks dropped `Hit.CKGNodeID` + the `real.go` mapping + comment
   sites + the b7-test observation. `grep ckg_node_id|CKGNodeID` → only a prose comment
   documenting the retirement. Build + tests clean.
2. **M1′ ✅ — go.mod pinned, no replace.** Local `replace ckv => ../` removed; ckv pinned
   to the column-removed `origin/main` (`v0.0.0-20260712000512-7f6268307669`). Reproducible
   on CI / other machines.

Remaining before/after merge: **rebase on main** (1 docs-only commit `#32`), and the
**dataset must be rebuilt with a `schema_version` bump** so the served index no longer
carries the dropped column (retire checklist "완료 게이트"). cks-seminar deck/asset sync
lives in that separate repo.

---

## Consolidated task table

Severity: `[중요]` high / `[권장]` recommended. Status verified against code on 2026-07-12.

| ID | Task | Severity | Status (verified) | Gate / prerequisite |
|---|---|---|---|---|
| **P0** | Reindex `pr-77-2` to recover serving (`reindex-dataset.sh run`, FAMILY=pr-77-2, SRC=vector-db-5). One pass also closes E2, lays down the versioned layout, and activates dual-side digest compare. | [중요] | Not done — serving degraded (vector index removed, per session-handoff §3.5). ckv full build = hours. | Coordinate who runs it (CKV rebuild may be in another session). |
| **M6** | Retire `ckg_node_id` (cks side): drop `Hit.CKGNodeID`, `real.go` mapping, comment sites, JSON-contract note, reflect in `symbol-identity-design.md`. | [권장] | ✅ Done (2026-07-12) — build + tests clean. Dataset schema-bump reindex still needed to drop the served column. | — |
| **M1′** | Remove committed `replace ckv => ../` and restore a proper module pin. | [중요] | ✅ Done (2026-07-12) — ckv pinned to `7f6268307669` (origin/main). | — |
| **M2** | Run the cks (combined) bench arm — last of the 5 arms. | [권장] | Not done. | **P0 first** (cannot measure a degraded instance). |
| **E4** | `symbol-identity-design.md` §7 — mark Phase 1/2 complete; only remaining is M7. | [권장] | ✅ Done (2026-07-12). | — |
| **E5** | `coordination-response-cks-2026-06-29.md` T1 — note the 2 methods await CKV release. | [권장] | Not done (stale). | Ready now. |
| **M7** | Domain-knowledge anchor `kind:` migration. | [권장] | Not done — 2/43 entry files carry `kind:`, 41 remain (back-compat working). | Ready now (minor). |
| **M3** | T7 — composer causal orchestration (multi-hop `expand_flow`). | [권장] | Not started. | Avoid clashing with M2 measurement freeze. |
| **M4** | Embedding-dimension measurement. | [권장] | Waiting. | External: reindex-B (qwen3) index, CKV-owned. |
| **M5** | Expose `find_invariants` / `get_conventions` as dedicated tools. | [권장] | Partly mitigated (knowledge quota already routes the chunks into the pack). | External: awaiting ckv Engine release. |

**Resolved (no rework):** E1 (source_root corrected), E2 (resolution path fixed),
E3 (instance restarted), M1 (deps resolved via local replace), **M6 + M1′ + E4
(2026-07-12, this branch)**.

**Recommended order:** land this branch (rebase + PR) → `P0 → (E5·M7 in parallel) → M2 → M3 → (M4·M5 external wait)`.

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
