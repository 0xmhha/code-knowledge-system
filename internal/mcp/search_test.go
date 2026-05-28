package mcp

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- semantic_search ---

func TestHandleSemanticSearch_MissingQuery_IsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	res, err := handleSemanticSearch(context.Background(), f.deps, callToolReq(nil))
	if err != nil {
		t.Fatalf("handleSemanticSearch: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError for missing query; got %+v", res)
	}
}

func TestHandleSemanticSearch_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.SearchHits = []contract.Hit{
			hit("a.go", 1, 50, 0.95, contract.HitSourceCKV),
			hit("b.go", 1, 50, 0.80, contract.HitSourceCKV),
		}
	})
	req := callToolReq(map[string]any{
		"query":    "validator quorum check",
		"k":        float64(5),
		"language": "go",
		"kinds":    "function",
	})
	res, err := handleSemanticSearch(context.Background(), f.deps, req)
	if err != nil {
		t.Fatalf("handleSemanticSearch: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out searchResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Query != "validator quorum check" {
		t.Errorf("Query = %q", out.Query)
	}
	if len(out.Hits) != 2 {
		t.Errorf("Hits = %d, want 2", len(out.Hits))
	}
	// Filter propagation.
	if len(f.ckv.Calls.SemanticSearch) != 1 {
		t.Fatalf("SemanticSearch calls = %d", len(f.ckv.Calls.SemanticSearch))
	}
	got := f.ckv.Calls.SemanticSearch[0].Opts
	if got.K != 5 {
		t.Errorf("K = %d, want 5", got.K)
	}
	if got.Filter.Language != "go" {
		t.Errorf("Language = %q", got.Filter.Language)
	}
	if len(got.Filter.SymbolKinds) != 1 || got.Filter.SymbolKinds[0] != "function" {
		t.Errorf("SymbolKinds = %v", got.Filter.SymbolKinds)
	}
}

// --- search_text ---

func TestHandleSearchText_MissingQuery_IsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	res, err := handleSearchText(context.Background(), f.deps, callToolReq(nil))
	if err != nil {
		t.Fatalf("handleSearchText: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError for missing query; got %+v", res)
	}
}

func TestHandleSearchText_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckg.BM25Hits = []contract.Hit{
			hit("consensus/wbft/finalize.go", 100, 150, 7.5, contract.HitSourceCKG),
		}
	})
	req := callToolReq(map[string]any{
		"query":     "verifyVotes quorum",
		"k":         float64(3),
		"path_glob": "consensus/**",
	})
	res, err := handleSearchText(context.Background(), f.deps, req)
	if err != nil {
		t.Fatalf("handleSearchText: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out searchResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Hits) != 1 {
		t.Errorf("Hits = %d, want 1", len(out.Hits))
	}
	if len(f.ckg.Calls.BM25Search) != 1 {
		t.Fatalf("BM25Search calls = %d", len(f.ckg.Calls.BM25Search))
	}
	got := f.ckg.Calls.BM25Search[0].Opts
	if got.K != 3 {
		t.Errorf("K = %d, want 3", got.K)
	}
	if got.Filter.PathGlob != "consensus/**" {
		t.Errorf("PathGlob = %q", got.Filter.PathGlob)
	}
}
