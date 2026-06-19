// Command cks-inventory-check runs the mechanical (script-checkable)
// half of the verification checklist from
// docs/domain-knowledge/shared/STATUS_LIFECYCLE.md against one
// project's domain-knowledge inventory.
//
// What it catches:
//   - schema-level: required fields, enum membership, ID/date patterns,
//     status-driven conditional fields (e.g. verified requires anchors).
//   - filename rule: every entry file is named <id>.yaml.
//   - cross-references: subsystem exists in subsystems.yaml,
//     related_concepts IDs resolve to other entries.
//   - filesystem refs: code_anchors[].file and existing_doc_ref[].file
//     exist under project.code_root.
//
// What it does not catch — those belong to the substantive (human)
// half of the checklist:
//   - whether summary text is factually correct against current code
//   - whether code_keywords actually appear in source
//   - whether invariants and pitfalls describe real behavior
//
// Usage:
//
//	cks-inventory-check -project docs/domain-knowledge/projects/go-stablenet
//
// Exit codes:
//   - 0: no errors. Warnings (if any) printed to stderr.
//   - 1: at least one error issue. Errors and warnings printed to stderr.
//   - 2: usage error (missing flag, project directory unreadable).
//
// With -update-inventory the command rewrites inventory.md's count
// tables from the loaded entries; useful after a batch of edits when
// you want the dashboard to reflect reality without re-running
// cks-entry-verify per file. The rewrite leaves freeform prose
// (Conventions, Pending work, etc.) untouched.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func main() {
	var (
		projectDir      = flag.String("project", "", "project directory (contains project.yaml, subsystems.yaml, entries/)")
		updateInventory = flag.Bool("update-inventory", false, "after validation, rewrite <project>/inventory.md's count tables")
		graphPath       = flag.String("graph", "", "optional ckg graph.db path; when set, assert every def code-anchor symbol resolves uniquely in ckg (Phase 3 A1-3)")
	)
	flag.Parse()

	if *projectDir == "" {
		fmt.Fprintln(os.Stderr, "cks-inventory-check: -project is required")
		flag.Usage()
		os.Exit(2)
	}

	p, err := inventory.LoadProject(*projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cks-inventory-check: %v\n", err)
		os.Exit(2)
	}

	issues := inventory.ValidateProject(p)
	errCount, warnCount := reportIssues(os.Stderr, issues)

	// Phase 3 A1-3: when a ckg graph is provided, assert every def anchor's
	// symbol resolves to exactly one definition. def anchors promise a uniquely
	// resolvable symbol; a 0- or multi-match means the symbol is wrong or too
	// short and would resolve ambiguously at query time.
	if *graphPath != "" {
		errCount += checkDefAnchorResolution(os.Stderr, p, *graphPath)
	}

	fmt.Fprintf(os.Stderr, "cks-inventory-check: %d entries, %d errors, %d warnings\n",
		len(p.Entries), errCount, warnCount)

	if *updateInventory && errCount == 0 {
		invPath := filepath.Join(p.Dir, "inventory.md")
		if err := inventory.UpdateInventoryCounts(invPath, p); err != nil {
			fmt.Fprintf(os.Stderr, "cks-inventory-check: update inventory.md: %v\n", err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "cks-inventory-check: updated %s\n", invPath)
	} else if *updateInventory && errCount > 0 {
		fmt.Fprintln(os.Stderr, "cks-inventory-check: skipping inventory.md update because errors are present")
	}

	if errCount > 0 {
		os.Exit(1)
	}
}

// checkDefAnchorResolution opens the ckg graph and verifies that every def
// code-anchor's symbol resolves to exactly one definition. Returns the number
// of errors found (0 when clean or when a symbol is absent — absence is a
// warning, not a hard error, since the graph may lag the entries). loc anchors
// are skipped: they intentionally point inside a symbol, not at a definition.
func checkDefAnchorResolution(w *os.File, p *inventory.Project, graphPath string) int {
	cli, err := ckgclient.NewReal(graphPath)
	if err != nil {
		fmt.Fprintf(w, "(project): error: : open ckg graph %q: %v\n", graphPath, err)
		return 1
	}
	defer cli.Close()
	ctx := context.Background()

	errors := 0
	for _, id := range p.EntryIDsSorted() {
		e := p.Entries[id]
		for _, a := range e.CodeAnchors {
			if a.ResolvedKind() != inventory.AnchorKindDef || a.Symbol == "" {
				continue
			}
			cits, err := cli.FindSymbol(ctx, a.Symbol, ckgclient.SymbolOpts{})
			if err != nil {
				fmt.Fprintf(w, "%s: error: %s: FindSymbol %q: %v\n", a.File, id, a.Symbol, err)
				errors++
				continue
			}
			// File-aware: an anchor pins file + symbol + line, so uniqueness is
			// judged WITHIN the anchor's file. A short symbol like "API.Status"
			// is globally ambiguous (clique vs wbft backend) but unique in its
			// file; scoping to a.File avoids that false positive. Distinct
			// definitions are counted by start line within the file.
			seen := map[string]struct{}{}
			for _, c := range cits {
				if c.File != a.File {
					continue
				}
				seen[fmt.Sprintf("%d", c.StartLine)] = struct{}{}
			}
			switch len(seen) {
			case 0:
				fmt.Fprintf(w, "%s: warning: %s: def anchor symbol %q does not resolve in its file (renamed/moved, graph lag, or a symbol form ckg does not store)\n", a.File, id, a.Symbol)
			case 1:
				// unique within the file — the def contract holds.
			default:
				fmt.Fprintf(w, "%s: error: %s: def anchor symbol %q resolves to %d definitions in the same file; qualify it\n", a.File, id, a.Symbol, len(seen))
				errors++
			}
		}
	}
	return errors
}

// reportIssues prints each issue in compiler-error format
// (<file>: <severity>: <entry-id>: <message>) and returns the counts.
// Grouping by entry would read nicer for humans, but the flat form is
// easier to grep and matches the style most editors highlight as a
// jump-to-line link.
func reportIssues(w *os.File, issues []inventory.Issue) (errors, warnings int) {
	for _, iss := range issues {
		file := iss.File
		if file == "" {
			file = "(project)"
		}
		switch iss.Severity {
		case inventory.SeverityError:
			errors++
			fmt.Fprintf(w, "%s: error: %s: %s\n", file, iss.EntryID, iss.Message)
		case inventory.SeverityWarning:
			warnings++
			fmt.Fprintf(w, "%s: warning: %s: %s\n", file, iss.EntryID, iss.Message)
		}
	}
	return errors, warnings
}
