// Package domainexport renders a domain-knowledge Project into a markdown
// corpus that ckv embeds via `ckv build --docs`. It is the producer side
// of channel ② (see docs/superpowers/specs/2026-06-05-channel-2-domain-
// embedding-design.md): one markdown file per embeddable entry plus copies
// of the project's authoritative docs.
package domainexport

import (
	"fmt"
	"os"
	"path/filepath"
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
//   - entries/<id>.md  for each entry whose status is embeddable
//   - docs/<basename>  copies of p.AuthoritativeDocs resolved under CodeRoot
//     (the original filename is preserved; basenames must be unique across
//     authoritative_docs or a later doc silently overwrites an earlier one)
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
