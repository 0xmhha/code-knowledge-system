package ckgclient

import (
	"context"
	"errors"
	"testing"
	"time"

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
	ns, err := f.Neighbors(context.Background(), src, nil, 1)
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(ns) != 1 || ns[0].Relation != contract.RelationCalls {
		t.Fatalf("Neighbors = %+v", ns)
	}
}

func TestFake_Neighbors_InvalidSrcErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	if _, err := f.Neighbors(context.Background(), contract.Citation{}, nil, 1); err == nil {
		t.Fatal("expected error for invalid src citation")
	}
}

func TestFake_Neighbors_ZeroHopsErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	if _, err := f.Neighbors(context.Background(), goodCitation("a.go"), nil, 0); err == nil {
		t.Fatal("expected error for hops == 0")
	}
}

// --- Health / Close ---

func TestFake_Health(t *testing.T) {
	t.Parallel()
	f := &Fake{HealthVal: Health{Reachable: true, Latency: 5 * time.Millisecond, SchemaVersion: "v1"}}
	h, err := f.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.Reachable || h.SchemaVersion != "v1" {
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
