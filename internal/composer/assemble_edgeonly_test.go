package composer

import (
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/composer/budget"
	"github.com/0xmhha/code-knowledge-system/internal/composer/sanitize"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func edgeCit(file string) contract.Citation {
	return contract.Citation{File: file, StartLine: 1, EndLine: 10, CommitHash: "c"}
}

// Cap-truncated neighbor targets (never evaluated by Stage 4) must join the
// pack as edge-only citations so the relation wiring survives — the
// neighbor-starvation regression.
func TestAssemblePack_EdgeOnlyTargetIncluded(t *testing.T) {
	t.Parallel()
	seed, target := edgeCit("seed.go"), edgeCit("target.go")
	s3 := stage3.Stage3Output{Neighbors: []stage3.ScoredNeighbor{{
		Edge: contract.Neighbor{Source: seed, Target: target, Relation: contract.RelationCalls, Distance: 1},
	}}}
	s4 := budget.Stage4Output{BudgetTokens: 100}
	s5 := sanitize.Stage5Output{Items: []sanitize.SanitizedItem{{Citation: seed, Body: "body"}}}

	pack := assemblePack("q", contract.IntentUnknown, s3, s4, s5, "test")
	if !pack.IsValid() {
		t.Fatalf("pack invalid: %+v", pack)
	}
	if len(pack.GraphNeighbors) != 1 {
		t.Fatalf("edge lost: %+v", pack.GraphNeighbors)
	}
	foundTarget := false
	for _, c := range pack.Citations {
		if c.File == "target.go" {
			foundTarget = true
		}
	}
	if !foundTarget {
		t.Fatal("edge-only target citation missing")
	}
	for _, b := range pack.Bodies {
		if b.Citation.File == "target.go" {
			t.Fatal("edge-only target must not carry a body")
		}
	}
}

// Targets Stage 4 evaluated and rejected (Skipped) must NOT ride in
// edge-only — their absence is a signal, not a budget artifact.
func TestAssemblePack_SkippedTargetExcluded(t *testing.T) {
	t.Parallel()
	seed, orphan := edgeCit("seed.go"), edgeCit("orphan.go")
	s3 := stage3.Stage3Output{Neighbors: []stage3.ScoredNeighbor{{
		Edge: contract.Neighbor{Source: seed, Target: orphan, Relation: contract.RelationCalls, Distance: 1},
	}}}
	s4 := budget.Stage4Output{BudgetTokens: 100, Skipped: []contract.Citation{orphan}}
	s5 := sanitize.Stage5Output{Items: []sanitize.SanitizedItem{{Citation: seed, Body: "body"}}}

	pack := assemblePack("q", contract.IntentUnknown, s3, s4, s5, "test")
	if len(pack.GraphNeighbors) != 0 {
		t.Fatalf("rejected target's edge leaked: %+v", pack.GraphNeighbors)
	}
	for _, c := range pack.Citations {
		if c.File == "orphan.go" {
			t.Fatal("rejected target joined citations")
		}
	}
}
