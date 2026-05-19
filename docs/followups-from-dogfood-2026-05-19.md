# cks Follow-ups from Dogfood Eval — 2026-05-19

> Source: `make dogfood-eval` (USE_CKV=0 and USE_CKV=1) against `cks` indexing itself.
> 9 scenarios in `eval/scenarios/`, runs=5, reference report at `eval/reports/baseline-dogfood.json`.
> This document tracks **cks-side** follow-ups only. ckg / ckv side gaps are split into:
> - `<ckg>/docs/followups-from-cks-dogfood-2026-05-19.md`
> - `<ckv>/docs/followups-from-cks-dogfood-2026-05-19.md`

## Baseline (current state)

USE_CKV=0 (ckg only, stable):
- 9 / 9 OK, avg recall = 0.741, latency p95 ≈ 250ms

USE_CKV=1 (real ckv subprocess proxy):
- 7 OK / 2 errored (both `context deadline exceeded` from ckv subprocess hang — see ckv followups)
- avg recall on OK runs = 0.667 (no semantic uplift because ckv ships only mock embedder right now)

## Open items (cks-side)

| # | Priority | Item | Trigger / Evidence | Notes |
|---|---|---|---|---|
| F-1 | High | Drop synthesized BM25 score once ckg exposes real rank/score | `internal/ckgclient/real.go:150-192` | Currently we fake `1 - i/(N+1)`. The sum→max rerank fix (`b0ff3fa`) covers the worst pathology, but the synthesized score still bleeds into Stage 1 confidence, Stage 2 weights, and Stage 4 budget ordering. Replace with the real number as soon as ckg ships a `Score`/`Rank` field on `pkg/store.Node` (see ckg followup CKG-1). |
| F-2 | High | Cross-file expansion for recall=0.67 scenarios | `mcp-tool-handlers`, `stamp-integrity-lookup` plateau at R=0.67 even with rerank fix | Symptom: Stage 3 graph expansion only picks up direct neighbors. Look at `internal/composer/stage3/expander.go` — current TraversalDepth=1 may be undercutting. Probe: rerun with TraversalDepth=2 and measure recall delta. If recall climbs and latency stays within budget, raise the default. |
| F-3 | High | Multi-kind `SymbolKinds` fan-out in Stage 2 | `internal/composer/stage2/searcher.go` currently takes one `SymbolKind` per query | Today we issue one `SymbolLookup` per intent. For `arch_explain` we want fns + types + interfaces in one pass. Add fan-out loop, dedupe by Citation key, cap by `MaxCandidates`. |
| F-4 | High | Wire `Filter.CommitHash` through Stage 2 / Stage 3 | `internal/ckgclient/real.go:144` ignores it today | Until ckg supports cross-commit search this is dead weight, but the field is on `contract.SearchFilter` and confusingly silent. Either remove it from the surface or forward + return clean error from ckgclient.Real when set. |
| F-5 | Mid | Real `bgeonnx` embedder for semantic eval | Today FakeEmbedder + ckv mock = zero semantic signal | This unblocks two things: (a) measuring whether ckv adds recall over ckg-only, (b) the IntentTestAdd supplemental-pass routing in `internal/composer/stage2/intent_routing.go` only triggers when the classifier produces non-fake intents. Track here, but the real work is **bgeonnx setup**, which is ckv-side (see CKV-3). |
| F-6 | Mid | Separate baselines per backend mode | `eval/reports/baseline-dogfood.json` is overwritten by each run | Either suffix the report by mode (`baseline-dogfood-ckg.json` / `-ckv.json`) or store both in one report with a `mode` field per result. Pick one before USE_CKV becomes a meaningful eval axis. |
| F-7 | Mid | `composer-pipeline-flow` 400ms latency spike | One scenario consistently runs ~3× others on p95 | Likely Stage 4 body-fetch hitting many files for a broad query. Add per-stage latency to footprint JSONL, then re-run to confirm which stage owns the spike. |
| F-8 | Low | `qa-review-intent` recall = 0.00 (USE_CKV=0) | Intent classifier mismatch: scenario expects qa_review semantics, FakeEmbedder labels it `unknown` | Expected with mock classifier. Will resolve when F-5 lands. |
| F-9 | Low | `concurrency-safety-real-adapters` recall = 0.50 | `ckgclient.Real.Close` + mutex paths aren't in BM25 keyword corpus until queried by symbol | Edge case of the synthesized-rank issue (F-1) plus single-kind Stage 2 (F-3). Likely auto-resolves after F-3 lands. |

## Already-landed defenses (for context, not action)

These shipped in this dogfood iteration and bound the failure modes the above items target:

- `b0ff3fa fix(stage1): rerank by max(hit.Score), not sum` — prevents unique-identifier dropout under synthesized BM25
- `be35715 feat(ckvclient): subprocess restart + per-call timeout` — caps ckv hangs at `DefaultCallTimeout=10s` and restarts on `transport closed`
- `c7518d3 feat(budget): real FilesystemFetcher for Stage 4 body sourcing` — citations were 0 before this
- `aed8a31 feat(cks-mcp): wire footprint logger to all composer stages` — without this, the two bugs above were invisible

## Reproduction

```bash
# Stable, ckg-only:
make dogfood-eval

# Real ckv subprocess (expect 2 errors until ckv side is fixed):
make dogfood-eval USE_CKV=1

# Summary:
make dogfood-eval-summary
```

Report: `eval/reports/baseline-dogfood.json`.
