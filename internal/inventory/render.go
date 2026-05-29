package inventory

import (
	"fmt"
	"os"
	"strings"
)

// Stable description text for the Knowledge-type coverage table. Kept
// in sync with shared/KNOWLEDGE_TYPES.md so the rendered cell text
// matches the canonical doc. If the type catalog gains a row, add it
// here and bump the iteration in renderKnowledgeTypeCoverage.
var knowledgeTypeDescriptions = []struct {
	ID, Desc string
}{
	{"B1", "Architecture"},
	{"B2", "Data Structure"},
	{"B3", "Algorithm / Flow"},
	{"B4", "Invariant / Constraint"},
	{"B5", "Pitfall / Anti-pattern"},
	{"B6", "Procedure / Checklist"},
	{"B7", "Reference (constants)"},
}

// UpdateInventoryCounts reads inventory.md at path, regenerates the
// four count tables, and writes the result back. All freeform prose
// (the "Conventions" section, "Pending work", etc.) is preserved
// verbatim — only the four known table sections are touched.
//
// If a known section heading is not present in the input file, its
// table is not added; UpdateInventoryCounts only updates sections that
// already exist. This keeps the function safe to run against
// hand-authored inventory.md files whose layout differs from the
// canonical four-table form.
func UpdateInventoryCounts(path string, p *Project) error {
	buf, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("inventory: read %q: %w", path, err)
	}
	updated := RenderInventoryMD(string(buf), p)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("inventory: write %q: %w", path, err)
	}
	return nil
}

// RenderInventoryMD takes the current inventory.md text and returns a
// new version with the four canonical count tables regenerated from p.
// Sections the renderer does not know about are passed through verbatim,
// including their bodies.
//
// The renderer operates on the markdown as a stream of H2 sections; an
// H2 heading begins a new section, and a section's body is every line
// after the heading until the next H2 (or EOF). Replacing a section's
// body means emitting one blank line, the new table, one blank line —
// matching the existing formatting in
// docs/domain-knowledge/projects/go-stablenet/inventory.md.
func RenderInventoryMD(input string, p *Project) string {
	knownSections := map[string][]string{
		"## Status summary":          renderStatusSummary(p),
		"## Subsystem coverage":      renderSubsystemCoverage(p),
		"## Knowledge-type coverage": renderKnowledgeTypeCoverage(p),
		"## Current entries":         renderCurrentEntries(p),
	}

	lines := strings.Split(input, "\n")

	type section struct {
		// header is the H2 heading line (empty for the prologue,
		// which is everything before the first H2).
		header string
		// body is every line after header, before the next H2.
		body []string
	}
	var sections []section
	cur := section{}
	for _, ln := range lines {
		trimmed := strings.TrimRight(ln, " \t")
		if strings.HasPrefix(trimmed, "## ") {
			sections = append(sections, cur)
			cur = section{header: trimmed}
			continue
		}
		cur.body = append(cur.body, ln)
	}
	sections = append(sections, cur)

	var out []string
	for i, s := range sections {
		if i > 0 {
			out = append(out, s.header)
		}
		if newBody, ok := knownSections[s.header]; ok {
			out = append(out, "")
			out = append(out, newBody...)
			out = append(out, "")
		} else {
			out = append(out, s.body...)
		}
	}
	return strings.Join(out, "\n")
}

func renderStatusSummary(p *Project) []string {
	counts := p.CountByStatus()
	return []string{
		"| Status | Count |",
		"|---|---|",
		fmt.Sprintf("| verified | %d |", counts["verified"]),
		fmt.Sprintf("| needs_verification | %d |", counts["needs_verification"]),
		fmt.Sprintf("| draft | %d |", counts["draft"]),
		fmt.Sprintf("| needs_author | %d |", counts["needs_author"]),
	}
}

func renderSubsystemCoverage(p *Project) []string {
	bySubsystem := p.CountBySubsystem()
	rows := []string{
		"| Subsystem | Name | Entries (verified / total) |",
		"|---|---|---|",
	}
	for _, id := range p.SubsystemOrder {
		s := p.Subsystems[id]
		c := bySubsystem[id]
		rows = append(rows, fmt.Sprintf("| %s | %s | %d / %d |", id, s.Name, c.Verified, c.Total))
	}
	return rows
}

func renderKnowledgeTypeCoverage(p *Project) []string {
	counts := p.CountByKnowledgeType()
	rows := []string{
		"| Type | Description | Count |",
		"|---|---|---|",
	}
	for _, kt := range knowledgeTypeDescriptions {
		rows = append(rows, fmt.Sprintf("| %s | %s | %d |", kt.ID, kt.Desc, counts[kt.ID]))
	}
	return rows
}

func renderCurrentEntries(p *Project) []string {
	rows := []string{
		"| ID | Subsystem | Type | Title | Status | Priority |",
		"|---|---|---|---|---|---|",
	}
	for _, id := range p.EntryIDsSorted() {
		e := p.Entries[id]
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s | %s | %s |",
			e.ID, e.Subsystem, e.KnowledgeType, e.Title, e.Status, e.Priority))
	}
	return rows
}
