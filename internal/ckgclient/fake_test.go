package ckgclient

import (
	"context"
	"errors"
	"testing"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func goodCitation(file string) contract.Citation {
	return contract.Citation{File: file, StartLine: 1, EndLine: 10, CommitHash: "abc"}
}

func goodHit(file string, rank int, score float64) contract.Hit {
	return contract.Hit{
		Citation: goodCitation(file),
		Rank:     rank,
		Score:    score,
		Source:   contract.HitSourceCKG,
	}
}

// --- BM25Search ---

func TestFake_BM25Search_ReturnsCannedHits(t *testing.T) {
	t.Parallel()
	f := &Fake{
		BM25Hits: []contract.Hit{goodHit("a.go", 1, 5.2), goodHit("b.go", 2, 4.1)},
	}
	hits, err := f.BM25Search(context.Background(), "query", SearchOpts{})
	if err != nil {
		t.Fatalf("BM25Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
}

func TestFake_BM25Search_RespectsK(t *testing.T) {
	t.Parallel()
	f := &Fake{
		BM25Hits: []contract.Hit{
			goodHit("a.go", 1, 5.2),
			goodHit("b.go", 2, 4.1),
			goodHit("c.go", 3, 3.0),
		},
	}
	hits, err := f.BM25Search(context.Background(), "q", SearchOpts{K: 1})
	if err != nil {
		t.Fatalf("BM25Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
}

func TestFake_BM25Search_EmptyQueryErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{BM25Hits: []contract.Hit{goodHit("a.go", 1, 1.0)}}
	if _, err := f.BM25Search(context.Background(), "", SearchOpts{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFake_BM25Search_NegativeKErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{BM25Hits: []contract.Hit{goodHit("a.go", 1, 1.0)}}
	if _, err := f.BM25Search(context.Background(), "q", SearchOpts{K: -1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFake_BM25Search_ErrTakesPrecedence(t *testing.T) {
	t.Parallel()
	want := errors.New("backend down")
	f := &Fake{
		BM25Hits: []contract.Hit{goodHit("a.go", 1, 1.0)},
		BM25Err:  want,
	}
	if _, err := f.BM25Search(context.Background(), "q", SearchOpts{}); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// --- FindSymbol ---

func TestFake_FindSymbol(t *testing.T) {
	t.Parallel()
	f := &Fake{SymbolCitations: []contract.Citation{goodCitation("a.go"), goodCitation("b.go")}}
	cits, err := f.FindSymbol(context.Background(), "MyFunc", SymbolOpts{})
	if err != nil {
		t.Fatalf("FindSymbol: %v", err)
	}
	if len(cits) != 2 {
		t.Fatalf("citations = %d, want 2", len(cits))
	}
}

func TestFake_FindSymbol_AcceptsMultipleKinds(t *testing.T) {
	t.Parallel()
	f := &Fake{SymbolCitations: []contract.Citation{goodCitation("a.go")}}
	_, err := f.FindSymbol(context.Background(), "Process", SymbolOpts{Kinds: []string{"function", "method"}})
	if err != nil {
		t.Fatalf("FindSymbol with plural Kinds: %v", err)
	}
	// Recorded with both kinds preserved.
	if len(f.Calls.FindSymbol) != 1 {
		t.Fatalf("FindSymbol calls = %d, want 1", len(f.Calls.FindSymbol))
	}
	got := f.Calls.FindSymbol[0].Opts.Kinds
	if len(got) != 2 || got[0] != "function" || got[1] != "method" {
		t.Errorf("recorded Kinds = %v, want [function method]", got)
	}
}

func TestFake_FindSymbol_EmptyNameErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	if _, err := f.FindSymbol(context.Background(), "", SymbolOpts{}); err == nil {
		t.Fatal("expected error for empty symbol name")
	}
}

func TestFake_FindSymbol_Err(t *testing.T) {
	t.Parallel()
	want := errors.New("schema mismatch")
	f := &Fake{SymbolErr: want}
	if _, err := f.FindSymbol(context.Background(), "X", SymbolOpts{}); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// --- Neighbors ---

func TestFake_Neighbors(t *testing.T) {
	t.Parallel()
	src := goodCitation("a.go")
	tgt := goodCitation("b.go")
	f := &Fake{
		NeighborEdges: []contract.Neighbor{
			{Source: src, Target: tgt, Relation: contract.RelationCalls, Distance: 1},
		},
	}
	ns, err := f.Neighbors(context.Background(), src, NeighborsOpts{Hops: 1})
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(ns) != 1 || ns[0].Relation != contract.RelationCalls {
		t.Fatalf("Neighbors = %+v", ns)
	}
}

func TestFake_Neighbors_ZeroHopsAllowed(t *testing.T) {
	t.Parallel()
	src := goodCitation("a.go")
	f := &Fake{NeighborEdges: []contract.Neighbor{
		{Source: src, Target: goodCitation("b.go"), Relation: contract.RelationCalls, Distance: 1},
	}}
	// Hops == 0 is treated as 1 per doc; should not error.
	if _, err := f.Neighbors(context.Background(), src, NeighborsOpts{}); err != nil {
		t.Fatalf("Neighbors with Hops=0: %v", err)
	}
}

func TestFake_Neighbors_NegativeHopsErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	_, err := f.Neighbors(context.Background(), goodCitation("a.go"), NeighborsOpts{Hops: -1})
	if err == nil {
		t.Fatal("expected error for negative hops")
	}
}

func TestFake_Neighbors_InvalidSrcErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	if _, err := f.Neighbors(context.Background(), contract.Citation{}, NeighborsOpts{Hops: 1}); err == nil {
		t.Fatal("expected error for invalid src citation")
	}
}

func TestFake_Neighbors_RespectsMaxTotal(t *testing.T) {
	t.Parallel()
	src := goodCitation("a.go")
	f := &Fake{NeighborEdges: []contract.Neighbor{
		{Source: src, Target: goodCitation("b.go"), Relation: contract.RelationCalls, Distance: 1},
		{Source: src, Target: goodCitation("c.go"), Relation: contract.RelationCalls, Distance: 1},
		{Source: src, Target: goodCitation("d.go"), Relation: contract.RelationCalls, Distance: 1},
	}}
	ns, err := f.Neighbors(context.Background(), src, NeighborsOpts{Hops: 1, MaxTotal: 2})
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(ns) != 2 {
		t.Fatalf("with MaxTotal=2 got %d neighbors, want 2", len(ns))
	}
}

// --- Health / Close ---

func TestFake_Health(t *testing.T) {
	t.Parallel()
	f := &Fake{HealthVal: Health{Reachable: true, SchemaVersion: "v1", IndexedHead: "abc"}}
	h, err := f.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.Reachable || h.SchemaVersion != "v1" || h.IndexedHead != "abc" {
		t.Errorf("Health = %+v", h)
	}
}

func TestFake_Health_Err(t *testing.T) {
	t.Parallel()
	want := errors.New("timeout")
	f := &Fake{HealthErr: want}
	if _, err := f.Health(context.Background()); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestFake_Close(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !f.Closed() {
		t.Fatal("Closed() should be true after Close()")
	}
}

// --- Call recording ---

func TestFake_RecordsAllMethods(t *testing.T) {
	t.Parallel()
	src := goodCitation("a.go")
	f := &Fake{
		BM25Hits:        []contract.Hit{goodHit("a.go", 1, 1.0)},
		SymbolCitations: []contract.Citation{goodCitation("b.go")},
		NeighborEdges:   []contract.Neighbor{{Source: src, Target: src, Relation: contract.RelationCalls, Distance: 1}},
	}

	_, _ = f.BM25Search(context.Background(), "q1", SearchOpts{K: 5})
	_, _ = f.BM25Search(context.Background(), "q2", SearchOpts{})
	_, _ = f.FindSymbol(context.Background(), "Foo", SymbolOpts{Kinds: []string{"function"}})
	_, _ = f.Neighbors(context.Background(), src, NeighborsOpts{Hops: 2, Relations: []contract.Relation{contract.RelationCalls}})
	_, _ = f.Health(context.Background())
	_ = f.Close()

	if len(f.Calls.BM25Search) != 2 {
		t.Errorf("BM25Search count = %d, want 2", len(f.Calls.BM25Search))
	}
	if len(f.Calls.FindSymbol) != 1 || f.Calls.FindSymbol[0].Name != "Foo" {
		t.Errorf("FindSymbol record = %+v", f.Calls.FindSymbol)
	}
	if len(f.Calls.Neighbors) != 1 || f.Calls.Neighbors[0].Opts.Hops != 2 {
		t.Errorf("Neighbors record = %+v", f.Calls.Neighbors)
	}
	if f.Calls.Health != 1 || f.Calls.Close != 1 {
		t.Errorf("Health/Close counts: %d/%d", f.Calls.Health, f.Calls.Close)
	}
}

func TestFake_RecordsCalls_EvenOnError(t *testing.T) {
	t.Parallel()
	f := &Fake{BM25Err: errors.New("backend down")}
	_, _ = f.BM25Search(context.Background(), "x", SearchOpts{})
	if len(f.Calls.BM25Search) != 1 {
		t.Fatalf("call not recorded on error: %d", len(f.Calls.BM25Search))
	}
}

func TestFake_CallsReset(t *testing.T) {
	t.Parallel()
	f := &Fake{BM25Hits: []contract.Hit{goodHit("a.go", 1, 1.0)}}
	_, _ = f.BM25Search(context.Background(), "x", SearchOpts{})
	_ = f.Close()

	f.Calls.Reset()

	if len(f.Calls.BM25Search) != 0 || f.Calls.Close != 0 {
		t.Fatalf("Reset did not clear all counters: %+v", f.Calls)
	}
}
