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
> **Verified**: 2026-07-12 on `main` (after PR #33 squash-merge).
> Re-confirm each item's evidence before starting — the tree changes fast.

---

## ckg_node_id retirement — code done, data-side still open

The retirement landed in code via **PR #33 (squash-merged to main, 2026-07-12)**:

1. **M6 ✅ — code retirement merged.** ckv `origin/main` (`7f62683`) dropped the
   `CKGNodeID` field; cks dropped `Hit.CKGNodeID` + the `real.go` mapping + comment
   sites + the b7-test observation. `grep ckg_node_id|CKGNodeID` → only a prose comment.
2. **M1′ ✅ — go.mod pinned, no replace.** ckv pinned to the column-removed `origin/main`
   (`v0.0.0-20260712000512-7f6268307669`). Reproducible on CI / other machines.

**Still open — the data side (see `M6-data` in the table).** PR #33 closed the *code*
side of ADR-0001. The *served* pr-77-2 vector index was built by the **old** ckv, so it
**still physically carries the `ckg_node_id` column**. It only disappears when the index
is rebuilt with the new ckv — the same pass as P0. This is the ADR's last mile, tracked
explicitly below so "serving recovered" is not mistaken for "column dropped".

Also separate: cks-seminar deck/asset `ckg_node_id`→`canonical_id` sync (that repo).

---

## Consolidated task table

Severity: `[중요]` high / `[권장]` recommended. Status verified against code on 2026-07-12.

| ID | Task | Severity | Status (verified) | Gate / prerequisite |
|---|---|---|---|---|
| **P0** | Reindex `pr-77-2` to recover serving (`reindex-dataset.sh run`, FAMILY=pr-77-2, SRC=vector-db-5). One pass also closes E2, lays down the versioned layout, activates dual-side digest compare, **and drops the served `ckg_node_id` column (M6-data)**. **Acceptance:** (1) health `serviceable=true` + `alignment.ok`; (2) index built by ckv `7f62683`+ (check `builder_version`) so the served schema no longer carries `ckg_node_id`. | [중요] | Not done — serving degraded (vector index removed, per session-handoff §3.5). ckv full build = hours. | Coordinate who runs it (CKV rebuild may be in another session). |
| **M6-data** | ADR-0001 data-side close: the served pr-77-2 index must be rebuilt by the column-removed ckv so `ckg_node_id` is physically gone. **Failure mode:** if the P0 reindex runs with a stale ckv binary (or resumes a pre-retire checkpoint), serving comes back green while the dead column persists — so verify the built ckv version, not just that serving is up. | [권장] | Not done — served index still carries the column (built by old ckv). Harmless (dead column) but ADR not closed end-to-end. | **Achieved by P0 iff P0 uses new ckv** — verify, don't assume. |
| **M6** | Retire `ckg_node_id` (cks code side): drop `Hit.CKGNodeID`, `real.go` mapping, comment sites, JSON-contract note, reflect in `symbol-identity-design.md`. | [권장] | ✅ Done (PR #33, 2026-07-12) — build + tests clean. Data side tracked as `M6-data`. | — |
| **M1′** | Remove committed `replace ckv => ../` and restore a proper module pin. | [중요] | ✅ Done (PR #33, 2026-07-12) — ckv pinned to `7f6268307669` (origin/main). | — |
| **M2** | Run the cks (combined) bench arm — last of the 5 arms. | [권장] | Not done. | **P0 first** (cannot measure a degraded instance). |
| **E4** | `symbol-identity-design.md` §7 — mark Phase 1/2 complete; only remaining is M7. | [권장] | ✅ Done (2026-07-12). | — |
| **E5** | `coordination-response-cks-2026-06-29.md` T1 overstated the 2 knowledge tools as shipped with the flow-4. | [권장] | ✅ Done (2026-07-12) — added a dated correction: find_invariants/get_conventions shipped separately via M5 (cks #34 + ckv facade #35), so T1's 6 tools are now all exposed. | — |
| **M7** | Domain-knowledge anchor `kind:` migration (def vs loc). | [권장] | **Deferred — needs the source-of-truth commit.** ~150/164 anchors are def (back-compat correct, no change); only a handful are loc. Accurate def/loc classification = "is `line` the declaration of `symbol`?", which must be checked against go-stablenet **at the commit the entries were authored against** (line numbers drift). The reason-text heuristic is unreliable — it cannot distinguish "def of X" from "loc using X" and produces false positives (e.g. `NativeCoinManagerAddress:219` reads as loc but is a def; `ExtractWBFTExtra:251` names the *called* symbol, not the enclosing one). Blind bulk editing would corrupt curated knowledge. | Pin the authoring go-stablenet commit, then do a source-verified pass. Back-compat working meanwhile — no functional issue. |
| **M3** | T7 — composer causal orchestration (multi-hop `expand_flow`). | [권장] | Not started. | Avoid clashing with M2 measurement freeze. |
| **M4** | Embedding-dimension measurement. | [권장] | Waiting. | External: reindex-B (qwen3) index, CKV-owned. |
| **M5** | Expose `find_invariants` / `get_conventions` as dedicated tools. | [권장] | 🔶 Wired (cks PR #34 + ckv facade PR #35, repin #35, 2026-07-12): FlowClient + MCP tools `cks.context.find_invariants`/`get_conventions`, build+test green. **Remaining:** coding-agent diagnose e2e (1 call over a live cks-mcp) — pending P0 serving recovery. | Code done; e2e blocked on P0. |

**Resolved (no rework):** E1 (source_root corrected), E2 (resolution path fixed),
E3 (instance restarted), M1 (deps resolved via local replace), **M6 + M1′ + E4
(2026-07-12)**, **M5 code/wiring (2026-07-12, PR #34/#35; only the e2e remains)**.

**Recommended order:** `P0 (incl. M6-data acceptance) → M2 → M3 → M5 e2e → (M4 external wait; M7 pending the authoring go-stablenet commit)`.
P0 is the critical path (it gates M2, the headline goal, and closes M6-data); start it first
and fill the build wait with E5·M7.

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
