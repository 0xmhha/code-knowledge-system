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
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func main() {
	var (
		projectDir      = flag.String("project", "", "project directory (contains project.yaml, subsystems.yaml, entries/)")
		updateInventory = flag.Bool("update-inventory", false, "after validation, rewrite <project>/inventory.md's count tables")
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
