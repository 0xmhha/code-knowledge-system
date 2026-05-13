# Stage 3 Graph Expander — Scoring Analysis

Records the design decisions for Stage 3's neighbor scoring and the
alternatives we considered. Phase-0 ships with the simple choices below;
Phase E (evaluation) is where we expect to revise based on PR #70
miscluster data.

## 1. Score Decay Formula

### Current (Phase 0)

```
neighbor_score = seed.Score / (1 + distance)
```

| distance | divisor | example: seed=10.0 |
|----------|---------|--------------------|
| 1 | 2 | 5.0   |
| 2 | 3 | 3.33  |
| 3 | 4 | 2.5   |
| 4 | 5 | 2.0   |

Characteristics:
- Smooth, monotonic.
- Score never reaches 0 — even distant neighbors retain a fraction.
- 1-hop neighbors carry exactly half the seed's evidence weight.
- Easy to reason about: "halve at hop 1, third at hop 2."

### Alternatives Considered

#### A) Exponential decay: `seed.Score * exp(-distance)`

| distance | factor | example: seed=10.0 |
|----------|--------|--------------------|
| 1 | 0.368 | 3.68 |
| 2 | 0.135 | 1.35 |
| 3 | 0.050 | 0.50 |
| 4 | 0.018 | 0.18 |

Trade-offs vs. current:
- ✅ Distant neighbors fade faster → top-N ranking dominated by close neighbors.
- ✅ Mirrors typical "attention decay" patterns in graph-RAG literature.
- ❌ Aggressive — at hop 3+ score is essentially zero, losing potentially
  useful long-range evidence.
- ❌ One more `math` import + a magic constant. Slight complexity cost.

#### B) Logarithmic dampening: `seed.Score / (1 + log(1 + distance))`

| distance | divisor | example: seed=10.0 |
|----------|---------|--------------------|
| 1 | 1.69 | 5.91 |
| 2 | 2.10 | 4.76 |
| 3 | 2.39 | 4.19 |
| 4 | 2.61 | 3.83 |

Trade-offs vs. current:
- ✅ Distant neighbors retain more evidence — useful for ArchExplain-type
  intents where the architecture story spans hops.
- ❌ Near-flat decay means distant neighbors compete with closer ones in
  the cap — defeats the purpose of distance discounting for trace intents.
- ❌ Less interpretable.

#### C) Threshold cutoff: `if distance > MaxDistance then 0 else seed.Score / (1 + distance)`

Trade-offs vs. current:
- ✅ Hard guard against very-distant neighbors slipping into the cap.
- ❌ ckg.NeighborsOpts.Hops already provides this cutoff at the source.
  Adding a Stage-3 threshold is redundant.
- ❌ One more config knob to tune.

### Why Current is the Phase-0 Default

The three alternatives differ from the current formula along a single axis:
**how aggressively to discount distance**. All three flatten or steepen the
score curve; none change the fundamental ranking order in most cases. The
difference matters only when the cap is binding and the top-N selection
includes neighbors at multiple distances.

We don't yet have data showing which curve fits the PR #70 evaluation best.
The current formula is:

- The simplest of the four.
- Monotonic and never zero (no surprise drop-offs).
- Easy to explain to a reviewer: "score halves at hop 1."

Until Phase E measures miscluster rates, switching to one of the
alternatives is speculation.

### Phase E Measurement Plan

When PR #70 evaluation data is available, measure:

1. **Distance distribution of correctly-identified citations.** If most
   are at hop 1, exponential decay is better (steeper helps close
   neighbors win). If hop 2+ matters, logarithmic is better.
2. **Top-N stability.** How often does the top-50 cap change between
   formulas? If rare, the choice doesn't matter. If frequent, tune to
   match human PR's distance preference.
3. **Per-Intent variation.** BugFix likely prefers close (steeper);
   ArchExplain likely prefers far (flatter). May need per-Intent
   formula override.

Concrete change protocol: add `DecayFormula` field to `stage3.Config`
with an enum (`Linear`, `Exponential`, `Logarithmic`), measure on PR
#70, pick the winner, lock as default.

## 2. Multi-Path Aggregation (max vs sum)

### Current (Phase 0)

When multiple seeds reach the same neighbor (Target citation), the
neighbor's score is set to `max(competing_scores)`. The Sources slice
records all paths.

### Why max, not sum

Graph hubs (e.g., a logging utility called from hundreds of sites)
would dominate any sum-based score regardless of semantic relevance.
Max preserves "best evidence we have for this target" without
over-weighting popularity.

### Known limitation

A genuinely "central" node — one that legitimately many seeds point at
because it IS the relevant piece — gets the same score as a node
reached from only one seed. Sum would correctly elevate the central node.

We don't currently distinguish "hub by coincidence" from "hub by
relevance." Phase E should:

1. Identify cases where human PR #70 modified a node that Stage 3
   reached from many seeds at low max-score.
2. If that pattern dominates, switch to a hybrid:
   `score = max(competing) + count_factor * log(len(paths))`
   — preserves the max base while rewarding multi-path concentration.

## 3. Intent → Relations Mapping Accuracy

The current per-Intent Relation mapping (see `intent_relations.go`) is
derived from a priori reasoning about what each Intent needs from the
graph. We haven't measured whether the choices match what human PRs
actually touch.

Phase E should:

1. For each Intent in the PR #70 corpus, compute which Relations the
   human PR's changed neighbors arrived through.
2. Compare to the current mapping. Mismatches are tuning candidates:
   - Relations in current mapping but rarely useful → remove or
     deprioritize.
   - Relations missing from current mapping but consistently useful →
     add.
3. Specific suspicions to verify:
   - **ConcurrencySafety includes `references`** — chosen to capture
     shared-state access patterns, but may bring too much noise from
     unrelated variable reads. Test against actual concurrency PRs.
   - **DocsUpdate at hops=1, no `calls`** — may miss documenting
     downstream effects (e.g., "function X calls Y, both need updated
     godoc"). Test against doc-only PRs.

## 4. Sources String Format

Format: `seed:<file>:<relation>:dist=<n>`

Example: `seed:internal/auth/login.go:called_by:dist=1`

Why this format:
- Colon-delimited fields parseable with a single `strings.Split`.
- Each field is self-describing (`dist=N` is explicit, not positional).
- Same shape as Stage 2's Sources format (`bm25:<keyword>=<score>`),
  so the audit log can grep both layers uniformly.

Phase E may extend with additional fields (e.g., elapsed time per ckg
call) but the field-prefix convention should stay so existing tooling
continues to parse correctly.

## 5. Cross-Stage Coupling

Stage 3 imports `stage2.ScoredCitation` directly. The package
boundary is:

- `stage2.ScoredCitation` carries Score + Sources of evidence from
  BM25/Symbol search.
- Stage 3 needs both fields (Score for decay, Citation for the
  seed-key set).

This is a one-way dependency (`stage3 → stage2`). No cyclic risk in
Phase 0. If a future stage (B.6 budget, B.8 wire-up) introduces a
back-edge, we'll factor a common `pkg/composer/types` package and
move the shared types there.
