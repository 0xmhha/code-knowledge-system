package budget

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
)

// knowledgeSeed is a seed whose hit came from the kind-scoped knowledge
// pass (invariant/convention chunk).
func knowledgeSeed(file string, score float64, kind string) stage2.ScoredCitation {
	sc := seed(file, score)
	sc.ChunkKind = kind
	return sc
}

func TestAllocate_KnowledgeReserveRescuesLowRankedInvariant(t *testing.T) {
	t.Parallel()
	// 4 code seeds outscore 1 invariant seed; MaxCitations=3 with
	// KnowledgeReserve=1. Without the reserve the total cap fires at the
	// three code seeds and the invariant never gets processed — the exact
	// starvation shape the neighbor reserve fixed one level down.
	seeds := []stage2.ScoredCitation{
		seed("a.go", 9.0),
		seed("b.go", 8.0),
		seed("c.go", 7.0),
		seed("d.go", 6.0),
		knowledgeSeed("rules.md", 0.5, "invariant"),
	}
	fetcher := &FakeFetcher{Bodies: map[string]string{
		cit("a.go", 1, 10).Key():     bodyN(10),
		cit("b.go", 1, 10).Key():     bodyN(10),
		cit("c.go", 1, 10).Key():     bodyN(10),
		cit("d.go", 1, 10).Key():     bodyN(10),
		cit("rules.md", 1, 10).Key(): bodyN(10),
	}}
	a, _ := New(fetcher, WithConfig(Config{
		MaxTokens: 100000, OverheadReserve: 0.10,
		MaxCitations: 3, KnowledgeReserve: 1,
	}))
	out, err := a.Allocate(context.Background(), seeds, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Selected) != 3 {
		t.Fatalf("Selected = %d, want 3", len(out.Selected))
	}
	got := map[string]string{}
	for _, s := range out.Selected {
		got[s.Citation.File] = s.ChunkKind
	}
	if _, ok := got["rules.md"]; !ok {
		t.Fatalf("invariant chunk not selected; got %v", got)
	}
	if got["rules.md"] != "invariant" {
		t.Errorf("SelectedItem.ChunkKind = %q, want invariant (kind must survive into Stage4Output)", got["rules.md"])
	}
	// The held slot displaces exactly one code seed: a.go + b.go stay, c.go loses.
	if _, ok := got["c.go"]; ok {
		t.Errorf("c.go selected despite knowledge holdback; got %v", got)
	}
}

func TestAllocate_KnowledgeReserveExemptsSeedCap(t *testing.T) {
	t.Parallel()
	// seedCap = MaxCitations - NeighborReserve = 1. The invariant seed is
	// past the seed quota but must still enter via the reserve exemption —
	// domain rules must not lose their slot to yet another code body.
	seeds := []stage2.ScoredCitation{
		seed("a.go", 9.0),
		seed("b.go", 8.0),
		knowledgeSeed("rules.md", 0.5, "convention"),
	}
	fetcher := &FakeFetcher{Bodies: map[string]string{
		cit("a.go", 1, 10).Key():     bodyN(10),
		cit("b.go", 1, 10).Key():     bodyN(10),
		cit("rules.md", 1, 10).Key(): bodyN(10),
	}}
	a, _ := New(fetcher, WithConfig(Config{
		MaxTokens: 100000, OverheadReserve: 0.10,
		MaxCitations: 2, NeighborReserve: 1, KnowledgeReserve: 1,
	}))
	out, err := a.Allocate(context.Background(), seeds, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]bool{}
	for _, s := range out.Selected {
		files[s.Citation.File] = true
	}
	if !files["rules.md"] {
		t.Fatalf("convention chunk lost to seed cap; selected %v", files)
	}
}

func TestAllocate_NoKnowledgeCandidatesLeavesReserveUnused(t *testing.T) {
	t.Parallel()
	// Holdback holds a slot only while knowledge candidates could still
	// appear; with none in the list the reserve simply goes unfilled and
	// selection stops one short of the cap (same semantics as the
	// neighbor reserve with no neighbors).
	seeds := []stage2.ScoredCitation{
		seed("a.go", 9.0),
		seed("b.go", 8.0),
		seed("c.go", 7.0),
	}
	fetcher := &FakeFetcher{Bodies: map[string]string{
		cit("a.go", 1, 10).Key(): bodyN(10),
		cit("b.go", 1, 10).Key(): bodyN(10),
		cit("c.go", 1, 10).Key(): bodyN(10),
	}}
	a, _ := New(fetcher, WithConfig(Config{
		MaxTokens: 100000, OverheadReserve: 0.10,
		MaxCitations: 3, KnowledgeReserve: 1,
	}))
	out, err := a.Allocate(context.Background(), seeds, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Selected) != 2 {
		t.Fatalf("Selected = %d, want 2 (one slot held for absent knowledge)", len(out.Selected))
	}
}

func TestDefaultConfig_KnowledgeReserve(t *testing.T) {
	t.Parallel()
	if DefaultConfig().KnowledgeReserve != DefaultKnowledgeReserve || DefaultKnowledgeReserve != 2 {
		t.Errorf("DefaultConfig.KnowledgeReserve = %d, want %d", DefaultConfig().KnowledgeReserve, DefaultKnowledgeReserve)
	}
}
