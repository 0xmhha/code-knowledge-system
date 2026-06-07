# cks-promotion-worksheet

Emits a markdown worksheet that a domain expert fills in during a
promotion session (`needs_verification` → `verified`). One section per
entry, plus a shared session header and a post-session sync footer.

The worksheet is the input artifact for the **substantive** half of
the verification checklist in
`docs/domain-knowledge/shared/STATUS_LIFECYCLE.md`. The **mechanical**
half is assumed already-passing — run `cks-inventory-check` first and
confirm `0 errors` before generating the worksheet.

## Why this tool exists

Without it, a 29-entry session blocks on the expert re-discovering, per
entry:

- which `code_anchors[]` lines to read,
- which 07 §9 hallucination-risk item this entry is supposed to block,
- which anchor most plausibly enforces each invariant,
- what command to run to promote the entry after APPROVE.

The generator pre-fills all four (best-effort) and leaves the actual
judgment — Q1–Q6 + decision — to the expert. In tests on
go-stablenet @9978930ba this reduces an expert session for 29 entries
from ~10–15 hours (cold) to ~3–4 hours.

## Usage

```bash
# default: status=needs_verification, all priorities, stdout
go run ./cmd/cks-promotion-worksheet \
    -project docs/domain-knowledge/projects/go-stablenet

# write to a file
go run ./cmd/cks-promotion-worksheet \
    -project docs/domain-knowledge/projects/go-stablenet \
    -out     docs/domain-knowledge/projects/go-stablenet/verification-worksheet.md

# narrow to one priority bucket
go run ./cmd/cks-promotion-worksheet \
    -project docs/domain-knowledge/projects/go-stablenet \
    -priority P0
```

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-project` | (required) | project directory (contains `project.yaml`, `subsystems.yaml`, `entries/`) |
| `-status` | `needs_verification` | entry status to include (`draft` / `verified` etc. also valid) |
| `-priority` | `""` (all) | optional `P0` / `P1` / `P2` / `P3` filter |
| `-out` | `""` (stdout) | output path; empty writes to stdout |

## What the generator pre-fills

| Slot | Source | Override |
|---|---|---|
| `Maps to:` | token-overlap heuristic against the 07 §9 catalog (mirrored in `main.go`, keep in sync when 07 §9 changes) | reviewer edits in-line |
| `suggested: <file:line>` on each invariant | anchor whose `reason` field shares the most tokens with the invariant text | reviewer ignores or replaces |
| Per-entry `cks-entry-verify` command | entry ID + project path | — |
| Post-session sync block | `cks-domain-sync` + `cks-glossary-gen` + `cks.ops.index{full}` | — |

## What the generator does NOT pre-fill

- Pitfall failure modes (Q4). Writing one is the single best evidence that the pitfall is real and not decorative — the expert must produce it.
- The decision triple. Promotion is atomic per `STATUS_LIFECYCLE`; no partial accept.

## Relationship to other tools

```
draft ─► needs_verification ─[ cks-inventory-check (mechanical, 0 errors) ]─►
       │                                                                    │
       └─► [ cks-promotion-worksheet ] ─► expert review ──► APPROVE ────────┘
                                                  │                         │
                                                  └─► REVISE / REJECT       ▼
                                                                  [ cks-entry-verify ]
                                                                            │
                                                                            ▼
                                                                       verified
```

## Exit codes

- `0` — worksheet emitted (even when 0 entries match: an empty queue is a legitimate result, not a tool error).
- `2` — usage error (missing `-project`, project directory unreadable).
