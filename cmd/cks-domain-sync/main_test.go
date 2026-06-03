package main

import (
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func TestDeriveViews_EmptyInput(t *testing.T) {
	ckv, ckg := deriveViews(nil)
	if ckv.Version != 1 {
		t.Errorf("ckv version = %d, want 1", ckv.Version)
	}
	if len(ckv.Categories) != 0 || len(ckg.Policies) != 0 {
		t.Errorf("empty input should yield empty views, got %d cats / %d policies", len(ckv.Categories), len(ckg.Policies))
	}
}

func TestDeriveViews_GroupsAndMaps(t *testing.T) {
	entries := []inventory.Entry{
		{
			ID: "E2", Subsystem: "consensus", Title: "Quorum size", Summary: "2f+1 quorum",
			Status: "verified",
			CodeAnchors: []inventory.CodeAnchor{
				{File: "consensus/wbft/quorum.go", Symbol: "wbft.QuorumSize"},
				{File: "consensus/wbft/quorum_test.go"},
			},
			Pitfalls:   []string{"off-by-one in 2f+1"},
			Invariants: []string{"power == 1 per validator"},
		},
		{
			ID: "E1", Subsystem: "consensus", Title: "Finalize", Summary: "block finalize",
			Status: "verified",
			CodeAnchors: []inventory.CodeAnchor{
				{File: "consensus/wbft/loop.go", Symbol: "wbft.loop"},
			},
		},
	}
	ckv, ckg := deriveViews(entries)

	// ckv: both entries share subsystem "consensus" → one category.
	if len(ckv.Categories) != 1 || ckv.Categories[0].Name != "consensus" {
		t.Fatalf("ckv categories = %+v, want one named consensus", ckv.Categories)
	}
	cat := ckv.Categories[0]
	// Paths deduped to the dir glob (loop.go + quorum.go + quorum_test.go all in consensus/wbft).
	if len(cat.Paths) != 1 || cat.Paths[0] != "consensus/wbft/**" {
		t.Errorf("paths = %v, want [consensus/wbft/**]", cat.Paths)
	}
	if !contains(cat.WatchOut, "off-by-one in 2f+1") || !contains(cat.WatchOut, "power == 1 per validator") {
		t.Errorf("watch_out = %v, want pitfalls+invariants", cat.WatchOut)
	}
	if len(cat.RequiredTests) != 1 || cat.RequiredTests[0] != "consensus/wbft/quorum_test.go" {
		t.Errorf("required_tests = %v, want the _test.go anchor", cat.RequiredTests)
	}

	// ckg: one policy per entry; governs = anchor symbols.
	if len(ckg.Policies) != 2 {
		t.Fatalf("ckg policies = %d, want 2", len(ckg.Policies))
	}
	e2 := findPolicy(ckg.Policies, "E2")
	if e2 == nil || e2.Name != "Quorum size" || e2.Category != "consensus" {
		t.Errorf("E2 policy = %+v", e2)
	}
	if len(e2.Governs) != 1 || e2.Governs[0] != "wbft.QuorumSize" {
		t.Errorf("E2 governs = %v, want [wbft.QuorumSize]", e2.Governs)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func findPolicy(ps []ckgPolicy, id string) *ckgPolicy {
	for i := range ps {
		if ps[i].ID == id {
			return &ps[i]
		}
	}
	return nil
}
