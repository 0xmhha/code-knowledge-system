# cks-anchor-refresh

Re-resolves each domain-knowledge entry's `code_anchors[].line` from its
`symbol` against the ckg graph, and rewrites the stored line when it has
drifted. The symbol is the stable key; the line is derived. This replaces the
manual, error-prone re-verification needed every time the indexed source tree
moves and line numbers shift.

## Why

Domain-knowledge entries pin code with `{file, symbol, line}`. When go-stablenet
moves, the `line` drifts while the `symbol` stays valid. Trusting a stale line
sends a reader (or an LLM) to the wrong place. Because the ckg graph is rebuilt
to track the current tree, the current line of any indexed symbol is one
`find_symbol` away — so anchor lines should be regenerated, not hand-maintained.

## Usage

```
cks-anchor-refresh -project <project-dir> -graph <graph.db> [-check]
```

- `-project` — a domain-knowledge project directory (contains `project.yaml`
  and `entries/`), e.g. `docs/domain-knowledge/projects/go-stablenet`.
- `-graph` — the ckg graph DB the anchors resolve against, e.g.
  `data/ckg-stablenet/graph.db` (must be indexed at the commit you want as the
  baseline).
- `-check` — report drift without writing; exit non-zero if any anchor drifted
  or is unresolved (use in CI to catch stale anchors).

Example:

```
cks-anchor-refresh \
  -project docs/domain-knowledge/projects/go-stablenet \
  -graph   data/ckg-stablenet/graph.db
```

## Behaviour (conservative)

- Only anchors carrying **both** a `symbol` and a `line` are considered.
- The resolved citation must be in the **same file** as the anchor; only the
  line is rewritten. If several citations share the file, the one whose line is
  closest to the recorded line wins (so a refresh never jumps to an unrelated
  same-named symbol).
- An anchor whose symbol no longer resolves in its recorded file (moved,
  renamed, deleted, or a non-code symbol such as a doc heading or a
  package-level var/const the graph does not index) is reported `UNRESOLVED`
  and left untouched — that is a judgement call for a human, not a mechanical
  refresh.
- YAML is edited through the node tree, so comments, ordering, blank lines, and
  block styles are preserved (only the `line:` values change).

Exit codes: `0` clean/fixed · `1` drift or unresolved found · `2` usage/IO error.

## Typical workflow

1. Rebuild the ckg graph for the current source baseline.
2. `cks-anchor-refresh -project … -graph … -check` to see what drifted.
3. Run without `-check` to apply the line corrections.
4. Review the `UNRESOLVED` list by hand (real moves/renames or non-code anchors).
5. `cks-inventory-check` to validate, then promote/commit as usual.
