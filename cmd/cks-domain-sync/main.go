// Command cks-domain-sync derives the two consumer policy views from the cks
// domain-knowledge master entries (00 §4.1): one edit in cks → both ckv and ckg
// refresh on their next reindex.
//
//   - ckv view  (policy/stablenet.yaml shape): category → {paths, watch_out,
//     required_tests} guidance, grouped by subsystem.
//   - ckg view  (policy.yaml shape): one policy per entry with governs[] code
//     anchors → governed_by edges.
//
// Only entries with status: verified are emitted (00 §4.2). Today zero entries
// are verified, so output is empty until the curation session promotes the
// byzantine-fairness entries — the codegen is correct, its output is gated on
// that human activity.
//
// Usage:
//
//	cks-domain-sync -entries <project-dir> [-ckv-out path] [-ckg-out path]
//
// With no -*-out flags both views are written to stdout (dry run).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func main() {
	entriesDir := flag.String("entries", "docs/domain-knowledge/projects/go-stablenet", "project dir holding entries/*.yaml")
	ckvOut := flag.String("ckv-out", "", "write the ckv stablenet.yaml view here (default: stdout)")
	ckgOut := flag.String("ckg-out", "", "write the ckg policy.yaml view here (default: stdout)")
	flag.Parse()

	if err := run(*entriesDir, *ckvOut, *ckgOut); err != nil {
		fmt.Fprintln(os.Stderr, "cks-domain-sync:", err)
		os.Exit(1)
	}
}

func run(entriesDir, ckvOut, ckgOut string) error {
	proj, err := inventory.LoadProject(entriesDir)
	if err != nil {
		return fmt.Errorf("load project %q: %w", entriesDir, err)
	}
	entries := verifiedEntries(proj)
	ckv, ckg := deriveViews(entries)

	if err := emit("ckv (stablenet.yaml)", ckv, ckvOut); err != nil {
		return err
	}
	if err := emit("ckg (policy.yaml)", ckg, ckgOut); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "cks-domain-sync: %d verified entries → %d ckv categories, %d ckg policies\n",
		len(entries), len(ckv.Categories), len(ckg.Policies))
	return nil
}

// --- output shapes (match the consumer files) ---

type ckvCategory struct {
	Name          string   `yaml:"name"`
	Paths         []string `yaml:"paths,omitempty"`
	WatchOut      []string `yaml:"watch_out,omitempty"`
	RequiredTests []string `yaml:"required_tests,omitempty"`
}

type ckvFile struct {
	Version    int           `yaml:"version"`
	Categories []ckvCategory `yaml:"categories"`
}

type ckgPolicy struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Category    string   `yaml:"category,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Governs     []string `yaml:"governs,omitempty"`
}

type ckgFile struct {
	Policies []ckgPolicy `yaml:"policies"`
}

// verifiedEntries returns the project's verified entries sorted by ID for
// deterministic output.
func verifiedEntries(p *inventory.Project) []inventory.Entry {
	var out []inventory.Entry
	for _, e := range p.Entries {
		if e.Status == "verified" {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// deriveViews is the pure derivation: verified entries → (ckv, ckg) views.
// Deterministic for a fixed input (every list sorted + deduped). Exposed for
// unit testing without a project dir on disk.
func deriveViews(entries []inventory.Entry) (ckvFile, ckgFile) {
	// ckv: group by subsystem → one category each.
	bySub := map[string]*ckvCategory{}
	var subOrder []string
	for _, e := range entries {
		cat := bySub[e.Subsystem]
		if cat == nil {
			cat = &ckvCategory{Name: e.Subsystem}
			bySub[e.Subsystem] = cat
			subOrder = append(subOrder, e.Subsystem)
		}
		for _, a := range e.CodeAnchors {
			if a.File == "" {
				continue
			}
			cat.Paths = append(cat.Paths, dirGlob(a.File))
			if strings.HasSuffix(a.File, "_test.go") {
				cat.RequiredTests = append(cat.RequiredTests, a.File)
			}
		}
		cat.WatchOut = append(cat.WatchOut, e.Pitfalls...)
		cat.WatchOut = append(cat.WatchOut, e.Invariants...)
	}
	sort.Strings(subOrder)
	ckv := ckvFile{Version: 1}
	for _, sub := range subOrder {
		c := bySub[sub]
		c.Paths = sortedUnique(c.Paths)
		c.WatchOut = sortedUnique(c.WatchOut)
		c.RequiredTests = sortedUnique(c.RequiredTests)
		ckv.Categories = append(ckv.Categories, *c)
	}

	// ckg: one policy per entry; governs = anchor symbols (qnames).
	var ckg ckgFile
	for _, e := range entries {
		var governs []string
		for _, a := range e.CodeAnchors {
			if a.Symbol != "" {
				governs = append(governs, a.Symbol)
			}
		}
		ckg.Policies = append(ckg.Policies, ckgPolicy{
			ID:          e.ID,
			Name:        e.Title,
			Category:    e.Subsystem,
			Description: e.Summary,
			Governs:     sortedUnique(governs),
		})
	}
	return ckv, ckg
}

// dirGlob turns a repo-relative file into the policy path glob for its
// directory (e.g. "consensus/wbft/loop.go" → "consensus/wbft/**").
func dirGlob(file string) string {
	d := filepath.ToSlash(filepath.Dir(file))
	if d == "" || d == "." {
		return "**"
	}
	return d + "/**"
}

func sortedUnique(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func emit(label string, v any, path string) error {
	raw, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", label, err)
	}
	if path == "" {
		fmt.Printf("# --- %s ---\n%s\n", label, raw)
		return nil
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s to %s: %w", label, path, err)
	}
	return nil
}
