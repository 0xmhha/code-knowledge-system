# CKG Change-Impact Evaluation — Report (v2)

**The real question:** does the code-knowledge graph (ckg, queried through cks) actually
help an AI **understand code relationally** — i.e. find the *complete* set of code affected
by a change — better than a plain LLM with `grep`? This is what matters in practice: if the
tool only matches what `grep` already finds, it adds nothing; but if it surfaces the
non-local relationships `grep` can't see, it prevents the worst case — **missing an impact
site → side-effect bug → repeated rework → blown token budget.**

This v2 reframes the earlier comprehension eval (which under-tested the graph because its
"where is X / what does X do" questions are localized lookups `grep` handles well). Here we
test **change-impact recall**: given a hypothetical change to a symbol, enumerate every code
location that must change.

- **16 change-impact questions** over go-stablenet, each with a verified ground-truth impact
  set (136 members total), every member tagged by reachability: `direct` / `transitive` /
  `interface-impl` / `semantic`.
- **4 modes:** α = raw `grep` view, β = `get_subgraph` structure, γ = live cks relational
  queries (`find_callers` / `impact_analysis`), δ = `get_for_task` pack.
- **Metric:** impact **recall** (did you find the affected code?) + precision, split by
  reachability — *relational recall* is the discriminator (the members bare `grep` misses).
- AI under test = this session's model (subagents); ground truth built independently via
  `grep` + reading (not from ckg) to avoid circularity.

---

## Results (16 questions per mode)

| Mode | Recall | Precision | **Relational recall** | Direct recall | Hallucinations | Avg tokens |
|------|:---:|:---:|:---:|:---:|:---:|:---:|
| α grep | 0.400 | 0.514 | **0.086** | 0.386 | 0 | 1,895 |
| β subgraph | 0.448 | 0.277 | 0.114 | 0.406 | 0 | 3,964 |
| **γ on-demand (ckg)** | **0.768** | 0.517 | **0.514** | 0.762 | 0 | 13,719 |
| δ auto-pack | 0.284 | 0.428 | 0.086 | 0.277 | 0 | 2,174 |

Relational members found: **γ 18/35 (51%) vs α 3/35 (9%)** — a **6× advantage**.
Direct members: γ 77/101 (76%) vs α 39/101 (39%) — **2×**, even on direct callers.

---

## Findings

**1. The graph decisively wins at finding the change surface — γ 0.77 vs grep 0.40 recall.**
And the win is concentrated exactly where it should be: **relational members** (interface
implementers, transitive callers, semantic dependents) — γ recovers **51%** of them vs grep's
**9%**. This is the empirical proof of the thesis: *code has relationships `grep` cannot see,
and ckg's `find_callers`/`impact_analysis` surface them.*

Per-question, the graph's margin appears precisely on non-local changes:

| Question (change) | α grep | γ ckg |
|---|:---:|:---:|
| CI02 `TransitionDb` add return → caller chain | 0.08 | **0.85** |
| CI13 `F()` formula (noisy name) | 0.20 | **1.00** |
| CI09 `WBFTExtra` encoding | 0.14 | **0.93** |
| CI15 epoch-info struct | 0.20 | **0.90** |
| CI07 `AggregateSignatures` blast radius | 0.00 | **1.00** |
| CI14 message classification | 0.11 | **0.67** |
| CI11 commit format | 0.09 | **0.64** |
| CI01/03/04/06 (well-named, local) | ~1.0 | ~1.0 (tie) |

`grep` ties only on localized, well-named direct calls (CI01/03/04/06); on every change whose
impact runs through a wrapper chain, an interface, a noisy name (`F`), or a non-textual link,
it collapses to 0.08–0.20 recall while ckg stays 0.64–1.00.

**2. precision is equal (γ 0.52 ≈ α 0.51) — the graph is not just dumping noise.** γ doubles
recall *without* sacrificing precision. (β, the raw structure dump, is the noisy one:
precision 0.28.)

**3. δ (`get_for_task`) is the worst mode for impact (0.284).** The composer is built to
assemble *task context*, not an exhaustive caller set — multiple bundles showed it drifting
to governance/test/p2p context and missing the real call sites. Don't use `get_for_task` for
"what breaks if I change this"; use `find_callers`/`impact_analysis`.

**4. Semantic dependents remain hard for everyone (ckg's real limit).** On CI05 (blacklist
bit storage) and CI12 (storage-slot constants), both grep and the graph score low (γ 0.28 /
0.12). These impacts flow through *data/layout coupling with no call edge* — code that reads
`StateAccount.Extra` bits or reaches a slot through a helper. **ckg's call graph does not
model data-flow/semantic coupling**, so it can't find them either. This is the honest ceiling
of the current graph.

**5. Zero hallucination across all modes** for impact enumeration (answers cite real files).

**6. Cost — and why γ's higher cost is the *cheaper* path.** γ spent ~13.7k retrieval tokens
vs grep's 1.9k (~7×), because it issues many `find_callers`/`impact_analysis` calls. But the
whole point of the user's framing: a change made on grep's 40%-complete picture **misses ~5 of
every ~8.5 affected sites** → side-effect bugs → review/rework cycles that cost far more than
12k tokens. Paying 13.7k up front to find 77% of the surface (and 6× the relational links) is
the economical choice when the alternative is silent breakage.

---

## ⚠️ Critical ckg/cks defects found (these gate the value above)

The graph delivers — *but only if queried correctly*, and three real defects currently hide
that value:

1. **`find_callers`/`impact_analysis` resolve only the BARE symbol name; the documented
   fully-qualified name returns `null`.** `find_callers("QuorumSize")` → full correct caller
   tree; `find_callers("consensus.wbft.validator.defaultSet.QuorumSize")` → `null`. This is a
   silent trap: **all four ground-truth builders initially concluded "the edges aren't
   populated"** because they passed FQNs. A planner/agent that follows the tool's own
   `"Fully-qualified symbol name"` doc gets empty results and wrongly concludes "no impact."
   **This single bug would have made γ score ~0.** Fix: accept/resolve FQNs (or fix the doc +
   make resolution robust).
2. **`find_callees` (forward edges) is unreliable.** `find_callees("QuorumSize")` returns
   `triedb/pathdb` and `crypto/blake2b` as callees — name-collision garbage (QuorumSize only
   calls `Size()`/`F()`/`math.Ceil`). Reverse edges (`find_callers`) are accurate; forward
   edges are not. `get_subgraph` inherits the same spurious `calls` edges.
3. **No data-flow / semantic-coupling edges** (Finding 4) — the reason CI05/CI12 fail.

---

## Threats to validity
- Ground truth built via grep+reading (independent of ckg, but human-curated → some
  imperfection; ~8.5 members/question).
- Single run, single model; γ token cost is self-estimated.
- γ used the *corrected* bare-name path; a realistic agent hitting the FQN trap would score
  far lower (that trap is itself Finding/Defect #1).
- Index head shifted (`9978930`→`fd22f7f`) mid-study (a reindex); v2 is internally consistent
  at the current head.

## Conclusion
**Yes — ckg helps understand code in the way that matters.** For finding the complete
change-impact surface, the graph's relational queries beat a grep baseline **~2× overall and
~6× on the non-local relationships grep structurally cannot see**, at equal precision and zero
hallucination. That is exactly the capability that prevents side-effect bugs and downstream
rework. The value is **real but currently gated** by usability defects (FQN resolution, noisy
forward edges) and bounded by a real limit (no semantic/data-flow edges). Fixing the FQN
resolution is the highest-leverage next step — without it, the graph's main advantage is
invisible to anyone who queries it "by the book."

## Recommendations
1. **Fix `find_callers`/`impact_analysis` FQN resolution** (or correct the doc to "bare name")
   — highest leverage; it's the difference between γ=0.77 and γ≈0 for a by-the-book caller.
2. **Repair or de-noise `find_callees`/`get_subgraph` forward edges** (name-collision).
3. **Add data-flow/field-coupling edges** to capture semantic impact (CI05/CI12 class).
4. Route "what breaks if I change X" through `find_callers`/`impact_analysis`, **not**
   `get_for_task`.

---

### Artifacts (`eval/ckg-4way/impact/`)
- `gt/CI*.json` — 16 verified impact sets (reach-tagged) · `bundles/CI*/` — α/β/δ context
- `results_impact.json` — 64 runs · `scored_impact.json` — per-question recall/precision
- `score_impact.py` — the scorer
