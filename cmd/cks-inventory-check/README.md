# cks-inventory-check

Runs the script-checkable half of the verification checklist from
`docs/domain-knowledge/shared/STATUS_LIFECYCLE.md` against one project's
domain-knowledge inventory.

The verification checklist has two halves:

1. **Mechanical** — schema validity, filename rule, cross-references,
   filesystem references. This is what `cks-inventory-check` covers.
2. **Substantive** — does the summary describe reality, do the
   invariants hold, are the aliases plausible. That stays a human task.

Running this before the substantive review lets reviewers focus on what
only humans can judge.

## What it checks

- **Schema-level**
  - required fields present
  - `id`, `subsystem`, `last_verified_at` match their patterns
  - `knowledge_type`, `status`, `priority`, `risk_level`, `source_of_truth` are in the allowed enum
  - title length 4..120, summary length >= 10
- **Status-driven**
  - `verified` requires `code_anchors >= 1`, `last_verified_at`, `verified_by`
  - `needs_verification` requires `code_anchors >= 1`
- **Filename rule**
  - filename stem equals the entry `id`
- **Cross-references**
  - every `subsystem` exists in `subsystems.yaml`
  - every `related_concepts[]` ID resolves to another entry
  - `related_concepts` does not reference itself
- **Filesystem refs**
  - every `code_anchors[].file` exists under `project.code_root`
  - every `existing_doc_ref[].file` exists under `project.code_root`
- **Soft warnings** (do not fail the check)
  - empty `code_keywords` → BM25 cannot match
  - no aliases at all → vocab resolver cannot match
  - `verified` without `risk_level`

## Usage

```bash
go run ./cmd/cks-inventory-check \
    -project docs/domain-knowledge/projects/go-stablenet
```

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `-project` | (required) | project directory containing `project.yaml`, `subsystems.yaml`, `entries/` |
| `-update-inventory` | false | after a clean validation, rewrite `<project>/inventory.md` count tables |

`-update-inventory` is skipped when errors are present, on the theory
that you do not want a dashboard reflecting a known-broken state.
Freeform sections in `inventory.md` (Conventions, Pending work, etc.)
are preserved byte-for-byte; only the four canonical count tables are
regenerated.

## Output format

Issues print in compiler-error format so editors can jump to the
offending file:

```
/abs/path/to/A1.bad.misnamed.yaml: error: A1.bad.actual: filename "A1.bad.misnamed.yaml" does not match id (want "A1.bad.actual.yaml")
/abs/path/to/A1.bad.misnamed.yaml: error: A1.bad.actual: knowledge_type "BX" not in B1..B7
cks-inventory-check: 2 entries, 2 errors, 0 warnings
```

Exit codes:

- `0` — no errors. Warnings (if any) printed.
- `1` — at least one error.
- `2` — usage error (missing flag, project unreadable).
