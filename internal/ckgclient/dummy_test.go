package ckgclient

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func TestDummy_AllOperationsRecordInstructions(t *testing.T) {
	d := NewDummy()
	coll := contract.NewInstructionCollector()
	ctx := contract.WithCollector(context.Background(), coll)

	if _, err := d.BM25Search(ctx, "validator quorum", SearchOpts{K: 10}); err != nil {
		t.Fatalf("BM25Search: %v", err)
	}
	if _, err := d.FindSymbol(ctx, "Finalize", SymbolOpts{Kinds: []string{"function"}}); err != nil {
		t.Fatalf("FindSymbol: %v", err)
	}
	src := contract.Citation{File: "consensus/wbft/finalize.go", StartLine: 10, EndLine: 50}
	if _, err := d.Neighbors(ctx, src, NeighborsOpts{Hops: 2}); err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if _, err := d.ImpactOfChange(ctx, "consensus.wbft.Finalize", ImpactOpts{Depth: 2}); err != nil {
		t.Fatalf("ImpactOfChange: %v", err)
	}
	if _, err := d.EvidenceForIntent(ctx, "validator slashing bug", EvidenceOpts{K: 5}); err != nil {
		t.Fatalf("EvidenceForIntent: %v", err)
	}
	if _, err := d.GetNodePRs(ctx, "consensus.wbft.Finalize", PRRefOpts{MaxCount: 10}); err != nil {
		t.Fatalf("GetNodePRs: %v", err)
	}
	if _, _, err := d.GetSubgraph(ctx, "consensus.wbft.Finalize", SubgraphOpts{Depth: 2}); err != nil {
		t.Fatalf("GetSubgraph: %v", err)
	}

	want := []string{
		"BM25Search",
		"FindSymbol",
		"Neighbors",
		"ImpactOfChange",
		"EvidenceForIntent",
		"GetNodePRs",
		"GetSubgraph",
	}
	got := coll.All()
	if len(got) != len(want) {
		t.Fatalf("instructions: got %d, want %d", len(got), len(want))
	}
	for i, op := range want {
		if got[i].Operation != op {
			t.Errorf("op[%d]: got %q, want %q", i, got[i].Operation, op)
		}
		if got[i].Backend != "ckg" {
			t.Errorf("op[%d].Backend: got %q, want ckg", i, got[i].Backend)
		}
		if got[i].SkillPath == "" {
			t.Errorf("op[%d].SkillPath empty", i)
		}
		if got[i].SourcePath == "" {
			t.Errorf("op[%d].SourcePath empty", i)
		}
		if got[i].Directive == "" {
			t.Errorf("op[%d].Directive empty", i)
		}
		if got[i].Expected == "" {
			t.Errorf("op[%d].Expected empty", i)
		}
	}
}

func TestDummy_HealthDoesNotRecord(t *testing.T) {
	d := NewDummy()
	coll := contract.NewInstructionCollector()
	ctx := contract.WithCollector(context.Background(), coll)

	h, err := d.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.Reachable {
		t.Errorf("Reachable: got false, want true")
	}
	if coll.Len() != 0 {
		t.Errorf("collector len: got %d, want 0 (Health should not record)", coll.Len())
	}
}
