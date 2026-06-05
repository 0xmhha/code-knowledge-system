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
