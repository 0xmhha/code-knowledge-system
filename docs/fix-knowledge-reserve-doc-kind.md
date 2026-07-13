# Fix: knowledge reserve never fires for the real domain-knowledge corpus

Branch: `fix/knowledge-reserve-doc-kind`

## Symptom (observed live, pr-77-gstable index)

`cks.context.get_for_task` for a generic bug prompt shipped **0 invariant chunks**
in the EvidencePack, even though the relevant invariant doc is the **top ckv
semantic hit** (score 0.54). Two live captures:

| prompt | bodies | tokens used / budget | invariants in pack |
|--------|:------:|:--------------------:|:------------------:|
| generic ("restore tx stuck in pending") | 12 | 2,829 / 7,200 (39%) | **0** |
| invariant-targeted | 11 | 1,517 / 7,200 (21%) | 2 |

So domain invariants reach the pack only when the query happens to rank them
above the code seeds — the KnowledgeReserve is not doing its job.

## Root cause (chunk_kind taxonomy mismatch)

The reserve code is correct and wired end-to-end, but it keys on the wrong value:

- Reserve rescue fires only for `chunk_kind ∈ {"invariant","convention"}`
  — `internal/composer/budget/allocator.go:245` (`isKnowledge`), and the
  kind-scoped stage1 pass filters the same set
  (`internal/composer/stage1/extractor.go:247`).
- But ckv indexes the domain-knowledge corpus from **out-of-tree markdown**
  (`A6.anzeon_gas.tip_override.md`, `## Invariants` section), so those chunks
  arrive as **`chunk_kind = "doc"` with an empty `commit_hash`** (they are not
  in the go-stablenet tree). `commit_hash == ""` is a reliable discriminator:
  in-tree markdown (READMEs) carries a real commit hash.
- Result: the invariant chunk is treated as an ordinary candidate, ranks below
  the code seeds, and the `MaxCitations = 12` cap
  (`allocator.go:320-322`, binds before the token budget) fills with code first
  → the invariant gets no slot.

Ruled out (both false): `commit_hash == ""` is **not** filtered by
`Citation.IsValid`/`assemblePack`/`EvidencePack.IsValid`
(`pkg/contract/citation.go:38-47` explicitly tolerates empty commit); and the
reserve is **not** dead code — it is wired
(`extractor.go:244 → composer.go:187 → stage2/merge.go:84 → allocator.go:245,252,263,311`).

## Task 1 — fix (DONE on this branch)

`internal/composer/budget/allocator.go:245` — recognise out-of-tree knowledge docs:

```go
isKnowledge := c.ChunkKind == "invariant" || c.ChunkKind == "convention" ||
    (c.ChunkKind == "doc" && c.Citation.CommitHash == "")
```

Tests added in `allocator_knowledge_test.go`:
- `TestAllocate_KnowledgeReserveRescuesDocKindCorpusChunk` — doc+empty-commit is rescued.
- `TestAllocate_InTreeDocIsNotKnowledge` — in-tree README (real commit) is NOT over-included.

`go build ./...` + `go test ./internal/composer/...` green.

Why the composer-side fix over a ckv reindex: the invariant chunks already reach
the candidate pool via the general semantic pass (confirmed live — capture #2),
so only the reserve gate was wrong. This needs **no reindex** and no ckv change.

### Alternative (data-side, defer)
Longer term, ckv could tag `## Invariants` / `## Conventions` doc sections with
the proper `chunk_kind`. That is cleaner semantically but requires a ckv indexer
change **plus a full bge-m3 reindex** of the corpus, and does not subsume the
`commit_hash == ""` guard. Not required for this fix.

## Task 2 — make it live + re-capture (PENDING deploy)

The running MCP still serves the old binary (`builder_version cks-mcp/0.1.0-90dc885d`).
To validate end-to-end and refresh the presentation capture:

1. Build: `go build -o bin/cks-mcp ./cmd/cks-mcp` (or the project's release path).
2. Restart the MCP the coding-agent connects to (`CKS_MCP_URL` / `CKS_MCP_BIN`) so
   it loads the new binary. **Remote host — operator action.**
3. Re-run the capture:
   `get_for_task("거버넌스로 GasTip을 30000에서 27600으로 복원하는 제안 tx가 pending에 정체 …")`
   Expect: `A6.anzeon_gas.tip_override.md` (`## Invariants`) now present in
   `bodies` for the **generic** prompt (was 0), and token utilisation up from 39%.

## Note on the 12-body cap (separate, not a bug)

`DefaultMaxCitations = 12` (`allocator.go:34`) intentionally stops selection at 12
bodies even with token budget left (`allocator.go:315-322`). That is why
utilisation sits at ~39%. It is a precision-over-volume choice, not a defect;
raise `MaxCitations` only if fuller packs are wanted. Tracked separately from this fix.
