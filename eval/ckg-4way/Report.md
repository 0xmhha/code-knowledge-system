# CKG 4-Way Retrieval Evaluation — Report

**Question under study:** does using the code-knowledge graph (ckg/cks) actually help an
AI answer code-comprehension questions — and by how much — versus just handing it raw
files? Measured on **30 known-answer questions** about **go-stablenet**, each scored under
4 context modes (120 runs).

The AI under test is this Claude Code session's model (subagents); graph context comes from
the live **cks MCP** tools over the indexed go-stablenet graph (`graph.db` / `vector.db`,
head `9978930`). **No external API key was used** — the "AI consumer" is the in-session model.

---

## The four modes

| Mode | Name | Context supplied to the answering agent |
|------|------|------------------------------------------|
| **α** | raw files (baseline) | Raw source of the top files from a naive ripgrep keyword search, concatenated. No graph. |
| **β** | whole graph at once | `get_subgraph(seed, depth=2)` — the graph **structure** (nodes + edges), dumped in one shot. No source bodies. |
| **γ** | individual queries | The cks query tools (`find_symbol`/`find_callers`/`get_subgraph`/`search_text`), used iteratively by the agent. Returns citations, **not bodies**. |
| **δ** | auto-selected at once | `get_for_task(question)` once → the composed, token-budgeted EvidencePack (citations **with source bodies**). |

*β interpretation:* the full graph (~220k nodes) can't fit a context window; β is the naive
"dump the relevant graph neighborhood, no smart selection" approach, capped at ~30 KB.

## Metrics

- **Location accuracy** — fraction of questions where a cited `file:line` overlaps the ground-truth range. (deterministic)
- **Answer correctness** — LLM-judge verdict vs reference: `correct` / `partial` / `incorrect`. *Pass* = correct+partial.
- **Hallucinations** — cited `file:line` that does not exist or whose lines are beyond the real file. (deterministic; see methodology note)
- **Token usage** — context tokens supplied to the answering agent (α/β/δ from the bundle; γ self-estimated).

---

## Results (30 questions per mode, 120 runs)

| Mode | Location acc. | Correct | Pass | Hallucinations | Avg tokens |
|------|:---:|:---:|:---:|:---:|:---:|
| **α** raw files | 0.267 | **0.900** | 0.933 | 4 | **1,982** |
| **β** whole graph | 0.567 | 0.000 | 0.033 | 0 (+3 self-ref) | 3,496 |
| **γ** on-demand | **0.800** | 0.667 | 0.867 | **0** | 4,020 |
| **δ** auto-select | 0.367 | 0.833 | **0.967** | **10** | 3,459 |

**Correctness by difficulty** (correct-rate, n):

| Mode | easy (9) | medium (14) | hard (7) |
|------|:---:|:---:|:---:|
| α raw | 0.778 | 0.929 | 1.000 |
| β graph | 0.000 | 0.000 | 0.000 |
| γ on-demand | 0.667 | 0.714 | 0.571 |
| δ auto | 0.889 | 0.857 | 0.714 |

Ranking by metric:
- **Correctness (strict):** α (0.90) > δ (0.833) > γ (0.667) ≫ β (0.00)
- **Pass (corr+part):** δ (0.967) > α (0.933) > γ (0.867) ≫ β (0.033)
- **Location accuracy:** γ (0.80) > β (0.567) > δ (0.367) > α (0.267)
- **Token efficiency:** α (2.0k) < δ (3.5k) ≈ β (3.5k) < γ (4.0k)
- **Citation honesty (no fabrication):** γ = β (0) > α (4) > δ (10)

---

## Findings

**1. The raw-file baseline (α) is shockingly competitive — the graph is not a slam-dunk.**
α scored the **highest strict correctness (0.90)** at by far the **lowest token cost (2.0k)**,
beating both graph-backed modes on answering quality. For go-stablenet's well-named Go symbols,
naive ripgrep usually surfaces the right file, and the model answers correctly from raw source.
This is the humbling headline: on this question set, *the graph's value over a raw-file baseline
is modest for answer correctness.*

**2. δ (composed pack) gives the best pass-rate and the best one-shot coverage with bodies.**
δ has the top pass rate (0.967) and answers most questions correctly (0.833) including hard ones
(0.714) — because its EvidencePack ships actual source bodies. It is the strongest *single-shot*
mode. But it costs ~1.7× α and carries the most fabricated citations (see #4).

**3. γ (on-demand) owns localization and citation integrity, but under-answers.**
γ has the **best location accuracy (0.80)** and **zero fabricated citations** — its citations are
copied straight from cks tool output. Yet strict correctness (0.667) trails α/δ because the cks
query tools return **citations without source bodies**, so the model often can't state the full
mechanism (pass 0.867 shows it usually earns partial credit). It is also the priciest mode (4.0k
tokens, 168 total citations) due to iterative querying. *γ knows exactly where, but not always what.*

**4. Hallucination is about citation discipline — and δ, not γ, is the offender.**
Verified by checking every flagged citation against the real files and the provided bundles:
- **γ = 0 fabrications.** It cites the exact `file:line` returned by tools → never invents.
- **δ = 10 fabrications.** Despite high correctness, the model reads the pack's *bodies*,
  understands them, then **reconstructs line numbers from its own estimate instead of copying the
  pack's headers**, overshooting the file length (e.g. cited `prepare.go:159-172` in a 146-line
  file; the pack only contained `prepare.go:35-79/87-146/114-191`). Confirmed the bundles did
  **not** contain these ranges — it's the model embellishing, not a bundle error.
- **α = 4**, similar reconstruction errors; **β = 0** real (3 self-references to the bundle file).

This is the actionable insight: graph context does **not** cause hallucination; the mode that
hands the model *citations* (γ) is the most honest, while the mode that hands it *bodies and lets
it re-derive locations* (δ) fabricates most. A citation-validation pass (the deterministic checker
here) or a "copy pack headers verbatim" instruction fixes it.

**5. β (raw graph dump) is unusable for "how/what" questions.** 0% correct at scale: structure
(symbol locations + relations) without source bodies localizes (0.567) but never explains.

### Bottom line
On 30 questions, the graph's value is **specific, not global**:
- **Clear graph wins:** γ's precise, hallucination-free **localization** (0.80, the best), and δ's
  high **pass-rate** when it ships bodies (0.967).
- **Clear graph loss:** β (dumping structure) is dominated everywhere.
- **The surprise:** a **naive raw-file baseline (α) matches or beats the graph modes on answer
  correctness (0.90) at ~half the tokens** for this corpus — so "use the graph" is justified by
  *localization accuracy and citation integrity (γ)* and *one-shot pass-rate (δ)*, not by raw
  answer correctness over a competent baseline.

---

## Scoring methodology notes (important)

Two scorer bugs were found and fixed during analysis; the numbers above are post-fix:
1. **Path normalization:** an early `lstrip("./")` mangled `.claude/...` → `claude/...`, falsely
   flagging real indexed docs as "no-file." **The `.claude/docs/*.md` files are real** (340/321-line
   knowledge-base docs that cks embeds), so γ/δ citing them is legitimate retrieval, **not**
   hallucination. Fixed.
2. **Bare-basename citations:** citations given as `roundchange.go` (no directory) are now resolved
   to the real repo file before judging, so only genuinely out-of-range/nonexistent citations count.
3. **Self-reference vs fabrication:** citing the eval's own context bundle file (β did this 3×) is
   tracked separately as `self-ref`, not as a hallucination.

## Threats to validity
- **Single run, single model:** one answer per cell, one LLM-judge (same family → mild leniency).
  Location & hallucination are deterministic and trustworthy.
- **α's strength is corpus-dependent:** go-stablenet symbols are well-named, so ripgrep finds the
  right file. α would degrade on poorly-named or cross-cutting code — consistent with its *worst*
  location accuracy (0.267) and its brittle miss on Q01 (keyword "quorum" pulled governance files).
- **γ/δ confound:** γ's lower correctness is largely because cks query tools return
  *citations-without-bodies* while `get_for_task` (δ) includes bodies. A **γ-with-bodies variant**
  would isolate graph value from "bodies included" — recommended follow-up.
- **Location accuracy = citation precision**, not understanding (α: high correctness, low location).
- **Token measurement asymmetry:** α/β/δ measured from the bundle; γ self-estimated.

## Recommendations
1. **Give γ (and β) a body-returning path.** δ's correctness edge over γ comes entirely from source
   bodies; a cks tool that returns the snippet for a citation would likely lift γ to δ-level
   correctness while keeping γ's perfect citation integrity — plausibly the best of both.
2. **Add citation validation to δ-style consumers.** δ answers are trustworthy in content but
   fabricate ~1 line-range per 3 questions; validate citations against the pack headers (or instruct
   "cite only verbatim pack locations").
3. **Keep α as the honest baseline** in future runs — it sets a high, cheap bar the graph must beat.
4. Optional rigor: k=3 runs/cell for variance; the γ-with-bodies variant (#1) as a 5th mode.

---

### Artifacts
- `dataset.yaml` — 30 questions + ground truth · `DESIGN.md` — methodology
- `bundles/<id>/` — the α/β/δ context handed to each answerer (+ token counts)
- `results.json` — 120 raw runs (answers, citations, judge verdicts)
- `scored.json` — deterministic location/hallucination scoring + by-mode/difficulty/domain aggregates
- `score.py` — the scorer (post-fix)
