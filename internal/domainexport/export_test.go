package domainexport

import (
	"os"
	"path/filepath"
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

func TestExport_NoCodeRoot(t *testing.T) {
	p := &inventory.Project{
		Subsystems:        map[string]inventory.Subsystem{"A1": {ID: "A1", Name: "Core"}},
		Entries:           map[string]inventory.Entry{"A1.v": {ID: "A1.v", Subsystem: "A1", KnowledgeType: "B1", Title: "V", Status: "verified", Summary: "s"}},
		AuthoritativeDocs: []inventory.AuthoritativeDoc{{File: "CLAUDE.md", Role: "overview"}},
		// CodeRoot intentionally empty.
	}
	out := t.TempDir()
	res, err := Export(p, out)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	// The entry corpus still ships; the doc copy is skipped with exactly one warning.
	if res.EntriesWritten != 1 {
		t.Errorf("EntriesWritten = %d, want 1", res.EntriesWritten)
	}
	if res.DocsCopied != 0 {
		t.Errorf("DocsCopied = %d, want 0", res.DocsCopied)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "code_root unset") {
		t.Errorf("expected one code_root-unset warning, got %v", res.Warnings)
	}
}

func TestExport_PrunesStaleEntries(t *testing.T) {
	out := t.TempDir()
	// A leftover file from a prior export (e.g. an entry since demoted to draft).
	staleDir := filepath.Join(out, "entries")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(staleDir, "A9.removed.md")
	if err := os.WriteFile(stale, []byte("# stale\n**Status:** verified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &inventory.Project{
		Subsystems: map[string]inventory.Subsystem{"A1": {ID: "A1", Name: "Core"}},
		Entries: map[string]inventory.Entry{
			"A1.v": {ID: "A1.v", Subsystem: "A1", KnowledgeType: "B1", Title: "V", Status: "verified", Summary: "s"},
		},
	}
	if _, err := Export(p, out); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale entry A9.removed.md should have been swept, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(staleDir, "A1.v.md")); err != nil {
		t.Errorf("current entry A1.v.md missing after sweep: %v", err)
	}
}
