# cks-entry-verify

Promotes one domain-knowledge entry to `status: verified` and refreshes
the inventory.md count tables. The entry YAML is rewritten in place
through `yaml.Node` mutation, so comments, blank lines, field ordering,
and multi-line literal styles all survive — only `status`,
`last_verified_at`, and `verified_by` change.

Used at the end of the substantive verification step
(`docs/domain-knowledge/shared/STATUS_LIFECYCLE.md`), once a reviewer
has confirmed by hand that the entry's claims hold against current code.

## Safety: pre-flight validation

Before touching the file, the planned mutation is run through
`inventory.ValidateEntry` against a simulated post-promotion project.
If any error issue appears, no file is touched and the issues print to
stderr — the reviewer fixes the underlying entry, then re-runs.

This means a verified-by run on a structurally broken entry (missing
anchor, stale `related_concepts`, etc.) is a no-op, not a half-written
mutation.

## Usage

```bash
go run ./cmd/cks-entry-verify \
    -project docs/domain-knowledge/projects/go-stablenet \
    -entry   A1.wbft_core.quorum_calc \
    -by      mhha
```

`-entry` accepts either the entry ID or a path to the entry YAML file.
Both forms resolve to the same on-disk file.

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `-project` | (required) | project directory |
| `-entry` | (required) | entry ID or path to `<id>.yaml` |
| `-by` | (required) | reviewer handle recorded under `verified_by` |
| `-date` | today (UTC) | verification date, must match `YYYY-MM-DD` |
| `-skip-inventory` | false | do not rewrite `inventory.md` afterwards |

## Effect

For `A1.wbft_core.quorum_calc`, the diff against the entry YAML is
exactly:

```diff
-status: needs_verification
+status: verified
-last_verified_at: null
+last_verified_at: "2026-05-29"
-verified_by: null
+verified_by: mhha
```

`last_verified_at` is emitted with double quotes to keep it as a string
even when downstream tools walk the entry through an `any`-typed
unmarshal. Without quoting, yaml.v3 would auto-detect the ISO date and
hand back a `time.Time` value.

Then `inventory.md` Status summary and Subsystem coverage tables are
recomputed; freeform sections (Conventions, Pending work, etc.) are
preserved verbatim.

## Exit codes

- `0` — entry promoted, `inventory.md` updated (or skipped per flag).
- `1` — pre-flight validation failed; entry unchanged.
- `2` — usage error, entry not found, or filesystem IO error.

## When to run

- After a reviewer completes the substantive checklist for one entry.
- After `cks-inventory-check` reports zero errors against the project.
- Not during bulk edits — `cks-inventory-check -update-inventory` is
  the right tool when many entries change at once and you only need
  the dashboard tables refreshed.
