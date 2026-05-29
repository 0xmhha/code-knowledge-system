# Entry Schema

> Every YAML file under `projects/<id>/entries/` follows this schema.
> The shape is stable across projects; new projects do not extend it
> without bumping `schema_version` in `project.yaml` and updating
> `entry.schema.yaml`.

## Field reference

### Required

| Field | Type | Meaning |
|---|---|---|
| `id` | string | Stable identifier. Format: `<subsystem>.<topic>.<subtopic>` (e.g. `A1.wbft_core.quorum_calc`). Filename must match: `<id>.yaml`. |
| `subsystem` | string | One of the IDs declared in this project's `subsystems.yaml`. |
| `knowledge_type` | enum | One of `B1`~`B7` (see `KNOWLEDGE_TYPES.md`). |
| `title` | string | Human-readable title. One line. |
| `summary` | string | 1~3 sentences. This is the primary text CKV embeds; write it for retrieval, not for narrative. |
| `status` | enum | `verified` / `draft` / `needs_verification` / `needs_author` (see `STATUS_LIFECYCLE.md`). |
| `priority` | enum | `P0` (core flow), `P1` (frequent reference), `P2` (rare / specialized). |

### Optional but strongly recommended

| Field | Type | Meaning |
|---|---|---|
| `code_anchors` | list of objects | Code locations this entry refers to. Each: `{file, symbol?, line?, reason?}`. `file` is a path relative to the project's `code_root`. At least one anchor required for `verified` status. |
| `code_keywords` | list of strings | Exact identifiers as they appear in source (function/type names, constants). Drives BM25 retrieval — case-sensitive copies from code. |
| `korean_aliases` | list of strings | Korean phrases a user might type. Used by the vocabulary resolver. |
| `english_aliases` | list of strings | Vague or domain-jargon English phrases that map to the same concept. |
| `related_concepts` | list of strings | IDs of other entries this one cross-references. Validated to exist. |
| `existing_doc_ref` | list of objects | Pointers into the project's own docs (`.claude/docs/...` etc.). Each: `{file, section?, subsection?}`. |

### Conditional (use only when relevant)

| Field | Type | Use when knowledge_type is | Meaning |
|---|---|---|---|
| `invariants` | list of strings | `B4` | Must-hold conditions phrased as plain statements. |
| `pitfalls` | list of strings | `B5` | Anti-patterns / common mistakes. |
| `procedure_steps` | ordered list | `B6` | Step-by-step checklist. Each item is a short imperative sentence. |
| `constants` | list of objects | `B7` | `{name, value, unit?, source_file}`. |

### Provenance (auto-managed where possible)

| Field | Type | Meaning |
|---|---|---|
| `risk_level` | enum | `low` / `medium` / `high`. Risk that this entry's aliases produce false positives when wired into the vocabulary resolver. Common Korean words ("처리", "실행") are `high`. Domain terms ("쿼럼", "fee delegation") are `low`. |
| `source_of_truth` | enum | `code` / `docs` / `paper` / `domain_expert` / `multiple`. Where the facts come from. |
| `last_verified_at` | string (YYYY-MM-DD) | When a human last confirmed every field. Set when status moves to `verified`. |
| `verified_by` | string | Who verified. Free-form; usually the reviewer's handle. |

## Filename rule

`projects/<id>/entries/<entry.id>.yaml` — the filename's stem must equal
the `id` field. The validator rejects mismatches because they break
cross-reference resolution.

## Validation

Two layers:

1. **Schema validation** — `entry.schema.yaml` (JSON Schema-shaped).
   Catches missing required fields, unknown enums, malformed types.
2. **Cross-reference validation** — runs over the whole project:
   - `subsystem` exists in `subsystems.yaml`
   - `related_concepts` IDs exist in `entries/`
   - `code_anchors[].file` exists under `project.code_root`
   - `existing_doc_ref[].file` exists

Verified status requires both layers to pass.

## Example

See `projects/go-stablenet/entries/A1.wbft_core.quorum_calc.yaml` for a
complete example using every field that applies.
