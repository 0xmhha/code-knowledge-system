# Registering a New Project

> Adding a project to the domain knowledge inventory is a directory
> creation + metadata declaration. No shared file needs to change.

## When to register a new project

- A new codebase needs CKV semantic search with its own domain corpus.
- An existing codebase is forked and the new fork's domain terms diverge
  enough that aliases would collide.
- A team wants an isolated index that cannot be polluted by another
  project's entries.

Do not register a project for a temporary branch or an experimental
fork — those should reuse the parent project's inventory.

## Steps

### 1. Pick the project ID

Lowercase, hyphen-separated, matches the canonical repo name when
possible. Examples: `go-stablenet`, `cosmos-sdk-app`, `polkadot-runtime`.

The ID is the directory name and the value of `project.yaml :: id`.

### 2. Create the directory

```bash
mkdir -p docs/domain-knowledge/projects/<project-id>/entries
```

### 3. Write `project.yaml`

Minimum fields:

```yaml
id: <project-id>
name: "<human-readable name>"
description: "<one-line summary>"

code_root: <absolute path to source tree>
skill_root: <absolute path to .claude or equivalent>  # optional

primary_language: <go|rust|typescript|...>
secondary_languages: []                                # optional

domain: <blockchain|backend|frontend|infra|...>
domain_tags: []                                        # free-form taxonomy

schema_version: 1

indexing:
  ckv_index_path: ".ckv-data/<project-id>"
  ckg_index_path: ".ckg-data/<project-id>"
  glossary_path: "glossary.yaml"
```

### 4. Write `subsystems.yaml`

Declare the project's `A1`~`AN` subsystem IDs. Aim for 5~15 subsystems;
fewer is hard to filter on, more is hard to remember.

```yaml
- id: A1
  name: "<subsystem name>"
  code_paths:
    - <path 1 relative to code_root>
    - <path 2>
  description: "<what lives here>"

- id: A2
  name: "..."
  code_paths: [...]
  description: "..."
```

Rules:

- IDs start with `A` for readability (the prefix has no other meaning;
  the schema only checks that the ID exists in this list).
- A subsystem must correspond to one or more concrete code paths under
  `code_root`. No imaginary subsystems.
- A code path can appear in only one subsystem (no overlap).
- Add new subsystems at the end with the next ID — never reorder.

### 5. Create empty `inventory.md` and `glossary.yaml`

`inventory.md` is the human dashboard; it gets populated as entries are
added. Initialize with the boilerplate header:

```markdown
# <project name> — Domain Knowledge Inventory

Project: `<project-id>`
Schema version: 1

| Status | Count |
|---|---|
| verified | 0 |
| needs_verification | 0 |
| draft | 0 |
| needs_author | 0 |
```

`glossary.yaml` initializes to:

```yaml
# Populated automatically from entries[*].korean_aliases /
# english_aliases / code_keywords. Hand-edit only when the auto-derived
# entry is wrong; the next regeneration will preserve hand-edits flagged
# with `pinned: true`.
version: 1
entries: []
```

### 6. Register with CKS

Add the project to CKS config so CKV/CKG/glossary are wired:

```yaml
# cks.yaml
projects:
  - id: <project-id>
    code_root: <absolute path>
    skill_root: <absolute path>
    ckv_path: .ckv-data/<project-id>
    ckg_path: .ckg-data/<project-id>
    glossary: docs/domain-knowledge/projects/<project-id>/glossary.yaml

# If this becomes the working project for the current session:
active_project: <project-id>
```

### 7. Bootstrap indexes (when ckv/ckg are real)

```bash
ckv build --src=<code_root> --out=.ckv-data/<project-id>
ckg build --src=<code_root> --out=.ckg-data/<project-id>
```

For the dummy phase, this step is a no-op; the Smart Dummy reads
`project.code_root` directly when it answers retrieval instructions.

### 8. Start writing entries

First entries should always be P0 entries in the most-frequently-used
subsystems. For a blockchain client that usually means the consensus
core and the genesis/bootstrap flow.

Each entry lives at `entries/<id>.yaml` with `status: needs_author` or
`draft` until a reviewer verifies it.

## Common mistakes

| Mistake | Effect |
|---|---|
| Reusing a project ID for a fork that diverges | Aliases collide; retrieval returns mixed results |
| Subsystem `code_paths` overlap | An entry could legitimately belong to two subsystems; cross-references break |
| Renaming a subsystem ID after entries reference it | `subsystem` validation fails on every dependent entry |
| Skipping `code_root` and using relative anchors | Validator cannot check anchor existence |
| Setting `status: verified` to skip verification | `last_verified_at` empty → next CI run fails the entry |

## Project lifecycle states

A project is one of:

- **active** — has entries; CKS knows about it; index is built or being
  built
- **dormant** — registered but no recent entry activity. Keep around if
  the project still ships code.
- **archived** — codebase retired. Move the project directory under
  `docs/domain-knowledge/_archived/` and remove from CKS config.

Archived projects are kept on disk because past entries are still useful
as reference when porting concepts to new projects.
