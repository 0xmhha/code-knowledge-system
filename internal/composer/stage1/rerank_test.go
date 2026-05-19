package stage1

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// rerankFake gives the test deterministic per-keyword BM25 hits without
// going through a real ckg backend. Each keyword's hits are a list of
// (score) pairs; the citations are filler — only score matters for the
// reranker.
type rerankFake struct {
	hitsByKW map[string][]contract.Hit
}

func (r *rerankFake) BM25Search(_ context.Context, q string, _ ckgclient.SearchOpts) ([]contract.Hit, error) {
	return r.hitsByKW[q], nil
}
func (r *rerankFake) FindSymbol(_ context.Context, _ string, _ ckgclient.SymbolOpts) ([]contract.Citation, error) {
	return nil, nil
}
func (r *rerankFake) Neighbors(_ context.Context, _ contract.Citation, _ ckgclient.NeighborsOpts) ([]contract.Neighbor, error) {
	return nil, nil
}
func (r *rerankFake) Health(_ context.Context) (ckgclient.Health, error) {
	return ckgclient.Health{Reachable: true}, nil
}
func (r *rerankFake) Close() error { return nil }

// hitScore is a Hit shorthand for tests — only score matters; citation
// is a deterministic placeholder.
func hitScore(s float64) contract.Hit {
	return contract.Hit{
		Citation: contract.Citation{File: "x.go", StartLine: 1, EndLine: 1},
		Score:    s,
	}
}

func TestRerank_PicksByMaxScoreNotSum(t *testing.T) {
	t.Parallel()
	// Regression scenario: a unique identifier with one strong hit
	// (score=1.0) must outrank a common word with K mediocre hits
	// (each score=0.6). Under the previous sum-based algorithm the
	// common word's sum=3.0 beat the unique identifier's sum=1.0,
	// causing rare-but-precise tokens to drop out at the
	// MaxKeywords cap. Under max(), the unique identifier wins.
	ckg := &rerankFake{
		hitsByKW: map[string][]contract.Hit{
			"ErrFailClosed": {hitScore(1.0)},
			"level":         {hitScore(0.6), hitScore(0.6), hitScore(0.6), hitScore(0.6), hitScore(0.6)},
		},
	}
	e := mustExtractor(t, ckg)
	out, _ := e.rerank(context.Background(), []string{"ErrFailClosed", "level"})

	if len(out) != 2 {
		t.Fatalf("expected both keywords kept, got %v", out)
	}
	if out[0] != "ErrFailClosed" {
		t.Errorf("top keyword = %q, want \"ErrFailClosed\" (max=1.0 beats max=0.6)", out[0])
	}
}

func TestRerank_DropsZeroScoreKeywords(t *testing.T) {
	t.Parallel()
	ckg := &rerankFake{
		hitsByKW: map[string][]contract.Hit{
			"sentinel":      nil,                       // no hits at all
			"ErrFailClosed": {hitScore(1.0)},
		},
	}
	e := mustExtractor(t, ckg)
	out, _ := e.rerank(context.Background(), []string{"sentinel", "ErrFailClosed"})
	if len(out) != 1 || out[0] != "ErrFailClosed" {
		t.Errorf("out = %v, want only [ErrFailClosed]", out)
	}
}

func TestRerank_TieBreakByStableSort(t *testing.T) {
	t.Parallel()
	// Same max score → input order wins (SliceStable preserves).
	ckg := &rerankFake{
		hitsByKW: map[string][]contract.Hit{
			"alpha": {hitScore(1.0)},
			"beta":  {hitScore(1.0)},
		},
	}
	e := mustExtractor(t, ckg)
	out, _ := e.rerank(context.Background(), []string{"alpha", "beta"})
	if len(out) != 2 || out[0] != "alpha" || out[1] != "beta" {
		t.Errorf("stable tie-break broken: got %v, want [alpha beta]", out)
	}
}

// mustExtractor builds an Extractor wired with the rerank fake. Stage 1
// requires both ckv and ckg; SemanticSearch isn't used by rerank() so
// the ckv fake is the standard ckvclient.Fake.
func mustExtractor(t *testing.T, ckg ckgclient.Client) *Extractor {
	t.Helper()
	ckv := &ckvclient.Fake{}
	e, err := New(ckv, ckg)
	if err != nil {
		t.Fatalf("stage1.New: %v", err)
	}
	return e
}
