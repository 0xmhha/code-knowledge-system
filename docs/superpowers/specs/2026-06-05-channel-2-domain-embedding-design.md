# Channel ② — Domain-Knowledge Embedding (design)

> **Status:** approved (2026-06-05) · **Scope:** P0-b of the domain-knowledge curation roadmap
> (`coding-agent/docs/r1-refactor/07-domain-knowledge-curation.md` §5, §7).
> **Repos touched:** cks (primary — render + orchestrate), ckv (one additive flag).
> **Out of scope:** coding-agent L3 internalization (P0-c); new subsystems / T2 authoring (P0-c).

## 1. Goal

Make cks domain-knowledge entries **retrievable as first-class documents** through ckv
`semantic_search` / the composer, so an LLM designing go-stablenet code receives the
curated invariants, pitfalls, divergence, and the **concept-anchored (T2) knowledge that
has no code anchor** — the dimension `06-integration-verification.md` found absent. The
same index also embeds the go-stablenet **authoritative docs** declared in `project.yaml`.

Channel ① (cks-domain-sync → ckv policy `watch_out` injected onto code chunks) already
exists and is unchanged. Channel ② adds the entry/doc **prose itself** to the vector store.

## 2. Decisions (locked)

| # | Decision | Value |
|---|----------|-------|
| D1 | Scope | Channel ② only (ckv embedding of entries + authoritative docs). Internalization deferred. |
| D2 | Gating | Embed entries with `status ∈ {verified, needs_verification}`. Each rendered doc carries a **Status** line so retrieval conveys confidence. `draft` / `needs_author` excluded. |
| D3 | Architecture | One ckv index over two sources: the go-stablenet code tree (`--src`) + a cks-generated **markdown corpus** (`--docs`). ckv stays schema-agnostic (consumes markdown); cks owns rendering. |

## 3. Architecture

```
entries/*.yaml + project.yaml(authoritative_docs) + $GO_STABLENET_ROOT/.claude/*.md
   │
   ├─(cks-domain-export)→ generated/domain-corpus/go-stablenet/   (gitignored, disposable)
   │        entries/<id>.md   (one per embeddable entry, Status badge)
   │        docs/<name>.md     (copies of authoritative_docs, resolved under code_root)
   │
   └─(ckv build --src $GO_STABLENET_ROOT --docs <corpus>)→ single vector index
            code chunks (Category from channel-① policy) + domain doc chunks (Category="domain")
   │
   →(cks SemanticSearch, in-process)→ Hits; domain hits carry corpus path + prose
   →(composer EvidencePack)→ entry prose surfaced alongside code
```

One `cks.ops.index` call refreshes channel ① (ckg policy) + channel ② (corpus + ckv) + the
code index together. This single orchestration is what keeps environment setup simple
(no requirement to clone/install extra repos at query time — see
`memory/go-stablenet-docs-distribution.md`).

## 4. Components

### 4.1 ckv — additive `--docs <dir>` on `ckv build`

- **`cmd/ckv/build.go`**: add repeatable `--docs <dir>` flag (additional markdown root(s)).
- **`internal/discover`**: walk each `--docs` root; classify `.md`/`.markdown` as markdown
  (existing `classifyLanguage`). No new parser/chunker — corpus files become
  `ChunkKind=ChunkDoc` / `SymbolKind=KindDocSection`, exactly like in-tree markdown today.
- **Tagging:** chunks discovered under a `--docs` root get `Category="domain"` so callers
  can filter domain knowledge from code. (Set at discovery/chunk construction; no schema
  change — `Category` already exists on `Chunk`.)
- **Citation:** `Chunk.File` = path relative to the corpus root (e.g.
  `entries/A4.system_contracts.addresses.md`), so retrieval surfaces a stable "where".
- **`internal/manifest`**: record `docs_root(s)` alongside `src_root` so `ckv reindex`
  (which reuses the manifest) re-walks the corpus. `reindex` without a recorded docs_root
  behaves as today.
- **Constraint:** the corpus must be embedded with the **same model** the code index uses
  (bge-m3 via Ollama, per cks config) — guaranteed because it is the same `ckv build` run.

### 4.2 cks — `cks-domain-export` (new `cmd/cks-domain-export`)

Renders the corpus from one project. Pure, deterministic, idempotent.

- **Input:** project dir → `inventory.LoadProject` (entries, subsystems, resolved
  `code_root`); reads `project.yaml` `authoritative_docs`.
- **Output dir** (`--out`, default `generated/domain-corpus/<project-id>/`):
  - `entries/<id>.md` for each entry with `status ∈ {verified, needs_verification}`.
  - `docs/<basename>.md` = byte copy of each `authoritative_docs[].file` resolved under
    `code_root`. (This finally gives `authoritative_docs` a consumer.)
- **Determinism:** entries sorted by ID; stable rendering → clean diffs across rebuilds.
- **Loader change:** `internal/inventory` — surface `AuthoritativeDocs` on `Project`
  (today `project.yaml`'s field is parsed-and-ignored). Add a typed field + loader wiring.

#### Entry → markdown renderer (new package `internal/domainexport`)

Kept out of `internal/inventory/render.go` (which owns inventory-table generation) for
single responsibility. `domainexport` depends on `inventory` (the `Entry`/`Project` model)
and produces the corpus; `cmd/cks-domain-export` is a thin CLI over it.

```markdown
# {Title}

**Status:** {status} · **Subsystem:** {subsystem} ({subsystem name}) · **Type:** {knowledge_type}

{Summary}

## Invariants
- {each invariant}

## Pitfalls
- {each pitfall}

## Code anchors
- `{file}`{:Symbol}{:Line} — {reason}

## Aliases
{korean_aliases + english_aliases + code_keywords, comma-joined}

## Related
{related_concepts as IDs}
```

- Omit empty sections (entries without invariants/pitfalls skip those headers).
- `Summary` is the primary embedding signal (carries T2 prose). Invariants/pitfalls are the
  "what must not break / trap" payload. Anchors give code back-links. Aliases boost
  Korean/English + vocab recall.
- The **Status** line (D2) makes confidence visible in every retrieved document.

### 4.3 cks — `cks.ops.index` orchestration (`internal/mcp/ops_index.go`)

Extend the existing maintenance op so one call does, in order:
1. `cks-domain-export` → (re)generate the corpus (in-process call to the export package,
   not a shell-out, since both are cks).
2. `ckv build --src <SourceRoot> --docs <CorpusDir>` (full) or `ckv reindex` (incremental,
   corpus re-walked from the manifest's recorded docs_root).
3. (existing) `ckg build [--policy-file]` for channel ①.

- **Config:** add `DomainProjectDir` + `DomainCorpusDir` to `IndexConfig` (and the cks
  config backing it). Empty `DomainProjectDir` disables channel ② (corpus step skipped) —
  back-compatible.

## 5. Data model touchpoints (summary)

| Field/struct | Where | Change |
|---|---|---|
| `Project.AuthoritativeDocs` | cks `internal/inventory/types.go` + `load.go` | **new** — parse the existing yaml field |
| entry→markdown renderer | cks new pkg `internal/domainexport` | **new** |
| `--docs` flag + walk | ckv `cmd/ckv/build.go`, `internal/discover` | **new** (additive) |
| `Category="domain"` on corpus chunks | ckv discover/chunk | reuse existing `Chunk.Category` |
| `docs_root` in manifest | ckv `internal/manifest` | **new** (additive) |
| `IndexConfig.DomainProjectDir/DomainCorpusDir` | cks `internal/mcp/ops_index.go`, `internal/config` | **new** (optional) |

No new ckv chunk kind or DB column is required; `Category` and the markdown path already
exist on `Chunk`.

## 6. Error handling

- **Missing authoritative_doc under code_root** → warn, skip that doc, continue (the
  inventory-check already validates existence, so this is a guard, not the common path).
- **`code_root` unset** → export entries only, warn (mirrors the existing degrade where
  anchor checks skip).
- **No embeddable entries** → corpus `entries/` empty; `ckv build` proceeds with code +
  whatever docs exist; an empty `--docs` root is a no-op.
- **Embedder/model mismatch** → unchanged; ckv manifest guard already covers it.
- Export failures are surfaced (no silent swallow); `cks.ops.index` reports which stage
  failed.

## 7. Testing

**cks**
- Renderer golden test: an entry with invariants/pitfalls/anchors/aliases → expected
  markdown; assert the Status line is present and reflects `status`.
- Gating test: a fixture set with all four statuses → only `verified` +
  `needs_verification` produce `entries/*.md`.
- authoritative_docs test: temp `code_root` with two docs → both copied to `docs/`;
  one missing → warned + skipped, others still copied.
- Determinism test: two exports of the same input are byte-identical.

**ckv**
- `--docs` discovery: a temp corpus dir with markdown → indexed; hits carry the
  corpus-relative `File` and `Category="domain"`.
- Manifest: `docs_root` recorded; `reindex` re-walks it.

**e2e (optional, behind Ollama availability)**
- Build a small index with `--src` + `--docs`; a domain-flavored query returns the entry
  document above the code chunks.

## 8. Build sequence

1. **ckv**: `--docs` flag + discover walk + `Category="domain"` tag + manifest `docs_root`
   + tests. (Independent; ship as a ckv PR.)
2. **cks**: `Project.AuthoritativeDocs` loader field; entry→markdown renderer; tests.
3. **cks**: `cks-domain-export` command wiring the renderer + authoritative-doc copy.
4. **cks**: `cks.ops.index` orchestration (export → ckv build --docs); config fields;
   `.gitignore` the corpus output dir.
5. **Verify**: run the orchestration against go-stablenet @9978930ba with
   `GO_STABLENET_ROOT` set; confirm a domain query returns entry prose.

Steps 2–4 are one cks PR; step 1 is one ckv PR (cks step 4 depends on it).

## 9. Out of scope (explicit)

- coding-agent L3 backstop / internalization of this knowledge (P0-c).
- Authoring new subsystems or T2 entries (P0-c).
- Channel ③ code markers.
- Embedding the entire `.claude` tree — only `project.yaml` `authoritative_docs`.
