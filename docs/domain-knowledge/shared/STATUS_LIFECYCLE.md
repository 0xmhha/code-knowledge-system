# Entry Status Lifecycle

> Status answers one question: *can this entry be indexed into CKV
> right now?* Only `verified` entries ship to production indexes. Every
> other status is a parking spot in the workflow.

## States

```
   needs_author ─────────►  draft ────►  needs_verification ────►  verified
        │                                       │                      │
        │                                       └──── fails review ────┘
        │                                                              │
        └────────────────────────────────────────── superseded ────────┘
```

### `needs_author`

- **Meaning**: The topic was identified (entry file exists or is planned)
  but nobody has written the content yet.
- **Required fields**: `id`, `subsystem`, `knowledge_type`, `title`,
  `priority`. Other fields may be empty/null.
- **CKV indexing**: excluded.
- **Owner**: someone with domain knowledge of the subsystem.

### `draft`

- **Meaning**: Content is written but has not been checked against the
  current code. Anchors may be stale.
- **Required fields**: all required + `summary`.
- **CKV indexing**: excluded.
- **Owner**: original author until they hand off to a verifier.

### `needs_verification`

- **Meaning**: Content is considered complete; pending the verification
  steps below.
- **Required fields**: all required + `summary` + at least one
  `code_anchor`.
- **CKV indexing**: excluded.
- **Owner**: a reviewer (separate from the author when possible).

### `verified`

- **Meaning**: All facts confirmed against current code; safe to embed.
- **Required fields**: every applicable field; `last_verified_at` and
  `verified_by` set.
- **CKV indexing**: included.
- **Re-verification cadence**: re-check whenever the code anchors'
  files change (tracked via git diff against `last_verified_at`).

### `superseded` (implicit)

- **Meaning**: Content was correct but a newer entry replaces it
  (e.g. system contract v2 entry supersedes v1). Marked by deleting the
  old YAML or moving to an archive directory.
- **CKV indexing**: excluded.

## Verification checklist (draft → needs_verification → verified)

A reviewer moves an entry from `needs_verification` to `verified` only
after all of the following pass:

### Mechanical (script-checkable)

- [ ] Schema valid (`entry.schema.yaml`)
- [ ] Filename matches `id`
- [ ] `subsystem` exists in the project's `subsystems.yaml`
- [ ] Every `code_anchors[].file` exists under `project.code_root`
- [ ] Every `code_anchors[].symbol` (when set) resolvable via grep
- [ ] Every `related_concepts[]` ID exists in the project's `entries/`
- [ ] Every `existing_doc_ref[].file` exists

### Substantive (human-checked)

- [ ] `summary` is factually correct against current code
- [ ] `code_keywords` match identifiers actually used in source
- [ ] `invariants` actually hold (reviewer can point at the code line
      that enforces each)
- [ ] `pitfalls` are real (reviewer can describe a failure mode for each)
- [ ] `korean_aliases` / `english_aliases` are domain terms a real user
      would type — not just translations the author invented
- [ ] `risk_level` matches the alias commonness (a Korean alias like
      "처리" must be `high`; "쿼럼" can be `low`)

> **Tooling**: `cks-promotion-worksheet -project <dir>` emits a
> markdown worksheet that turns the substantive checklist above into
> per-entry fillable sections (Q1–Q4 + decision triple). It pre-fills
> the `Maps to:` byzantine-catalog cross-reference (best-effort
> heuristic) and a `suggested: <file:line>` hint on each invariant
> based on `code_anchors[].reason` token overlap. The reviewer fills
> the actual judgments. APPROVE decisions are promoted by the inline
> per-entry `cks-entry-verify` command at the bottom of each section.

### Provenance

- [ ] `last_verified_at` set to today's date
- [ ] `verified_by` set to the reviewer's handle
- [ ] `source_of_truth` matches where the facts came from

## Status field is fact, not preference

The status enum is not a quality rating. It records the verification
state at a point in time. Use it consistently:

- An entry with great content but unverified anchors is `draft`, not
  `verified` "because the content looks good".
- An entry that was `verified` last month but whose anchor file changed
  in the meantime is no longer `verified` — it's `needs_verification`
  until re-confirmed.

## Operational impact

- `inventory.md` aggregates the count by status so the dashboard shows
  index readiness at a glance.
- CKV's `ckv build` reads `status: verified` entries only.
- A `verified` count of zero is a valid state — the project is still
  registered, but its inventory is not yet ready for indexing.
