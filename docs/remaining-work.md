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

**Data side — ✅ closed (2026-07-12).** PR #33 closed the *code* side of ADR-0001; the
*served* index is now `pr-77-gstable/vector-db`, built by the column-removed ckv, so the
`ckg_node_id` column is physically gone (verified: served binary `cks-mcp/0.1.0-90dc885d`,
`serviceable:true`). The stale-binary failure mode fired on the first cutover and was caught
fail-loud — see `M6-data` in the table.

Also separate: cks-seminar deck/asset `ckg_node_id`→`canonical_id` sync (that repo).

---

## Consolidated task table

Severity: `[중요]` high / `[권장]` recommended. Status verified against code on 2026-07-12.

| ID | Task | Severity | Status (verified) | Gate / prerequisite |
|---|---|---|---|---|
| **P0** | Serve the current dataset with a fresh, aligned index. | [중요] | ✅ Done (2026-07-12) via **cutover, not a fresh reindex** — the docs' premise was stale (see note below). Cut serving over to `pr-77-gstable` (already built by another session: column-removed ckv + sources ledger, ckg schema 1.23, commit `0bf2f4d1b`). Verified: `serviceable:true`, `alignment.ok:true` (digest actual==expected, source_root_ok), `builder_version cks-mcp/0.1.0-90dc885d`. | — |
| **M6-data** | ADR-0001 data-side close: the served index must be built by the column-removed ckv so `ckg_node_id` is physically gone. | [권장] | ✅ Done (2026-07-12) — served index (`pr-77-gstable/vector-db`) has no `ckg_node_id` column, and the serving binary was rebuilt from column-removed main. The predicted failure mode fired and was caught: the first cutover with a **stale `bin/cks-mcp`** hit `ckv.Open: no such column: ckg_node_id → degraded` (fail-loud); rebuilding `bin/cks-mcp` from current main resolved it. | — |
| **M6** | Retire `ckg_node_id` (cks code side): drop `Hit.CKGNodeID`, `real.go` mapping, comment sites, JSON-contract note, reflect in `symbol-identity-design.md`. | [권장] | ✅ Done (PR #33, 2026-07-12) — build + tests clean. Data side tracked as `M6-data`. | — |
| **M1′** | Remove committed `replace ckv => ../` and restore a proper module pin. | [중요] | ✅ Done (PR #33, 2026-07-12) — ckv pinned to `7f6268307669` (origin/main). | — |
| **M2** | Run the cks bench arm (cks-bench live, `pr-77-gstable`, 30 golden Qs). | [권장] | ✅ Done (2026-07-12). **Result: cks retrieval ~doubles correctness vs the fair (grep-only) baseline.** vs `M1_fair` (grep-only, 33.3% correct, 12,883 tok, 0 halluc): cks auto-pack `M4_get_for_task` = 70.0% correct / 4,899 tok (−62%); `M3_incremental` = 73.3% / 1,780 tok; `M2_graph_full` = 56.7% / 35,891 tok. Oracle ceiling `M1_raw` (golden files injected) = 93.3%. **Caveat:** cks methods hallucinate more (M3=19, M4=16) than grep-only (0) — needs root-cause. Report: `coding-agent/bench/cks-bench/runs/ckg-bench-live-{gstable,m1fair}/report/`. N=30, single run; single-turn Q&A ≠ the coding-agent bug-cycle-cost thesis. | — |
| **E4** | `symbol-identity-design.md` §7 — mark Phase 1/2 complete; only remaining is M7. | [권장] | ✅ Done (2026-07-12). | — |
| **E5** | `coordination-response-cks-2026-06-29.md` T1 overstated the 2 knowledge tools as shipped with the flow-4. | [권장] | ✅ Done (2026-07-12) — added a dated correction: find_invariants/get_conventions shipped separately via M5 (cks #34 + ckv facade #35), so T1's 6 tools are now all exposed. | — |
| **M7** | Domain-knowledge anchor `kind:` migration (def vs loc). | [권장] | **Deferred — needs the source-of-truth commit.** ~150/164 anchors are def (back-compat correct, no change); only a handful are loc. Accurate def/loc classification = "is `line` the declaration of `symbol`?", which must be checked against go-stablenet **at the commit the entries were authored against** (line numbers drift). The reason-text heuristic is unreliable — it cannot distinguish "def of X" from "loc using X" and produces false positives (e.g. `NativeCoinManagerAddress:219` reads as loc but is a def; `ExtractWBFTExtra:251` names the *called* symbol, not the enclosing one). Blind bulk editing would corrupt curated knowledge. | Pin the authoring go-stablenet commit, then do a source-verified pass. Back-compat working meanwhile — no functional issue. |
| **M3** | T7 — composer causal orchestration (multi-hop `expand_flow`). | [권장] | Not started. | Avoid clashing with M2 measurement freeze. |
| **M4** | Embedding-dimension measurement. | [권장] | Waiting. | External: reindex-B (qwen3) index, CKV-owned. |
| **M5** | Expose `find_invariants` / `get_conventions` as dedicated tools. | [권장] | ✅ Done (2026-07-12). cks: FlowClient + MCP tools (cks #34 + ckv facade #35, repin #35). Live e2e against `pr-77-gstable`: `find_invariants` → 151 real invariants (file/tier filtered), `get_conventions` → per-package idioms. coding-agent: analyzer granted both tools + prompt pointer (coding-agent #60, 0.1.53) — the consumer gap that blocked the diagnose path. Only an autonomous-diagnose *observation* is left as an optional demo (plumbing proven end-to-end; needs a plugin reload + a full diagnose run). | — |

**Resolved (no rework):** E1, E2, E3, M1, **M6 + M1′ + E4** (2026-07-12), **M5**
(cks #34/#35 + coding-agent #60; live e2e proven, autonomous-diagnose demo optional),
**P0 + M6-data** (2026-07-12, cutover to `pr-77-gstable`), **M2** (2026-07-12, cks-bench
live: cks ~2× correctness over grep-only; hallucination follow-up noted).

**Live serving (2026-07-12):** `cks-stablenet` @ `192.168.0.116:8080`, dataset `pr-77-gstable`
(ckg schema 1.23 + column-removed ckv, commit `0bf2f4d1b`), `builder_version cks-mcp/0.1.0-90dc885d`,
`serviceable:true`, `alignment.ok:true`. Config `cks-stablenet.yaml` is gitignored;
regenerate with `CKS_DATASET_DIR=<KD>/pr-77-gstable GO_STABLENET_ROOT=<…>/go-stablenet/pr/pr-77-problem scripts/gen-cks-config.sh`.

**Ground-truth note (docs drift):** the old P0 plan (reindex `pr-77-2`, `SRC=vector-db-5`, "serving
degraded") was stale. Actual: serving was healthy on `pr-77` (pre-retire, still had the column); the
new `pr-77-gstable` had already been built by another session with the column-removed + sources-ledger
ckv. P0 became a **cutover + binary rebuild**, not a reindex.

**Recommended order (next):** `M2-hallucination follow-up (why do cks methods hallucinate more
than grep-only?) → M3 → (M4 external wait; M7 pending the authoring go-stablenet commit)`. The core
items (P0, M2, M5, M6) are done; what remains is the hallucination root-cause from M2, M3 (T7), and
the two external/deferred items. (M5's optional autonomous-diagnose demo can piggyback on any diagnose
run — it needs only a plugin reload, not dedicated work.)

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
