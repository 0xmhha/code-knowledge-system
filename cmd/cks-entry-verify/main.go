// Command cks-entry-verify promotes a single domain-knowledge entry to
// status: verified and refreshes the inventory.md count tables in one
// atomic-feeling operation.
//
// The promotion writes exactly three fields back into the entry YAML:
//
//	status:           verified
//	last_verified_at: <-date, default today>
//	verified_by:      <-by>
//
// All other fields, ordering, comments, and multi-line literal styles
// are preserved. The rewrite goes through yaml.Node mutation, not a
// struct re-marshal — see internal/inventory/verify.go for details.
//
// Before writing, the entry is simulated through ValidateEntry to make
// sure the verified transition passes every mechanical check (anchors
// exist, cross-references resolve, etc.). If validation fails, no file
// is touched and the issues print to stderr; the operator fixes the
// underlying problem in the entry YAML, then re-runs.
//
// Usage:
//
//	cks-entry-verify \
//	    -project docs/domain-knowledge/projects/go-stablenet \
//	    -entry   A1.wbft_core.quorum_calc \
//	    -by      mhha
//
// Optional flags:
//
//	-date 2026-05-29    explicit verification date (defaults to today UTC)
//	-skip-inventory     do not rewrite <project>/inventory.md afterwards
//
// Exit codes:
//   - 0: entry promoted, inventory.md updated.
//   - 1: validation failed; entry unchanged.
//   - 2: usage error (missing flag, entry not found, IO error).
package main

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func main() {
	var (
		projectDir     = flag.String("project", "", "project directory")
		entryRef       = flag.String("entry", "", "entry id (e.g. A1.wbft_core.quorum_calc) or path to entry YAML")
		reviewer       = flag.String("by", "", "reviewer handle to record under verified_by")
		date           = flag.String("date", "", "verification date YYYY-MM-DD (default: today UTC)")
		skipInventory  = flag.Bool("skip-inventory", false, "do not rewrite inventory.md after the promotion")
	)
	flag.Parse()

	if *projectDir == "" || *entryRef == "" || *reviewer == "" {
		fmt.Fprintln(os.Stderr, "cks-entry-verify: -project, -entry, and -by are required")
		flag.Usage()
		os.Exit(2)
	}
	if *date == "" {
		*date = time.Now().UTC().Format("2006-01-02")
	}

	p, err := inventory.LoadProject(*projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cks-entry-verify: %v\n", err)
		os.Exit(2)
	}

	entryID, sourcePath, ok := resolveEntry(p, *entryRef)
	if !ok {
		fmt.Fprintf(os.Stderr, "cks-entry-verify: entry %q not found in project %s\n", *entryRef, p.ID)
		os.Exit(2)
	}

	original := p.Entries[entryID]
	mutated := original
	mutated.Status = "verified"
	mutated.LastVerifiedAt = *date
	mutated.VerifiedBy = *reviewer

	// Simulate the post-promotion project so ValidateEntry sees the
	// new fields. Subsystems and other entries are shared with p, so
	// cross-reference resolution works the same.
	simulated := *p
	simulated.Entries = make(map[string]inventory.Entry, len(p.Entries))
	maps.Copy(simulated.Entries, p.Entries)
	simulated.Entries[entryID] = mutated

	issues := inventory.ValidateEntry(&simulated, mutated)
	errCount := 0
	for _, iss := range issues {
		switch iss.Severity {
		case inventory.SeverityError:
			errCount++
			fmt.Fprintf(os.Stderr, "%s: error: %s: %s\n", iss.File, iss.EntryID, iss.Message)
		case inventory.SeverityWarning:
			fmt.Fprintf(os.Stderr, "%s: warning: %s: %s\n", iss.File, iss.EntryID, iss.Message)
		}
	}
	if errCount > 0 {
		fmt.Fprintf(os.Stderr, "cks-entry-verify: %d errors blocking verified transition; entry unchanged\n", errCount)
		os.Exit(1)
	}

	if err := inventory.MarkVerified(sourcePath, *date, *reviewer); err != nil {
		fmt.Fprintf(os.Stderr, "cks-entry-verify: %v\n", err)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "cks-entry-verify: %s -> verified (by=%s date=%s)\n", entryID, *reviewer, *date)

	if *skipInventory {
		return
	}
	// Reload so the inventory.md update sees the new status. This is
	// cheap (small file count) and avoids any risk of stale in-memory
	// state lagging behind disk after MarkVerified.
	p2, err := inventory.LoadProject(*projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cks-entry-verify: reload project: %v\n", err)
		os.Exit(2)
	}
	invPath := filepath.Join(p2.Dir, "inventory.md")
	if err := inventory.UpdateInventoryCounts(invPath, p2); err != nil {
		fmt.Fprintf(os.Stderr, "cks-entry-verify: update inventory.md: %v\n", err)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "cks-entry-verify: refreshed %s\n", invPath)
}

// resolveEntry accepts either an entry ID (as it appears in entries/*.yaml)
// or a path to an entry YAML file, and returns the canonical entry ID
// plus the absolute source path. Paths are resolved to absolute and
// matched against each entry's SourcePath so that callers can pass
// either form interchangeably.
func resolveEntry(p *inventory.Project, ref string) (id, path string, ok bool) {
	if e, found := p.Entries[ref]; found {
		return e.ID, e.SourcePath, true
	}
	abs, err := filepath.Abs(ref)
	if err != nil {
		return "", "", false
	}
	for _, e := range p.Entries {
		if e.SourcePath == abs {
			return e.ID, e.SourcePath, true
		}
	}
	return "", "", false
}
