package ckvclient

import (
	"context"
	"errors"
	"testing"

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

// --- SemanticSearch ---

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
		SearchHits: []contract.Hit{goodHit("a.go", 1, 0.9)},
		SearchErr:  want,
	}
	_, err := f.SemanticSearch(context.Background(), "q", SearchOpts{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// --- Health / Close ---

func TestFake_Health(t *testing.T) {
	t.Parallel()
	f := &Fake{
		HealthVal: Health{Reachable: true, StatsHash: "xyz"},
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

// --- Call recording ---

func TestFake_RecordsSemanticSearchCalls(t *testing.T) {
	t.Parallel()
	f := &Fake{SearchHits: []contract.Hit{goodHit("a.go", 1, 0.5)}}

	_, _ = f.SemanticSearch(context.Background(), "first", SearchOpts{K: 3})
	_, _ = f.SemanticSearch(context.Background(), "second", SearchOpts{K: 5, Filter: SearchFilter{Language: "go"}})

	if got := len(f.Calls.SemanticSearch); got != 2 {
		t.Fatalf("SemanticSearch call count = %d, want 2", got)
	}
	if f.Calls.SemanticSearch[0].Query != "first" || f.Calls.SemanticSearch[0].Opts.K != 3 {
		t.Errorf("call[0] = %+v", f.Calls.SemanticSearch[0])
	}
	if f.Calls.SemanticSearch[1].Opts.Filter.Language != "go" {
		t.Errorf("call[1] filter = %+v", f.Calls.SemanticSearch[1].Opts.Filter)
	}
}

func TestFake_RecordsCalls_EvenOnError(t *testing.T) {
	t.Parallel()
	// Recording must happen before the error-return branch so tests can
	// still assert "the composer attempted to search even though it
	// failed".
	f := &Fake{SearchErr: errors.New("backend down")}
	_, _ = f.SemanticSearch(context.Background(), "x", SearchOpts{})
	if len(f.Calls.SemanticSearch) != 1 {
		t.Fatalf("call not recorded on error: %d", len(f.Calls.SemanticSearch))
	}
}

func TestFake_RecordsHealthAndCloseCounts(t *testing.T) {
	t.Parallel()
	f := &Fake{}
	_, _ = f.Health(context.Background())
	_, _ = f.Health(context.Background())
	_ = f.Close()
	_ = f.Close()
	_ = f.Close()
	if f.Calls.Health != 2 {
		t.Errorf("Health count = %d, want 2", f.Calls.Health)
	}
	if f.Calls.Close != 3 {
		t.Errorf("Close count = %d, want 3", f.Calls.Close)
	}
}

func TestFake_CallsReset(t *testing.T) {
	t.Parallel()
	f := &Fake{SearchHits: []contract.Hit{goodHit("a.go", 1, 0.5)}}
	_, _ = f.SemanticSearch(context.Background(), "x", SearchOpts{})
	_, _ = f.Health(context.Background())
	_ = f.Close()

	f.Calls.Reset()

	if len(f.Calls.SemanticSearch) != 0 || f.Calls.Health != 0 || f.Calls.Close != 0 {
		t.Fatalf("Reset did not clear all counters: %+v", f.Calls)
	}
}
