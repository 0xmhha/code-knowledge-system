# cks-glossary-gen

Builds a project's `glossary.yaml` from its domain-knowledge `entries/*.yaml`.

The resulting file is loadable by `internal/vocab.Load`; the same file
is consumed by composer Stage 1 when `vocab.glossary_path` is set in
`cks.yaml`.

## Default behavior

By default the generator includes only entries with `status: verified`.
This matches the gate `ckv build` will use once it ships, so the
glossary the resolver loads at runtime is always a strict subset of
what the index actually contains.

During development — when most entries are still `needs_verification`
— pass `-status=needs_verification` or `-status=all` to widen the gate.

## Usage

```bash
go run ./cmd/cks-glossary-gen \
    -project docs/domain-knowledge/projects/go-stablenet \
    -out    docs/domain-knowledge/projects/go-stablenet/glossary.yaml \
    -status verified
```

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `-project` | (required) | project directory containing `entries/` |
| `-out` | `<project>/glossary.yaml` | output path |
| `-status` | `verified` | gate; `verified` / `needs_verification` / `draft` / `all` |
| `-dry-run` | false | print counts to stderr, do not write the output file |

## Output format

```yaml
version: 1
entries:
  - aliases: [<korean and english alias union, deduped>]
    canonical: <entry id, e.g. A1.wbft_core.quorum_calc>
    code_keywords: [<entry's code_keywords, deduped, order preserved>]
```

An entry is skipped — counted toward `skipped` in the stderr summary —
when it has no aliases or no code keywords; those entries cannot serve
the resolver and would only add load with no upside.

## Regenerate when

- A reviewer marks one or more entries `verified` (the gate's "in" set
  grows).
- An author edits a verified entry's `korean_aliases`,
  `english_aliases`, or `code_keywords`.

CI integration is deferred — for now `cks-mcp` operators rerun the
generator manually before restarting the server when the inventory
changes.
