package budget

import (
	"context"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

type mapFetcher map[string]string

func (m mapFetcher) Fetch(_ context.Context, c contract.Citation) (string, error) {
	return m[c.Key()], nil
}

func drCit(file string) contract.Citation {
	return contract.Citation{File: file, StartLine: 1, EndLine: 10, CommitHash: "c"}
}

// A body too large for the remaining budget must degrade to a head snippet
// (marked Degraded) instead of being dropped.
func TestAllocate_DegradesToSnippetInsteadOfDrop(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("line of go code here\n", 200) // ~200 lines
	f := mapFetcher{drCit("big.go").Key(): big}
	a, err := New(f, WithConfig(Config{
		MaxTokens: 220, OverheadReserve: 0, MaxCitations: 12,
		NeighborReserve: 0, SnippetLines: 8,
	}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.Allocate(context.Background(),
		[]stage2.ScoredCitation{{Citation: drCit("big.go"), Score: 1.0}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Selected) != 1 || !out.Selected[0].Degraded {
		t.Fatalf("want 1 degraded selection, got %+v (skipped=%d)", out.Selected, len(out.Skipped))
	}
	if !strings.Contains(out.Selected[0].Body, "truncated to head snippet") {
		t.Fatalf("snippet marker missing: %q", out.Selected[0].Body[:80])
	}
}

// The neighbor reserve must prevent seeds from monopolizing every body slot.
func TestAllocate_NeighborReservePreventsStarvation(t *testing.T) {
	t.Parallel()
	f := mapFetcher{}
	seeds := make([]stage2.ScoredCitation, 0, 6)
	for _, name := range []string{"s1.go", "s2.go", "s3.go", "s4.go", "s5.go", "s6.go"} {
		c := drCit(name)
		f[c.Key()] = "seed body"
		seeds = append(seeds, stage2.ScoredCitation{Citation: c, Score: 1.0})
	}
	nc := drCit("n1.go")
	f[nc.Key()] = "neighbor body"
	neighbors := []stage3.ScoredNeighbor{{
		Edge:  contract.Neighbor{Source: drCit("s1.go"), Target: nc, Relation: contract.RelationCalls, Distance: 1},
		Score: 0.1, // 항상 시드보다 낮음
	}}
	a, err := New(f, WithConfig(Config{
		MaxTokens: 10000, OverheadReserve: 0, MaxCitations: 4,
		NeighborReserve: 1, SnippetLines: 8,
	}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.Allocate(context.Background(), seeds, neighbors)
	if err != nil {
		t.Fatal(err)
	}
	gotNeighbor := false
	for _, sel := range out.Selected {
		if sel.Origin == OriginNeighbor {
			gotNeighbor = true
		}
	}
	if !gotNeighbor {
		t.Fatalf("neighbor starved despite reserve: %+v", out.Selected)
	}
	if len(out.Selected) > 4 {
		t.Fatalf("citation cap exceeded: %d", len(out.Selected))
	}
}
