# Domain Knowledge Inventory

> Per-project, human-verified knowledge corpus that CKV embeds alongside
> source code so retrieval can bridge natural-language queries to code
> identifiers without inventing the connection.

## Directory layout

```
domain-knowledge/
├── shared/            # rules that apply to every project
│   ├── SCHEMA.md
│   ├── KNOWLEDGE_TYPES.md
│   ├── STATUS_LIFECYCLE.md
│   ├── PROJECT_REGISTRATION.md
│   └── entry.schema.yaml
└── projects/
    └── <project-id>/
        ├── project.yaml
        ├── subsystems.yaml
        ├── inventory.md
        ├── glossary.yaml
        └── entries/
            └── *.yaml
```

## What lives where

| Concern | Lives in | Stable across projects? |
|---|---|---|
| Entry YAML field shape | `shared/SCHEMA.md` + `entry.schema.yaml` | yes |
| Knowledge type B1~B7 meaning | `shared/KNOWLEDGE_TYPES.md` | yes |
| Verification status lifecycle | `shared/STATUS_LIFECYCLE.md` | yes |
| How to register a new project | `shared/PROJECT_REGISTRATION.md` | yes |
| Project metadata (code root, language) | `projects/<id>/project.yaml` | no |
| Subsystem IDs A1~AN | `projects/<id>/subsystems.yaml` | no |
| Entries | `projects/<id>/entries/*.yaml` | no |
| Glossary (Korean/English alias → code keywords) | `projects/<id>/glossary.yaml` | no |
| Status dashboard | `projects/<id>/inventory.md` | no |

## Why per-project isolation

- One project must not pollute another's index.
- Korean aliases like "합의" map to different code symbols in different
  blockchains (WBFT.Finalize vs Tendermint.Commit vs HotStuff.Decide).
- Subsystem decompositions differ — go-stablenet's WBFT does not exist in
  other clients; their A1 is a different module.
- CKV/CKG indexes are built per project root; the inventory tracks which
  index a project's entries belong to.

## How CKS consumes this

CKV indexes each project's source tree plus its `entries/*.yaml` files.
Chunks carry a `project_id` tag so search results can filter or attribute
hits to a project. `glossary.yaml` is wired into the vocabulary resolver
of the active project so Korean prompts expand against the right code
keywords.

See `shared/PROJECT_REGISTRATION.md` to add a new project.
