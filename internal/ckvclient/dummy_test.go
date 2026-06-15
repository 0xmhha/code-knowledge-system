package ckvclient

import (
	"context"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func TestDummy_SemanticSearch_RecordsInstruction(t *testing.T) {
	d := NewDummy()
	coll := contract.NewInstructionCollector()
	ctx := contract.WithCollector(context.Background(), coll)

	hits, err := d.SemanticSearch(ctx, "consensus failure handling", SearchOpts{
		K:      5,
		Filter: SearchFilter{Language: "go"},
	})
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits: got %d, want 1 placeholder", len(hits))
	}
	if hits[0].Source != contract.HitSourceCKV {
		t.Errorf("hit.Source: got %q, want %q", hits[0].Source, contract.HitSourceCKV)
	}

	got := coll.All()
	if len(got) != 1 {
		t.Fatalf("collector: got %d instructions, want 1", len(got))
	}
	inst := got[0]
	if inst.Backend != "ckv" {
		t.Errorf("Backend: got %q, want ckv", inst.Backend)
	}
	if inst.Operation != "SemanticSearch" {
		t.Errorf("Operation: got %q, want SemanticSearch", inst.Operation)
	}
	if inst.Query != "consensus failure handling" {
		t.Errorf("Query: got %q", inst.Query)
	}
	if inst.SkillPath != d.skill() {
		t.Errorf("SkillPath: got %q, want %q", inst.SkillPath, d.skill())
	}
	if inst.SourcePath != d.source() {
		t.Errorf("SourcePath: got %q, want %q", inst.SourcePath, d.source())
	}
	if inst.Args["k"] != "5" {
		t.Errorf("Args[k]: got %q, want 5", inst.Args["k"])
	}
	if inst.Args["language"] != "go" {
		t.Errorf("Args[language]: got %q", inst.Args["language"])
	}
	if !strings.Contains(inst.Directive, "consensus failure handling") {
		t.Errorf("Directive missing query: %q", inst.Directive)
	}
}

func TestDummy_NoCollectorIsHarmless(t *testing.T) {
	d := NewDummy()
	hits, err := d.SemanticSearch(context.Background(), "anything", SearchOpts{K: 1})
	if err != nil {
		t.Fatalf("SemanticSearch without collector: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("hits without collector: got %d, want 1", len(hits))
	}
}

func TestDummy_Freshness_RecordsInstruction(t *testing.T) {
	d := NewDummy()
	coll := contract.NewInstructionCollector()
	ctx := contract.WithCollector(context.Background(), coll)

	report, err := d.Freshness(ctx)
	if err != nil {
		t.Fatalf("Freshness: %v", err)
	}
	if !report.Fresh {
		t.Errorf("Fresh: got false, want true")
	}
	if coll.Len() != 1 {
		t.Fatalf("collector len: got %d, want 1", coll.Len())
	}
	if coll.All()[0].Operation != "Freshness" {
		t.Errorf("Operation: got %q", coll.All()[0].Operation)
	}
}
