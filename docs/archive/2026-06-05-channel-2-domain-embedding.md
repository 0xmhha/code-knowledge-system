# Channel ② Domain-Knowledge Embedding Implementation Plan

> **ARCHIVED 2026-07-19.** Plan executed (`internal/domainexport`, `config.DomainConfig` + `cks.ops.index`, `budget.DocsRoots`, `cmd/cks-domain-export`). Design record: [`../../superpowers/specs/2026-06-05-channel-2-domain-embedding-design.md`](../../superpowers/specs/2026-06-05-channel-2-domain-embedding-design.md).

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed cks domain-knowledge entries and the project's authoritative docs as first-class markdown documents in the same ckv vector index, so `semantic_search` returns the curated knowledge itself.

**Architecture:** cks renders entries → a markdown corpus and copies the project.yaml authoritative docs into it; ckv gains an additive `build --docs <dir>` flag that walks extra markdown roots and tags those chunks `Category="domain"`; `cks.ops.index` orchestrates export → `ckv build --docs`. One index over the go-stablenet `--src` tree + the cks corpus.

**Tech Stack:** Go, cobra (ckv CLI), mcp-go (cks tool), sqlite-vec store, yaml.v3, the mock embedder for tests.

**Repos & order:** Phase A = ckv (repo `code-knowledge-vector`, new branch `feat-build-docs-flag`) ships first. Phase B = cks (repo `code-knowledge-system`, branch `p0b-channel2-domain-embedding`, already holds the spec) depends on the ckv `--docs` flag.

**Spec:** `code-knowledge-system/docs/superpowers/specs/2026-06-05-channel-2-domain-embedding-design.md`

---

## File Structure

**ckv (`code-knowledge-vector`)**
- Modify `internal/manifest/manifest.go` — add `DocsRoots []string` (additive, same schema version).
- Modify `internal/build/builder.go` — add `Options.DocsRoots`, index docs roots tagged `Category="domain"`, record DocsRoots in manifest.
- Modify `cmd/ckv/build.go` — add the `--docs` repeatable flag, thread to `build.Options`.
- Test: `internal/manifest/manifest_test.go`, `internal/build/builder_test.go`, `cmd/ckv/build_test.go`.

**cks (`code-knowledge-system`)**
- Modify `internal/inventory/types.go` — add `AuthoritativeDoc` + `Project.AuthoritativeDocs`.
- Modify `internal/inventory/load.go` — parse `authoritative_docs` into the project.
- Create `internal/domainexport/export.go` — `RenderEntry` + `Export` (single responsibility: turn a Project into the corpus).
- Create `cmd/cks-domain-export/main.go` — thin CLI over `domainexport.Export`.
- Modify `internal/mcp/ops_index.go` — `IndexConfig.DomainProjectDir/DomainCorpusDir`, run export before ckv, pass `--docs` on full builds.
- Modify `internal/config/config.go` + `cmd/cks-mcp/main.go` — config block + wiring into `IndexConfig`.
- Modify `.gitignore` — ignore the generated corpus.
- Test: `internal/inventory/load_authdocs_test.go`, `internal/domainexport/export_test.go`, `internal/mcp/ops_index_test.go`.

---

## Phase A — ckv `build --docs` (repo: code-knowledge-vector)

Branch setup (run once):

```bash
cd /Users/wm-it-22-00661/Work/github/tools/code-knowledge-vector
git checkout main && git pull --ff-only
git checkout -b feat-build-docs-flag
```

### Task A1: manifest records DocsRoots

**Files:**
- Modify: `internal/manifest/manifest.go:47` (after the `CKVIgnore` field)
- Test: `internal/manifest/manifest_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/manifest/manifest_test.go`:

```go
func TestSaveLoad_DocsRootsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := &Manifest{
		SchemaVersion:  SchemaVersionCurrent,
		EmbeddingModel: "mock",
		EmbeddingDim:   8,
		DocsRoots:      []string{"/abs/corpus/go-stablenet"},
	}
	if err := Save(dir, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out.DocsRoots) != 1 || out.DocsRoots[0] != "/abs/corpus/go-stablenet" {
		t.Errorf("DocsRoots round-trip = %v, want [/abs/corpus/go-stablenet]", out.DocsRoots)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/manifest/ -run TestSaveLoad_DocsRootsRoundTrip -v`
Expected: FAIL — `Manifest` has no field `DocsRoots`.

- [ ] **Step 3: Add the field**

In `internal/manifest/manifest.go`, after the `CKVIgnore` field (line 47):

```go
	// Ignore patterns surfaced for transparency
	CKVIgnore []string `json:"ckvignore,omitempty"`

	// DocsRoots are additional markdown corpus directories indexed via
	// `ckv build --docs` (outside SrcRoot, e.g. a cks-rendered
	// domain-knowledge corpus). Recorded so callers can see every source
	// the index covers. Additive — old readers see nil.
	DocsRoots []string `json:"docs_roots,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/manifest/ -run TestSaveLoad_DocsRootsRoundTrip -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/manifest/manifest.go internal/manifest/manifest_test.go
git commit -m "Add DocsRoots to the manifest"
```

### Task A2: build indexes `--docs` roots as domain-tagged markdown

**Files:**
- Modify: `internal/build/builder.go:62` (Options), `:314` (after convention chunks, before `builtAt`), `:342` (manifest population), and a helper near `absOrEmpty`.
- Test: `internal/build/builder_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/build/builder_test.go`:

```go
func TestRunIndexesDocsRoots(t *testing.T) {
	src := resolveTestdataSample(t)
	docs := t.TempDir()
	if err := os.WriteFile(filepath.Join(docs, "A4.addresses.md"),
		[]byte("# System contract addresses\n\nNativeCoinAdapter is 0x1000.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()

	_, err := Run(context.Background(), Options{
		SrcRoot:   src,
		OutDir:    out,
		DocsRoots: []string{docs},
		Embedder:  mock.Default(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The corpus markdown must be indexed and tagged Category="domain",
	// cited by its path relative to the docs root.
	store, err := sqlitevec.Open(filepath.Join(out, "vector.db"), mock.Default().Dimension())
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store.Close()

	q, _ := mock.Default().Embed(context.Background(), []string{"system contract address NativeCoinAdapter"})
	hits, err := store.Search(context.Background(), q[0], 50, types.Filter{Language: "markdown"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var found bool
	for _, h := range hits {
		if h.Chunk.File == "A4.addresses.md" {
			found = true
			if h.Chunk.Category != "domain" {
				t.Errorf("docs chunk Category = %q, want domain", h.Chunk.Category)
			}
		}
	}
	if !found {
		t.Fatalf("corpus doc A4.addresses.md not indexed (got %d markdown hits)", len(hits))
	}

	m, err := manifest.Load(out)
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}
	if len(m.DocsRoots) != 1 {
		t.Errorf("manifest DocsRoots = %v, want one entry", m.DocsRoots)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/build/ -run TestRunIndexesDocsRoots -v`
Expected: FAIL — `Options` has no field `DocsRoots`.

- [ ] **Step 3: Add the Options field**

In `internal/build/builder.go`, inside `Options` after `PolicyPath` (line 62):

```go
	PolicyPath string

	// DocsRoots are extra directories walked for markdown AFTER SrcRoot.
	// Files found here are tagged Category="domain" and cited by their
	// path relative to the docs root. Used to embed an out-of-tree curated
	// corpus (the cks domain-knowledge entries + authoritative docs) in
	// the same index. These roots are not git repos, so chunks carry no
	// commit hash.
	DocsRoots []string
```

- [ ] **Step 4: Index the docs roots**

In `internal/build/builder.go`, after the convention-chunks block (the `if convChunks := ...` block ending near line 314) and before `builtAt := o.Now().UTC()...`:

```go
	// --docs: index additional markdown corpora living outside SrcRoot.
	// Not a git repo (commit=""); tagged Category="domain" so callers can
	// tell curated knowledge from code. processFile handles markdown the
	// same as in-tree docs.
	for _, docsRoot := range o.DocsRoots {
		docFiles, docWalkErrs, werr := discover.Walk(docsRoot, discover.Options{})
		if werr != nil {
			return nil, fmt.Errorf("walk docs %q: %w", docsRoot, werr)
		}
		for _, e := range docWalkErrs {
			fmt.Fprintf(os.Stderr, "ckv: docs walk warning: %v\n", e)
		}
		for _, f := range docFiles {
			chunks, perr := processFile(f.AbsPath, f.RelPath, f.Language, "", parsers, cfg, chunker)
			if perr != nil {
				fmt.Fprintf(os.Stderr, "ckv: %v\n", perr)
				continue
			}
			if len(chunks) == 0 {
				continue
			}
			for i := range chunks {
				chunks[i].Category = "domain"
			}
			if err := embedAndUpsert(ctx, store, o.Embedder, chunks, o.BatchSize, memSig, embedTextFn); err != nil {
				return nil, fmt.Errorf("embed/upsert docs %s: %w", f.RelPath, err)
			}
			indexedFiles++
			languageCounts[f.Language] += len(chunks)
			accumulateStats(&totalStats, chunks)
		}
	}
```

- [ ] **Step 5: Record DocsRoots in the manifest + add the helper**

In `internal/build/builder.go`, in the `manifest.Manifest{...}` literal after `CKVIgnore: o.CKVIgnore,` (line 342):

```go
		CKVIgnore:          o.CKVIgnore,
		DocsRoots:          absRoots(o.DocsRoots),
```

And add next to `absOrEmpty` (end of file):

```go
// absRoots returns the absolute form of each root, preserving order.
// nil in → nil out so the manifest field stays omitted when unused.
func absRoots(roots []string) []string {
	if len(roots) == 0 {
		return nil
	}
	out := make([]string, len(roots))
	for i, r := range roots {
		out[i] = absOrEmpty(r)
	}
	return out
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/build/ -run TestRunIndexesDocsRoots -v`
Expected: PASS

- [ ] **Step 7: Run the package tests (no regressions)**

Run: `go test ./internal/build/ ./internal/manifest/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/build/builder.go internal/build/builder_test.go
git commit -m "Index --docs corpus roots as domain-tagged markdown"
```

### Task A3: `ckv build --docs` flag

**Files:**
- Modify: `cmd/ckv/build.go:29` (buildOpts), `:58` (flags), `:95` (build.Options)
- Test: `cmd/ckv/build_test.go`

- [ ] **Step 1: Write the failing test**

Create or append `cmd/ckv/build_test.go`:

```go
package main

import "testing"

func TestBuildCmd_HasDocsFlag(t *testing.T) {
	cmd := newBuildCmd()
	f := cmd.Flags().Lookup("docs")
	if f == nil {
		t.Fatal("build command missing --docs flag")
	}
	if f.Value.Type() != "stringSlice" {
		t.Errorf("--docs type = %q, want stringSlice", f.Value.Type())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ckv/ -run TestBuildCmd_HasDocsFlag -v`
Expected: FAIL — flag `docs` not found.

- [ ] **Step 3: Add the flag and thread it**

In `cmd/ckv/build.go`, add to `buildOpts` (after `policy string`, line 23):

```go
	policy    string
	docs      []string
```

Add the flag registration (after the `--policy` flag, line 54):

```go
	f.StringSliceVar(&opts.docs, "docs", nil, "additional markdown corpus dirs to embed in the same index (repeatable; chunks tagged Category=domain; e.g. --docs=generated/domain-corpus/go-stablenet)")
```

Pass it into `build.Options` (in the `buildOpts := build.Options{...}` literal, after `PolicyPath: opts.policy,` line 94):

```go
		PolicyPath:              opts.policy,
		DocsRoots:               opts.docs,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/ckv/ -run TestBuildCmd_HasDocsFlag -v`
Expected: PASS

- [ ] **Step 5: Build the binary and smoke-test help**

Run: `go build ./... && go run ./cmd/ckv build --help | grep -- --docs`
Expected: the `--docs` line prints.

- [ ] **Step 6: Commit**

```bash
git add cmd/ckv/build.go cmd/ckv/build_test.go
git commit -m "Add ckv build --docs flag"
```

- [ ] **Step 7: Push and open the ckv PR**

```bash
git push -u origin feat-build-docs-flag
gh pr create --title "Add ckv build --docs flag for out-of-tree markdown corpora" \
  --body "Adds an additive \`--docs <dir>\` flag (repeatable) to \`ckv build\`. Each docs root is walked for markdown after \`--src\`; its chunks are tagged \`Category=\"domain\"\` and cited by their path relative to the docs root. The manifest records \`docs_roots\`. Enables embedding a curated domain-knowledge corpus in the same index as code. No change to existing builds (flag defaults to none)."
```

**Phase A merge gate:** the ckv PR must be merged before cks Task B5 (orchestration shells `ckv build --docs`). Tasks B1–B4 do not depend on it and can proceed in parallel.

---

## Phase B — cks corpus export + orchestration (repo: code-knowledge-system)

Branch already exists (`p0b-channel2-domain-embedding`, holds the spec). Continue on it.

```bash
cd /Users/wm-it-22-00661/Work/github/tools/code-knowledge-system
git status   # expect: on branch p0b-channel2-domain-embedding
```

### Task B1: parse `authoritative_docs` into the project

**Files:**
- Modify: `internal/inventory/types.go:130` (after `DocRef`), `:55` (Project)
- Modify: `internal/inventory/load.go:21` (projectFile), `:83` (Project literal)
- Test: `internal/inventory/load_authdocs_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/inventory/load_authdocs_test.go`:

```go
package inventory

import (
	"path/filepath"
	"testing"
)

func TestLoadProject_AuthoritativeDocs(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "entries"))
	mustWrite(t, filepath.Join(dir, "project.yaml"),
		"id: s\nname: s\nschema_version: 1\n"+
			"authoritative_docs:\n"+
			"  - file: CLAUDE.md\n    role: overview\n"+
			"  - file: .claude/docs/wbft-consensus.md\n    role: consensus\n")
	mustWrite(t, filepath.Join(dir, "subsystems.yaml"),
		"- id: A1\n  name: x\n  description: x\n  code_paths:\n    - .\n")
	mustWrite(t, filepath.Join(dir, "entries", "A1.e.f.yaml"),
		"id: A1.e.f\nsubsystem: A1\nknowledge_type: B1\ntitle: T\n"+
			"summary: long enough summary\nstatus: draft\npriority: P0\n")

	p, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if len(p.AuthoritativeDocs) != 2 {
		t.Fatalf("AuthoritativeDocs len = %d, want 2", len(p.AuthoritativeDocs))
	}
	if p.AuthoritativeDocs[0].File != "CLAUDE.md" || p.AuthoritativeDocs[0].Role != "overview" {
		t.Errorf("doc[0] = %+v", p.AuthoritativeDocs[0])
	}
	if p.AuthoritativeDocs[1].File != ".claude/docs/wbft-consensus.md" {
		t.Errorf("doc[1].File = %q", p.AuthoritativeDocs[1].File)
	}
}
```

> Note: `mustMkdir` / `mustWrite` already exist in the package's test helpers (used by `load_env_test.go`). Reuse them.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/inventory/ -run TestLoadProject_AuthoritativeDocs -v`
Expected: FAIL — `Project` has no field `AuthoritativeDocs`.

- [ ] **Step 3: Add the type and Project field**

In `internal/inventory/types.go`, after the `DocRef` struct (line 130):

```go
// AuthoritativeDoc is one entry in project.yaml's authoritative_docs:
// a curated, human-maintained document (path relative to CodeRoot) that
// the domain-knowledge corpus embeds wholesale. Role is a short label of
// what the doc covers.
type AuthoritativeDoc struct {
	File string `yaml:"file"`
	Role string `yaml:"role,omitempty"`
}
```

In the `Project` struct, after the `Entries` field (line 54):

```go
	Entries map[string]Entry

	// AuthoritativeDocs mirrors project.yaml's authoritative_docs: the
	// curated docs (relative to CodeRoot) the embedding corpus copies in.
	AuthoritativeDocs []AuthoritativeDoc
```

- [ ] **Step 4: Parse it in the loader**

In `internal/inventory/load.go`, extend `projectFile` (after `SchemaVersion`, line 20):

```go
	CodeRoot      string `yaml:"code_root"`
	SchemaVersion int    `yaml:"schema_version"`
	AuthoritativeDocs []AuthoritativeDoc `yaml:"authoritative_docs,omitempty"`
```

In the returned `&Project{...}` literal (after `Entries: entries,`, line 82):

```go
		Entries:        entries,
		AuthoritativeDocs: pf.AuthoritativeDocs,
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/inventory/ -run TestLoadProject_AuthoritativeDocs -v`
Expected: PASS

- [ ] **Step 6: Run the package tests (no regressions)**

Run: `go test ./internal/inventory/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/inventory/types.go internal/inventory/load.go internal/inventory/load_authdocs_test.go
git commit -m "Parse authoritative_docs into the loaded project"
```

### Task B2: render one entry to markdown

**Files:**
- Create: `internal/domainexport/export.go`
- Test: `internal/domainexport/export_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domainexport/export_test.go`:

```go
package domainexport

import (
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func sampleProject() *inventory.Project {
	return &inventory.Project{
		Subsystems: map[string]inventory.Subsystem{
			"A4": {ID: "A4", Name: "System Contracts"},
		},
	}
}

func TestRenderEntry_FullEntry(t *testing.T) {
	e := inventory.Entry{
		ID: "A4.system_contracts.addresses", Subsystem: "A4", KnowledgeType: "B7",
		Title: "System contract addresses", Status: "verified",
		Summary:    "Five governance contracts at fixed addresses.",
		Invariants: []string{"NativeCoinAdapter is 0x1000."},
		Pitfalls:   []string{"Do not hardcode 0xB00002 elsewhere."},
		CodeAnchors: []inventory.CodeAnchor{
			{File: "params/config_wbft.go", Symbol: "DefaultGovMinterAddress", Line: 41, Reason: "GovMinter"},
		},
		EnglishAliases: []string{"system contract addresses"},
		CodeKeywords:   []string{"DefaultGovMinterAddress"},
		RelatedConcepts: []string{"A5.account_extra.bit_layout"},
	}
	md := RenderEntry(e, sampleProject())

	for _, want := range []string{
		"# System contract addresses",
		"**Status:** verified · **Subsystem:** A4 (System Contracts) · **Type:** B7",
		"Five governance contracts at fixed addresses.",
		"## Invariants\n- NativeCoinAdapter is 0x1000.",
		"## Pitfalls\n- Do not hardcode 0xB00002 elsewhere.",
		"`params/config_wbft.go` DefaultGovMinterAddress:41 — GovMinter",
		"## Aliases\nsystem contract addresses, DefaultGovMinterAddress",
		"## Related\nA5.account_extra.bit_layout",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered markdown missing %q\n---\n%s", want, md)
		}
	}
}

func TestRenderEntry_OmitsEmptySections(t *testing.T) {
	e := inventory.Entry{
		ID: "A1.x", Subsystem: "A1", KnowledgeType: "B1",
		Title: "Minimal", Status: "needs_verification", Summary: "Just a summary.",
	}
	md := RenderEntry(e, &inventory.Project{Subsystems: map[string]inventory.Subsystem{}})
	if strings.Contains(md, "## Invariants") || strings.Contains(md, "## Pitfalls") ||
		strings.Contains(md, "## Code anchors") || strings.Contains(md, "## Aliases") {
		t.Errorf("empty sections should be omitted:\n%s", md)
	}
	if !strings.Contains(md, "**Status:** needs_verification") {
		t.Errorf("status line missing:\n%s", md)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domainexport/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the renderer**

Create `internal/domainexport/export.go`:

```go
// Package domainexport renders a domain-knowledge Project into a markdown
// corpus that ckv embeds via `ckv build --docs`. It is the producer side
// of channel ② (see docs/superpowers/specs/2026-06-05-channel-2-domain-
// embedding-design.md): one markdown file per embeddable entry plus copies
// of the project's authoritative docs.
package domainexport

import (
	"fmt"
	"sort"
	"strings"

	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

// RenderEntry turns one entry into a markdown document for embedding. The
// Status line surfaces the entry's confidence at retrieval time; empty
// sections are omitted. p supplies the subsystem's human name.
func RenderEntry(e inventory.Entry, p *inventory.Project) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", e.Title)

	subName := e.Subsystem
	if s, ok := p.Subsystems[e.Subsystem]; ok && s.Name != "" {
		subName = fmt.Sprintf("%s (%s)", e.Subsystem, s.Name)
	}
	fmt.Fprintf(&b, "**Status:** %s · **Subsystem:** %s · **Type:** %s\n\n", e.Status, subName, e.KnowledgeType)

	if strings.TrimSpace(e.Summary) != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimRight(e.Summary, "\n"))
	}
	if len(e.Invariants) > 0 {
		b.WriteString("## Invariants\n")
		for _, s := range e.Invariants {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n")
	}
	if len(e.Pitfalls) > 0 {
		b.WriteString("## Pitfalls\n")
		for _, s := range e.Pitfalls {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n")
	}
	if len(e.CodeAnchors) > 0 {
		b.WriteString("## Code anchors\n")
		for _, a := range e.CodeAnchors {
			line := "- `" + a.File + "`"
			if a.Symbol != "" {
				line += " " + a.Symbol
			}
			if a.Line > 0 {
				line += fmt.Sprintf(":%d", a.Line)
			}
			if a.Reason != "" {
				line += " — " + a.Reason
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	aliases := append([]string{}, e.KoreanAliases...)
	aliases = append(aliases, e.EnglishAliases...)
	aliases = append(aliases, e.CodeKeywords...)
	if len(aliases) > 0 {
		fmt.Fprintf(&b, "## Aliases\n%s\n\n", strings.Join(aliases, ", "))
	}
	if len(e.RelatedConcepts) > 0 {
		rel := append([]string{}, e.RelatedConcepts...)
		sort.Strings(rel)
		fmt.Fprintf(&b, "## Related\n%s\n", strings.Join(rel, ", "))
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domainexport/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domainexport/export.go internal/domainexport/export_test.go
git commit -m "Render a domain entry to markdown"
```

### Task B3: export the corpus (gating + authoritative-doc copy)

**Files:**
- Modify: `internal/domainexport/export.go` (append `Export`, `Result`, `embeddableStatuses`)
- Test: `internal/domainexport/export_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/domainexport/export_test.go`:

```go
import (
	"os"
	"path/filepath"
)
// (merge these imports into the existing import block)

func TestExport_GatingAndDocs(t *testing.T) {
	codeRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(codeRoot, "CLAUDE.md"), []byte("# overview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &inventory.Project{
		CodeRoot:   codeRoot,
		Subsystems: map[string]inventory.Subsystem{"A1": {ID: "A1", Name: "Core"}},
		Entries: map[string]inventory.Entry{
			"A1.v":  {ID: "A1.v", Subsystem: "A1", KnowledgeType: "B1", Title: "V", Status: "verified", Summary: "s"},
			"A1.nv": {ID: "A1.nv", Subsystem: "A1", KnowledgeType: "B1", Title: "NV", Status: "needs_verification", Summary: "s"},
			"A1.d":  {ID: "A1.d", Subsystem: "A1", KnowledgeType: "B1", Title: "D", Status: "draft", Summary: "s"},
			"A1.na": {ID: "A1.na", Subsystem: "A1", KnowledgeType: "B1", Title: "NA", Status: "needs_author", Summary: "s"},
		},
		AuthoritativeDocs: []inventory.AuthoritativeDoc{
			{File: "CLAUDE.md", Role: "overview"},
			{File: ".claude/docs/missing.md", Role: "absent"},
		},
	}
	out := t.TempDir()
	res, err := Export(p, out)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	// Gating: only verified + needs_verification produce entry docs.
	if res.EntriesWritten != 2 {
		t.Errorf("EntriesWritten = %d, want 2", res.EntriesWritten)
	}
	for _, id := range []string{"A1.v", "A1.nv"} {
		if _, err := os.Stat(filepath.Join(out, "entries", id+".md")); err != nil {
			t.Errorf("expected entries/%s.md: %v", id, err)
		}
	}
	for _, id := range []string{"A1.d", "A1.na"} {
		if _, err := os.Stat(filepath.Join(out, "entries", id+".md")); !os.IsNotExist(err) {
			t.Errorf("entries/%s.md should not exist", id)
		}
	}
	// Authoritative docs: existing copied, missing warned + skipped.
	if res.DocsCopied != 1 {
		t.Errorf("DocsCopied = %d, want 1", res.DocsCopied)
	}
	if _, err := os.Stat(filepath.Join(out, "docs", "CLAUDE.md")); err != nil {
		t.Errorf("expected docs/CLAUDE.md: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "missing.md") {
		t.Errorf("expected one warning about missing.md, got %v", res.Warnings)
	}
}

func TestExport_Deterministic(t *testing.T) {
	p := &inventory.Project{
		Subsystems: map[string]inventory.Subsystem{"A1": {ID: "A1", Name: "Core"}},
		Entries: map[string]inventory.Entry{
			"A1.v": {ID: "A1.v", Subsystem: "A1", KnowledgeType: "B1", Title: "V", Status: "verified", Summary: "s"},
		},
	}
	a, b := t.TempDir(), t.TempDir()
	if _, err := Export(p, a); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(p, b); err != nil {
		t.Fatal(err)
	}
	da, _ := os.ReadFile(filepath.Join(a, "entries", "A1.v.md"))
	db, _ := os.ReadFile(filepath.Join(b, "entries", "A1.v.md"))
	if string(da) != string(db) {
		t.Errorf("export not deterministic:\nA=%s\nB=%s", da, db)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domainexport/ -run TestExport -v`
Expected: FAIL — `Export` / `Result` undefined.

- [ ] **Step 3: Implement Export**

Append to `internal/domainexport/export.go` (and add `os`, `path/filepath` to the import block):

```go
// embeddableStatuses gates which entries are rendered. Per channel-② D2:
// verified + needs_verification (each doc shows its status); draft and
// needs_author are excluded as not-yet-trustworthy.
var embeddableStatuses = map[string]bool{
	"verified":           true,
	"needs_verification": true,
}

// Result reports what Export produced.
type Result struct {
	EntriesWritten int
	DocsCopied     int
	Warnings       []string
}

// Export writes the embedding corpus for p into outDir:
//   - entries/<id>.md   for each entry whose status is embeddable
//   - docs/<basename>.md copies of p.AuthoritativeDocs resolved under CodeRoot
//
// Output is deterministic (entries in sorted ID order). A missing
// authoritative doc, or an unset CodeRoot, is warned and skipped rather
// than fatal — the entry corpus still ships.
func Export(p *inventory.Project, outDir string) (Result, error) {
	var res Result
	entriesDir := filepath.Join(outDir, "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		return res, fmt.Errorf("domainexport: mkdir entries: %w", err)
	}
	for _, id := range p.EntryIDsSorted() {
		e := p.Entries[id]
		if !embeddableStatuses[e.Status] {
			continue
		}
		path := filepath.Join(entriesDir, e.ID+".md")
		if err := os.WriteFile(path, []byte(RenderEntry(e, p)), 0o644); err != nil {
			return res, fmt.Errorf("domainexport: write %s: %w", path, err)
		}
		res.EntriesWritten++
	}

	if len(p.AuthoritativeDocs) > 0 {
		docsDir := filepath.Join(outDir, "docs")
		if err := os.MkdirAll(docsDir, 0o755); err != nil {
			return res, fmt.Errorf("domainexport: mkdir docs: %w", err)
		}
		for _, ad := range p.AuthoritativeDocs {
			if p.CodeRoot == "" {
				res.Warnings = append(res.Warnings, "code_root unset; skipping authoritative_docs copy")
				break
			}
			src := filepath.Join(p.CodeRoot, ad.File)
			data, err := os.ReadFile(src)
			if err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("authoritative_doc %q: %v (skipped)", ad.File, err))
				continue
			}
			dst := filepath.Join(docsDir, filepath.Base(ad.File))
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return res, fmt.Errorf("domainexport: write %s: %w", dst, err)
			}
			res.DocsCopied++
		}
	}
	return res, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domainexport/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domainexport/export.go internal/domainexport/export_test.go
git commit -m "Export the domain corpus with status gating and authoritative-doc copy"
```

### Task B4: `cks-domain-export` CLI

**Files:**
- Create: `cmd/cks-domain-export/main.go`
- Test: manual smoke (no unit test — thin CLI over tested `Export`)

- [ ] **Step 1: Write the command**

Create `cmd/cks-domain-export/main.go`:

```go
// Command cks-domain-export renders a project's domain-knowledge entries
// (status verified/needs_verification) plus its authoritative_docs into a
// markdown corpus that `ckv build --docs <out>` embeds. This is the
// producer side of channel ②.
//
// Usage:
//
//	cks-domain-export -project docs/domain-knowledge/projects/go-stablenet \
//	  -out generated/domain-corpus/go-stablenet
//
// code_root for authoritative_docs resolves via CKS_CODE_ROOT or the
// project.yaml ${GO_STABLENET_ROOT} env, same as the validator.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/0xmhha/code-knowledge-system/internal/domainexport"
	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func main() {
	projectDir := flag.String("project", "", "project directory (contains project.yaml, subsystems.yaml, entries/)")
	outDir := flag.String("out", "", "output corpus directory")
	flag.Parse()

	if *projectDir == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "cks-domain-export: -project and -out are required")
		flag.Usage()
		os.Exit(2)
	}

	p, err := inventory.LoadProject(*projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cks-domain-export: %v\n", err)
		os.Exit(1)
	}
	res, err := domainexport.Export(p, *outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cks-domain-export: %v\n", err)
		os.Exit(1)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "cks-domain-export: warning: %s\n", w)
	}
	fmt.Printf("cks-domain-export: %d entries, %d docs -> %s\n", res.EntriesWritten, res.DocsCopied, *outDir)
}
```

- [ ] **Step 2: Build and smoke-test against go-stablenet**

```bash
export GO_STABLENET_ROOT=/Users/wm-it-22-00661/Work/github/tools/go-stablenet
go run ./cmd/cks-domain-export \
  -project docs/domain-knowledge/projects/go-stablenet \
  -out generated/domain-corpus/go-stablenet
ls generated/domain-corpus/go-stablenet/entries | head
ls generated/domain-corpus/go-stablenet/docs
```

Expected: prints a count (≥9 entries — the 7 verified + the needs_verification ones; ≥6 docs); `entries/` and `docs/` populated.

- [ ] **Step 3: Commit**

```bash
git add cmd/cks-domain-export/main.go
git commit -m "Add the cks-domain-export command"
```

### Task B5: gitignore the corpus + orchestrate in cks.ops.index

**Files:**
- Modify: `.gitignore`
- Modify: `internal/mcp/ops_index.go:29` (IndexConfig), `:96` (handleOpsIndex), `:142` (ckvIndexArgs)
- Modify: `internal/config/config.go` (a `domain` config block) + `cmd/cks-mcp/main.go` (wire into IndexConfig)
- Test: `internal/mcp/ops_index_test.go`

- [ ] **Step 1: Ignore the generated corpus**

Append to `.gitignore`:

```gitignore
# Generated domain-knowledge embedding corpus (channel ②; rebuilt by cks-domain-export)
/generated/
```

- [ ] **Step 2: Write the failing test**

Create `internal/mcp/ops_index_test.go`:

```go
package mcp

import (
	"strings"
	"testing"
)

func TestCKVIndexArgs_FullIncludesDocs(t *testing.T) {
	ic := IndexConfig{
		CKVDataPath:     "./ckv-stablenet",
		SourceRoot:      "/src",
		EmbedModel:      "bge-m3",
		DomainCorpusDir: "generated/domain-corpus/go-stablenet",
	}
	args := ckvIndexArgs(ic, "full", "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--docs generated/domain-corpus/go-stablenet") {
		t.Errorf("full build args missing --docs: %v", args)
	}
}

func TestCKVIndexArgs_IncrementalOmitsDocs(t *testing.T) {
	ic := IndexConfig{CKVDataPath: "./ckv-stablenet", DomainCorpusDir: "generated/corpus"}
	args := ckvIndexArgs(ic, "incremental", "")
	if strings.Contains(strings.Join(args, " "), "--docs") {
		t.Errorf("incremental (reindex) must not pass --docs: %v", args)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestCKVIndexArgs -v`
Expected: FAIL — `IndexConfig` has no `DomainCorpusDir`.

- [ ] **Step 4: Extend IndexConfig and ckvIndexArgs**

In `internal/mcp/ops_index.go`, add to `IndexConfig` (after `CKGPolicyFile`, line 28):

```go
	CKGPolicyFile string // ckg --policy-file (governed_by edges); "" omits the flag

	// Channel ②: DomainProjectDir is the cks domain-knowledge project dir
	// to export before building; DomainCorpusDir is the export output AND
	// the ckv --docs root. Both empty disables channel ② (no corpus step).
	DomainProjectDir string
	DomainCorpusDir  string
```

In `ckvIndexArgs`, the full-build branch (line 142):

```go
	if mode == "full" {
		args := []string{"build", "--src", ic.SourceRoot, "--out", ic.CKVDataPath}
		if ic.DomainCorpusDir != "" {
			args = append(args, "--docs", ic.DomainCorpusDir)
		}
		return append(args, embed...)
	}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run TestCKVIndexArgs -v`
Expected: PASS

- [ ] **Step 6: Run the export before the ckv build**

In `internal/mcp/ops_index.go`, add the import:

```go
import (
	"context"
	"fmt"
	"os/exec"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/domainexport"
	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)
```

In `handleOpsIndex`, immediately before `if ic.CKVBinary != "" {` (line 96):

```go
	// Channel ②: regenerate the corpus so the ckv build below (--docs)
	// embeds the latest entries + authoritative docs. Disabled when the
	// project dir is unset.
	if ic.DomainProjectDir != "" && ic.DomainCorpusDir != "" {
		proj, err := inventory.LoadProject(ic.DomainProjectDir)
		if err != nil {
			resp.CKV.Error = fmt.Sprintf("domain export: load project: %v", err)
			return mcpgo.NewToolResultStructured(resp, "index refresh FAILED (domain export)"), nil
		}
		if _, err := domainexport.Export(proj, ic.DomainCorpusDir); err != nil {
			resp.CKV.Error = fmt.Sprintf("domain export: %v", err)
			return mcpgo.NewToolResultStructured(resp, "index refresh FAILED (domain export)"), nil
		}
	}
```

- [ ] **Step 7: Wire config → IndexConfig**

In `internal/config/config.go`, add a config block (after the `VocabConfig` type) and a field on `Config`:

```go
// DomainConfig configures channel ② (domain-knowledge embedding). Empty
// ProjectDir disables it: cks.ops.index then refreshes only code + ckg.
type DomainConfig struct {
	// ProjectDir is the cks domain-knowledge project directory exported
	// before a full ckv build (e.g. docs/domain-knowledge/projects/go-stablenet).
	ProjectDir string `yaml:"project_dir"`
	// CorpusDir is the export output and the ckv --docs root
	// (e.g. generated/domain-corpus/go-stablenet).
	CorpusDir string `yaml:"corpus_dir"`
}
```

Add to the `Config` struct (after `Vocab VocabConfig`):

```go
	Vocab  VocabConfig  `yaml:"vocab"`
	Domain DomainConfig `yaml:"domain"`
```

In `cmd/cks-mcp/main.go`, where the `IndexConfig{...}` is constructed for `Deps.Index` (search for `IndexConfig{`), add:

```go
		DomainProjectDir: cfg.Domain.ProjectDir,
		DomainCorpusDir:  cfg.Domain.CorpusDir,
```

- [ ] **Step 8: Document the config in the example**

In `policies/cks.yaml.example`, append:

```yaml
# Channel ② (domain-knowledge embedding). Empty project_dir disables it.
domain:
  # cks domain-knowledge project to export before a full ckv build.
  project_dir: "docs/domain-knowledge/projects/go-stablenet"
  # Export output + ckv --docs root (gitignored; rebuilt each full index).
  corpus_dir: "generated/domain-corpus/go-stablenet"
```

- [ ] **Step 9: Run the full cks build and tests**

Run: `go build ./... && go test ./internal/mcp/ ./internal/inventory/ ./internal/domainexport/ ./internal/config/`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add .gitignore internal/mcp/ops_index.go internal/mcp/ops_index_test.go internal/config/config.go cmd/cks-mcp/main.go policies/cks.yaml.example
git commit -m "Orchestrate channel-2 corpus export in cks.ops.index"
```

### Task B6: end-to-end verification against go-stablenet

**Files:** none (verification only).

- [ ] **Step 1: Build both binaries**

```bash
cd /Users/wm-it-22-00661/Work/github/tools/code-knowledge-vector && go build -o /tmp/ckv ./cmd/ckv
cd /Users/wm-it-22-00661/Work/github/tools/code-knowledge-system && go build -o /tmp/cks-domain-export ./cmd/cks-domain-export
```

- [ ] **Step 2: Export the corpus and build the index with --docs**

```bash
export GO_STABLENET_ROOT=/Users/wm-it-22-00661/Work/github/tools/go-stablenet
cd /Users/wm-it-22-00661/Work/github/tools/code-knowledge-system
/tmp/cks-domain-export -project docs/domain-knowledge/projects/go-stablenet -out generated/domain-corpus/go-stablenet
/tmp/ckv build --src "$GO_STABLENET_ROOT" --docs generated/domain-corpus/go-stablenet --out /tmp/ckv-stablenet --embedder=ollama --model-name=bge-m3
```

Expected: build reports indexed files including the corpus markdown; `/tmp/ckv-stablenet/manifest.json` contains `docs_roots`.

> Requires a running Ollama daemon with bge-m3. If Ollama is unavailable, skip this step and rely on the unit tests (the mock-embedder build test in A2 already proves the `--docs` path end-to-end).

- [ ] **Step 3: Confirm a domain query returns entry prose**

```bash
/tmp/ckv build --json --src "$GO_STABLENET_ROOT" --docs generated/domain-corpus/go-stablenet --out /tmp/ckv-stablenet --embedder=ollama --model-name=bge-m3 >/dev/null
grep -c docs_roots /tmp/ckv-stablenet/manifest.json
```

Expected: `1` (docs_roots recorded). Manual: a `semantic_search` for "validator quorum calculation" should surface `entries/A1.wbft_core.quorum_calc.md`.

- [ ] **Step 4: Push and open the cks PR**

```bash
git push -u origin p0b-channel2-domain-embedding
gh pr create --title "Channel 2: embed domain knowledge and authoritative docs in ckv" \
  --body "Implements channel ② per docs/superpowers/specs/2026-06-05-channel-2-domain-embedding-design.md. cks renders verified + needs_verification entries to a markdown corpus and copies the project's authoritative docs into it; cks.ops.index runs the export then \`ckv build --docs\` so entries + docs land in the same index. Depends on the ckv --docs flag (merged separately)."
```

---

## Self-Review

**Spec coverage:**
- D1 (scope = channel ②): Phases A+B build only the ckv embedding; no coding-agent work. ✓
- D2 (gating verified + needs_verification, status shown): Task B3 `embeddableStatuses`; Task B2 Status line. ✓
- D3 (single index, corpus + --docs, ckv schema-agnostic): Tasks A1–A3 (markdown only, Category tag) + B5 orchestration. ✓
- §4.1 ckv --docs / Category="domain" / manifest docs_root: A1, A2, A3. ✓
- §4.2 domainexport + RenderEntry + authoritative_docs + loader field: B1, B2, B3, B4. ✓
- §4.3 cks.ops.index orchestration + config: B5. ✓
- §6 error handling (missing doc, code_root unset, no entries): B3 tests cover missing doc + (unset code_root path in Export). ✓
- §7 tests (renderer golden, gating, determinism, --docs discovery, manifest): A2, B2, B3. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every test asserts concrete values. The one lookup ("search for `IndexConfig{`" in cms-mcp/main.go, Task B5 Step 7) is a concrete 2-line insertion at a known construction site, not a placeholder.

**Type consistency:** `Options.DocsRoots`, `Manifest.DocsRoots`, `IndexConfig.DomainProjectDir/DomainCorpusDir`, `domainexport.Export/RenderEntry/Result`, `inventory.AuthoritativeDoc`, `Project.AuthoritativeDocs` are named identically across all tasks. `Category="domain"` is the single tag value used in A2 (set) and A2 test (assert).
