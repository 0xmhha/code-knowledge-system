package stage2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- helpers ---

func cit(file string, start, end int) contract.Citation {
	return contract.Citation{File: file, StartLine: start, EndLine: end, CommitHash: "abc"}
}

func bm25Hit(file string, start, end int, score float64) contract.Hit {
	return contract.Hit{
		Citation: cit(file, start, end),
		Rank:     1,
		Score:    score,
		Source:   contract.HitSourceCKG,
	}
}

// --- New / construction ---

func TestNew_NilCKGErrors(t *testing.T) {
	t.Parallel()
	_, err := New(nil)
	if err == nil {
		t.Fatal("expected error for nil ckg")
	}
}

func TestNew_AppliesDefaultConfig(t *testing.T) {
	t.Parallel()
	s, err := New(&ckgclient.Fake{})
	if err != nil {
		t.Fatal(err)
	}
	if s.config.BM25K != DefaultBM25K {
		t.Errorf("BM25K = %d, want %d", s.config.BM25K, DefaultBM25K)
	}
	if s.config.MaxCitations != DefaultMaxCitations {
		t.Errorf("MaxCitations = %d, want %d", s.config.MaxCitations, DefaultMaxCitations)
	}
	if s.config.SymbolBonus != DefaultSymbolBonus {
		t.Errorf("SymbolBonus = %v, want %v", s.config.SymbolBonus, DefaultSymbolBonus)
	}
}

// --- Search / happy paths ---

func TestSearch_EmptyKeywordsReturnsEmpty(t *testing.T) {
	t.Parallel()
	s, _ := New(&ckgclient.Fake{})
	out, err := s.Search(context.Background(), nil, contract.IntentBugFix)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(out.Citations) != 0 || len(out.Hits) != 0 {
		t.Fatalf("expected empty output, got %+v", out)
	}
}

func TestSearch_BM25HitsContribute(t *testing.T) {
	t.Parallel()
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{bm25Hit("login.go", 10, 20, 8.0)},
	}
	s, _ := New(ckg)

	out, err := s.Search(context.Background(), []string{"Login"}, contract.IntentBugFix)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Citations) != 1 {
		t.Fatalf("Citations count = %d, want 1", len(out.Citations))
	}
	if out.Citations[0].Score != 8.0 {
		t.Errorf("Score = %v, want 8.0", out.Citations[0].Score)
	}
	if len(out.Citations[0].Sources) != 1 || !strings.HasPrefix(out.Citations[0].Sources[0], "bm25:Login") {
		t.Errorf("Sources = %v", out.Citations[0].Sources)
	}
}

func TestSearch_SymbolHitsAddBonus(t *testing.T) {
	t.Parallel()
	ckg := &ckgclient.Fake{
		SymbolCitations: []contract.Citation{cit("login.go", 10, 20)},
	}
	s, _ := New(ckg)

	out, err := s.Search(context.Background(), []string{"Login"}, contract.IntentBugFix)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Citations) != 1 {
		t.Fatalf("Citations count = %d, want 1", len(out.Citations))
	}
	if out.Citations[0].Score != DefaultSymbolBonus {
		t.Errorf("Score = %v, want %v", out.Citations[0].Score, DefaultSymbolBonus)
	}
	if !strings.Contains(out.Citations[0].Sources[0], "symbol:Login") {
		t.Errorf("Sources = %v", out.Citations[0].Sources)
	}
}

func TestSearch_BM25AndSymbolSumOnSameCitation(t *testing.T) {
	t.Parallel()
	// ckg returns the same citation from both BM25 and Symbol — score
	// should sum. Score = 8.0 + DefaultSymbolBonus.
	ckg := &ckgclient.Fake{
		BM25Hits:        []contract.Hit{bm25Hit("login.go", 10, 20, 8.0)},
		SymbolCitations: []contract.Citation{cit("login.go", 10, 20)},
	}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"Login"}, contract.IntentBugFix)
	if len(out.Citations) != 1 {
		t.Fatalf("Citations count = %d, want 1 (dedup)", len(out.Citations))
	}
	want := 8.0 + DefaultSymbolBonus
	if out.Citations[0].Score != want {
		t.Errorf("Score = %v, want %v", out.Citations[0].Score, want)
	}
	if len(out.Citations[0].Sources) != 2 {
		t.Errorf("Sources length = %d, want 2", len(out.Citations[0].Sources))
	}
}

func TestSearch_MultipleKeywordsAccumulate(t *testing.T) {
	t.Parallel()
	// Each keyword's BM25Search returns the same canned hit (Fake limit:
	// no per-query branching). So both keywords add to the same citation.
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{bm25Hit("handler.go", 1, 50, 3.0)},
	}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"alpha", "beta"}, contract.IntentBugFix)
	if len(out.Citations) != 1 {
		t.Fatalf("Citations count = %d, want 1", len(out.Citations))
	}
	// Two keywords each contributing 3.0 -> 6.0 total.
	if out.Citations[0].Score != 6.0 {
		t.Errorf("Score = %v, want 6.0", out.Citations[0].Score)
	}
	if len(out.Citations[0].Sources) != 2 {
		t.Errorf("Sources length = %d, want 2", len(out.Citations[0].Sources))
	}
}

func TestSearch_SortsByScoreDescending(t *testing.T) {
	t.Parallel()
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{
			bm25Hit("low.go", 1, 10, 1.0),
			bm25Hit("high.go", 1, 10, 9.0),
			bm25Hit("mid.go", 1, 10, 5.0),
		},
	}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"k"}, contract.IntentBugFix)
	if len(out.Citations) != 3 {
		t.Fatalf("Citations count = %d, want 3", len(out.Citations))
	}
	want := []string{"high.go", "mid.go", "low.go"}
	for i, sc := range out.Citations {
		if sc.Citation.File != want[i] {
			t.Errorf("Citations[%d].File = %q, want %q", i, sc.Citation.File, want[i])
		}
	}
}

func TestSearch_TiesBrokenByFileThenStartLine(t *testing.T) {
	t.Parallel()
	// All hits have the same score -> deterministic tiebreaker by file,
	// then start_line.
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{
			bm25Hit("b.go", 30, 40, 5.0),
			bm25Hit("a.go", 1, 10, 5.0),
			bm25Hit("b.go", 1, 10, 5.0),
		},
	}
	s, _ := New(ckg)
	out, _ := s.Search(context.Background(), []string{"k"}, contract.IntentBugFix)
	want := []contract.Citation{
		cit("a.go", 1, 10),
		cit("b.go", 1, 10),
		cit("b.go", 30, 40),
	}
	for i, sc := range out.Citations {
		if sc.Citation != want[i] {
			t.Errorf("Citations[%d] = %+v, want %+v", i, sc.Citation, want[i])
		}
	}
}

func TestSearch_RespectsMaxCitations(t *testing.T) {
	t.Parallel()
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{
			bm25Hit("a.go", 1, 1, 5),
			bm25Hit("b.go", 1, 1, 4),
			bm25Hit("c.go", 1, 1, 3),
			bm25Hit("d.go", 1, 1, 2),
			bm25Hit("e.go", 1, 1, 1),
		},
	}
	s, _ := New(ckg, WithConfig(Config{
		BM25K:        DefaultBM25K,
		MaxCitations: 2,
		SymbolBonus:  DefaultSymbolBonus,
	}))

	out, _ := s.Search(context.Background(), []string{"k"}, contract.IntentBugFix)
	if len(out.Citations) != 2 {
		t.Fatalf("Citations count = %d, want 2 (capped)", len(out.Citations))
	}
}

// --- Coverage / FailedKeywords ---

func TestSearch_FailedKeywordsAndCoverage(t *testing.T) {
	t.Parallel()
	// Fake returns empty hits AND empty symbols by default — all
	// keywords fail.
	ckg := &ckgclient.Fake{}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"a", "b", "c"}, contract.IntentBugFix)
	if len(out.FailedKeywords) != 3 {
		t.Errorf("FailedKeywords = %v, want all 3 failed", out.FailedKeywords)
	}
	if out.Coverage != 0 {
		t.Errorf("Coverage = %v, want 0", out.Coverage)
	}
}

func TestSearch_PartialCoverage(t *testing.T) {
	t.Parallel()
	// Fake always returns the same canned BM25 hit, so any keyword that
	// triggers a BM25 call gets a hit. To simulate partial failure, we'd
	// need per-query branching in the fake. Workaround: set BM25Err to
	// always fail BM25; SymbolCitations only returns on FindSymbol. So
	// every keyword's only success path is Symbol — but Fake returns
	// the same SymbolCitations regardless of name. Therefore in this
	// fake setup, all keywords hit. To test partial failure, we rely
	// on a single-keyword setup.
	//
	// This test verifies coverage = 1.0 when all keywords hit at least
	// one of BM25/Symbol.
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{bm25Hit("x.go", 1, 1, 1.0)},
	}
	s, _ := New(ckg)
	out, _ := s.Search(context.Background(), []string{"a", "b"}, contract.IntentBugFix)
	if out.Coverage != 1.0 {
		t.Errorf("Coverage = %v, want 1.0", out.Coverage)
	}
	if len(out.FailedKeywords) != 0 {
		t.Errorf("FailedKeywords = %v, want empty", out.FailedKeywords)
	}
}

// --- Error tolerance ---

func TestSearch_BM25ErrorIsTolerated(t *testing.T) {
	t.Parallel()
	// BM25 errors but FindSymbol succeeds -> keyword still counted as
	// hit, output non-empty.
	ckg := &ckgclient.Fake{
		BM25Err:         errors.New("bm25 down"),
		SymbolCitations: []contract.Citation{cit("x.go", 1, 1)},
	}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"k"}, contract.IntentBugFix)
	if len(out.FailedKeywords) != 0 {
		t.Errorf("FailedKeywords = %v, want empty (Symbol succeeded)", out.FailedKeywords)
	}
	if len(out.Citations) != 1 {
		t.Errorf("Citations count = %d, want 1", len(out.Citations))
	}
}

func TestSearch_SymbolErrorIsTolerated(t *testing.T) {
	t.Parallel()
	ckg := &ckgclient.Fake{
		BM25Hits:  []contract.Hit{bm25Hit("x.go", 1, 1, 2.0)},
		SymbolErr: errors.New("symbol index down"),
	}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"k"}, contract.IntentBugFix)
	if len(out.FailedKeywords) != 0 {
		t.Errorf("FailedKeywords = %v, want empty (BM25 succeeded)", out.FailedKeywords)
	}
	if out.Citations[0].Score != 2.0 {
		t.Errorf("Score = %v, want 2.0", out.Citations[0].Score)
	}
}

func TestSearch_BothErrorsKeywordFails(t *testing.T) {
	t.Parallel()
	ckg := &ckgclient.Fake{
		BM25Err:   errors.New("bm25 down"),
		SymbolErr: errors.New("symbol down"),
	}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"k"}, contract.IntentBugFix)
	if len(out.FailedKeywords) != 1 || out.FailedKeywords[0] != "k" {
		t.Errorf("FailedKeywords = %v, want [k]", out.FailedKeywords)
	}
	if out.Coverage != 0 {
		t.Errorf("Coverage = %v, want 0", out.Coverage)
	}
}

// --- Intent -> Kinds mapping ---

func TestSearch_PassesKindsFromIntent(t *testing.T) {
	t.Parallel()
	ckg := &ckgclient.Fake{
		SymbolCitations: []contract.Citation{cit("x.go", 1, 1)},
	}
	s, _ := New(ckg)

	_, _ = s.Search(context.Background(), []string{"X"}, contract.IntentArchExplain)

	if len(ckg.Calls.FindSymbol) != 1 {
		t.Fatalf("FindSymbol called %d times, want 1", len(ckg.Calls.FindSymbol))
	}
	gotKinds := ckg.Calls.FindSymbol[0].Opts.Kinds
	wantKinds := intentToKinds(contract.IntentArchExplain)
	if len(gotKinds) != len(wantKinds) {
		t.Fatalf("Kinds count = %d, want %d", len(gotKinds), len(wantKinds))
	}
	for i, k := range wantKinds {
		if gotKinds[i] != k {
			t.Errorf("Kinds[%d] = %q, want %q", i, gotKinds[i], k)
		}
	}
}

func TestIntentToKinds(t *testing.T) {
	t.Parallel()
	cases := map[contract.Intent][]string{
		contract.IntentBugFix:            {"function", "method"},
		contract.IntentFeatureAdd:        {"function", "method", "type", "interface"},
		contract.IntentArchExplain:       {"type", "interface", "const"},
		contract.IntentTestAdd:           {"function", "method"},
		contract.IntentConcurrencySafety: {"function", "method"},
		contract.IntentSecurity:          {"function", "method", "interface"},
		contract.IntentDocsUpdate:        {"type", "interface", "function", "method"},
		contract.IntentRefactor:          nil,
		contract.IntentQAReview:          nil,
		contract.IntentUnknown:           nil,
	}
	for in, want := range cases {
		t.Run(string(in), func(t *testing.T) {
			t.Parallel()
			got := intentToKinds(in)
			if len(got) != len(want) {
				t.Errorf("got %v, want %v", got, want)
				return
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

// --- Footprint ---

func TestSearch_EmitsFootprintEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{bm25Hit("x.go", 1, 1, 3.0)},
	}
	s, _ := New(ckg, WithFootprint(fp))

	_, _ = s.Search(context.Background(), []string{"Login"}, contract.IntentBugFix)
	_ = fp.Sync()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if rec["event"] != "composer.stage2_searched" {
		t.Errorf("event = %v", rec["event"])
	}
	if rec["intent"] != "bug_fix" {
		t.Errorf("intent = %v", rec["intent"])
	}
	if rec["coverage"].(float64) != 1.0 {
		t.Errorf("coverage = %v", rec["coverage"])
	}
}

// --- Aggregator unit tests ---

func TestAggregator_DedupsByCitationKey(t *testing.T) {
	t.Parallel()
	a := newAggregator(5.0)
	a.addBM25Hit("k1", bm25Hit("x.go", 1, 10, 3.0))
	a.addBM25Hit("k2", bm25Hit("x.go", 1, 10, 4.0))
	a.addSymbolHit("k3", cit("x.go", 1, 10))

	out := a.results(0)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1 (dedup)", len(out))
	}
	if out[0].Score != 3.0+4.0+5.0 {
		t.Errorf("Score = %v, want %v", out[0].Score, 3.0+4.0+5.0)
	}
	if len(out[0].Sources) != 3 {
		t.Errorf("Sources length = %d, want 3", len(out[0].Sources))
	}
}

func TestAggregator_EmptyResultsNil(t *testing.T) {
	t.Parallel()
	a := newAggregator(5.0)
	if got := a.results(10); got != nil {
		t.Errorf("results on empty aggregator = %v, want nil", got)
	}
}
