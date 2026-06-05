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
