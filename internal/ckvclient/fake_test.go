package ckvclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func goodHit(file string, rank int, score float64) contract.Hit {
	return contract.Hit{
		Citation: contract.Citation{File: file, StartLine: 1, EndLine: 5, CommitHash: "abc"},
		Rank:     rank,
		Score:    score,
		Source:   contract.HitSourceCKV,
	}
}

func TestFake_SemanticSearch_ReturnsCannedHits(t *testing.T) {
	t.Parallel()
	f := &Fake{
		SearchHits: []contract.Hit{
			goodHit("a.go", 1, 0.9),
			goodHit("b.go", 2, 0.7),
		},
	}
	hits, err := f.SemanticSearch(context.Background(), "query", SearchOpts{})
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	if hits[0].Citation.File != "a.go" {
		t.Errorf("hits[0].File = %q", hits[0].Citation.File)
	}
}

func TestFake_SemanticSearch_RespectsK(t *testing.T) {
	t.Parallel()
	f := &Fake{
		SearchHits: []contract.Hit{
			goodHit("a.go", 1, 0.9),
			goodHit("b.go", 2, 0.7),
			goodHit("c.go", 3, 0.5),
		},
	}
	hits, err := f.SemanticSearch(context.Background(), "query", SearchOpts{K: 2})
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("with K=2 got %d hits, want 2", len(hits))
	}
}

func TestFake_SemanticSearch_EmptyQueryErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{SearchHits: []contract.Hit{goodHit("a.go", 1, 0.9)}}
	_, err := f.SemanticSearch(context.Background(), "", SearchOpts{})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestFake_SemanticSearch_NegativeKErrors(t *testing.T) {
	t.Parallel()
	f := &Fake{SearchHits: []contract.Hit{goodHit("a.go", 1, 0.9)}}
	_, err := f.SemanticSearch(context.Background(), "q", SearchOpts{K: -1})
	if err == nil {
		t.Fatal("expected error for K < 0")
	}
}

func TestFake_SemanticSearch_ErrTakesPrecedence(t *testing.T) {
	t.Parallel()
	want := errors.New("backend down")
	f := &Fake{
		SearchHits: []contract.Hit{goodHit("a.go", 1, 0.9)}, // populated
		SearchErr:  want,                                    // overrides
	}
	_, err := f.SemanticSearch(context.Background(), "q", SearchOpts{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestFake_Health(t *testing.T) {
	t.Parallel()
	f := &Fake{
		HealthVal: Health{Reachable: true, Latency: 10 * time.Millisecond, StatsHash: "xyz"},
	}
	h, err := f.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.Reachable || h.StatsHash != "xyz" {
		t.Errorf("Health = %+v", h)
	}
}

func TestFake_Health_Err(t *testing.T) {
	t.Parallel()
	want := errors.New("timeout")
	f := &Fake{HealthErr: want}
	_, err := f.Health(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestFake_Close(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	if f.Closed() {
		t.Fatal("Closed() should be false before Close()")
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !f.Closed() {
		t.Fatal("Closed() should be true after Close()")
	}
}

func TestFake_Close_Err(t *testing.T) {
	t.Parallel()
	want := errors.New("close failed")
	f := &Fake{CloseErr: want}
	if err := f.Close(); !errors.Is(err, want) {
		t.Fatalf("Close err = %v, want %v", err, want)
	}
	if !f.Closed() {
		t.Fatal("Closed() should be true even when Close returns error")
	}
}
