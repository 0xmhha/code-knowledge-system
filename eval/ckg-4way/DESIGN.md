# CKG 4-Way Retrieval Evaluation — Design

**Goal:** quantify whether using the code-knowledge graph actually helps an AI answer
code-comprehension questions, by comparing four ways of supplying context over a fixed
set of known-answer questions about **go-stablenet**.

The AI under test is **this Claude Code session's model** (subagents), so **no external
`ANTHROPIC_API_KEY` is needed**. Context for graph-backed modes comes from the live
**cks MCP** tools (`cks.context.*`), which delegate to the indexed go-stablenet ckg/ckv
backends (`data/ckg-stablenet/graph.db`, `ckv-stablenet/vector.db`).

## The four modes

| Mode | Name | Context supplied to the answering agent | Built by |
|------|------|------------------------------------------|----------|
| **α** | raw files (baseline) | Raw source of the top files from a naive keyword/path search (ripgrep), concatenated and token-capped. No graph. | pre-built bundle |
| **β** | whole graph at once | `get_subgraph(seed, depth=2)` around the question's seed symbol, serialized and dumped in one shot. Unfiltered neighborhood. | pre-built bundle |
| **γ** | individual queries | No pre-built context. The answering agent is given the cks query tools (`find_symbol`, `find_callers`, `find_callees`, `get_subgraph`, `search_text`) and must locate the answer itself, iteratively. | live tools |
| **δ** | auto-selected at once | `get_for_task(question)` once → the composed, token-budgeted EvidencePack, supplied in one shot. No further tools. | pre-built bundle |

> **β interpretation.** The literal full graph (~220k nodes / 1.9M edges) cannot fit a
> context window. β represents the *naive "give the AI the whole graph, no smart
> selection"* approach: the depth-2 unfiltered subgraph around the seed, capped at a
> generous token budget. This is documented as a methodological assumption.

## Metrics (per question × mode)

1. **Location accuracy** — do the file:line citations in the answer match the
   ground-truth location? A citation *hits* if `file` matches and its line range
   overlaps the ground-truth range (±0 lines; overlap by ≥1 line). Score = hit/miss.
2. **Answer correctness** — LLM-judge subagent compares the answer to the reference
   `answer_summary` + ground truth → `correct | partial | incorrect`.
3. **Hallucination count** — deterministic: for each file:line the answer cites, verify
   the file exists, the line range is within the file, and (if a symbol is named) the
   symbol exists at/near those lines. Each invalid citation = 1 hallucination.
4. **Token usage** — size of context the answering agent consumed:
   - α/β/δ: token count of the supplied bundle (δ also reports `used_tokens` from pack metadata).
   - γ: sum of the tokens of the snippets the agent actually retrieved via tools.

## Pipeline

1. **Dataset** — `dataset.yaml`, 30 questions with ground-truth `file:line` + reference
   answers (8 flagged `pilot: true`).
2. **Bundle build** — for each pilot question, produce `bundles/<id>/{alpha,beta,delta}.txt`
   + `meta.json` (token counts, seed). γ needs no bundle.
3. **Answer** — one subagent per (question, mode): α/β/δ read the bundle and answer
   *only* from it; γ uses the live tools. Returns `{answer, citations:[{file,start,end,symbol}]}`.
4. **Judge** — one subagent per answer: correctness verdict vs reference.
5. **Score + aggregate** — deterministic location/hallucination scoring merged with
   judge verdict and token counts → `results.json` → `Report.md`.

## Layout

```
eval/ckg-4way/
├── DESIGN.md          # this file
├── dataset.yaml       # 30 questions + ground truth
├── bundles/<id>/      # pre-built α/β/δ context per question (+ meta.json)
├── results.json       # raw per-(question,mode) scored results
└── Report.md          # the comparison report (deliverable)
```
