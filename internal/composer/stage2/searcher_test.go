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
	if s.config.RRFK != DefaultRRFK {
		t.Errorf("RRFK = %d, want %d", s.config.RRFK, DefaultRRFK)
	}
	if s.config.BMWeight != DefaultBMWeight {
		t.Errorf("BMWeight = %v, want %v", s.config.BMWeight, DefaultBMWeight)
	}
	if s.config.SymbolWeight != DefaultSymbolWeight {
		t.Errorf("SymbolWeight = %v, want %v", s.config.SymbolWeight, DefaultSymbolWeight)
	}
}

// rrfContribution computes one RRF term so tests stay readable when the
// default constants move. weight / (k + rank).
func rrfContribution(weight float64, rank int) float64 {
	return weight / float64(DefaultRRFK+rank)
}

// almostEqual avoids floating-point exact-equality flakiness in tests.
func almostEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

// --- Intent-driven supplemental BM25 (IntentTestAdd) ---

func TestSearch_IntentTestAddTriggersSupplementalGlobPass(t *testing.T) {
	t.Parallel()
	// IntentTestAdd promises Stage 2 surfaces "target symbol plus
	// same-package *_test.go files". Verify the supplemental BM25
	// call runs with PathGlob="*_test.go".
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{bm25Hit("a.go", 1, 5, 0.9)},
	}
	s, _ := New(ckg)
	_, err := s.Search(context.Background(), []string{"Foo"}, nil, contract.IntentTestAdd)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(ckg.Calls.BM25Search); got != 2 {
		t.Fatalf("BM25Search calls = %d, want 2 (1 unfiltered + 1 *_test.go pass)", got)
	}
	// First call: unfiltered.
	if pg := ckg.Calls.BM25Search[0].Opts.Filter.PathGlob; pg != "" {
		t.Errorf("first call PathGlob = %q, want empty", pg)
	}
	// Second call: same keyword, *_test.go filter.
	if c := ckg.Calls.BM25Search[1]; c.Query != "Foo" || c.Opts.Filter.PathGlob != "*_test.go" {
		t.Errorf("supplemental call = %+v, want Query=Foo PathGlob=*_test.go", c)
	}
}

func TestSearch_NonTestAddIntentSkipsSupplementalPass(t *testing.T) {
	t.Parallel()
	// Every non-TestAdd intent stays on the single-pass path. Adding
	// a routing for another intent (e.g. DocsUpdate) is a deliberate
	// change; the supplemental pass must NOT fire by default.
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{bm25Hit("a.go", 1, 5, 0.9)},
	}
	s, _ := New(ckg)
	for _, intent := range []contract.Intent{
		contract.IntentUnknown,
		contract.IntentBugFix,
		contract.IntentArchExplain,
		contract.IntentRefactor,
		contract.IntentSecurity,
		contract.IntentConcurrencySafety,
		contract.IntentQAReview,
		contract.IntentDocsUpdate,
		contract.IntentFeatureAdd,
	} {
		ckg.Calls.Reset()
		_, _ = s.Search(context.Background(), []string{"Foo"}, nil, intent)
		if got := len(ckg.Calls.BM25Search); got != 1 {
			t.Errorf("intent=%s: BM25Search calls = %d, want 1", intent, got)
		}
	}
}

func TestSearch_IntentTestAddAggregatesBothPasses(t *testing.T) {
	t.Parallel()
	// The supplemental hit's score should add to the aggregator on
	// top of the unfiltered hit's score (double-count is the
	// intentional boost — that's how the *_test.go promise lands).
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{bm25Hit("foo_test.go", 10, 20, 0.5)},
	}
	s, _ := New(ckg)
	out, err := s.Search(context.Background(), []string{"Foo"}, nil, contract.IntentTestAdd)
	if err != nil {
		t.Fatal(err)
	}
	// Aggregator accumulated the hit twice (one per pass) as two
	// separate RRF-ranked lists. Each contributes BMWeight/(K+1).
	if len(out.Citations) != 1 {
		t.Fatalf("citations = %d, want 1", len(out.Citations))
	}
	want := 2 * rrfContribution(DefaultBMWeight, 1)
	if !almostEqual(out.Citations[0].Score, want) {
		t.Errorf("aggregated score = %.6f, want %.6f (two passes, rank 1 each)", out.Citations[0].Score, want)
	}
}

// --- Search / happy paths ---

func TestSearch_EmptyKeywordsReturnsEmpty(t *testing.T) {
	t.Parallel()
	s, _ := New(&ckgclient.Fake{})
	out, err := s.Search(context.Background(), nil, nil, contract.IntentBugFix)
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

	out, err := s.Search(context.Background(), []string{"Login"}, nil, contract.IntentBugFix)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Citations) != 1 {
		t.Fatalf("Citations count = %d, want 1", len(out.Citations))
	}
	want := rrfContribution(DefaultBMWeight, 1)
	if !almostEqual(out.Citations[0].Score, want) {
		t.Errorf("Score = %v, want %v (rank 1 BM25)", out.Citations[0].Score, want)
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

	out, err := s.Search(context.Background(), []string{"Login"}, nil, contract.IntentBugFix)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Citations) != 1 {
		t.Fatalf("Citations count = %d, want 1", len(out.Citations))
	}
	want := rrfContribution(DefaultSymbolWeight, 1)
	if !almostEqual(out.Citations[0].Score, want) {
		t.Errorf("Score = %v, want %v (rank 1 Symbol)", out.Citations[0].Score, want)
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

	out, _ := s.Search(context.Background(), []string{"Login"}, nil, contract.IntentBugFix)
	if len(out.Citations) != 1 {
		t.Fatalf("Citations count = %d, want 1 (dedup)", len(out.Citations))
	}
	want := rrfContribution(DefaultBMWeight, 1) + rrfContribution(DefaultSymbolWeight, 1)
	if !almostEqual(out.Citations[0].Score, want) {
		t.Errorf("Score = %v, want %v (rank 1 BM25 + rank 1 Symbol)", out.Citations[0].Score, want)
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

	out, _ := s.Search(context.Background(), []string{"alpha", "beta"}, nil, contract.IntentBugFix)
	if len(out.Citations) != 1 {
		t.Fatalf("Citations count = %d, want 1", len(out.Citations))
	}
	// Two keywords each pull the same citation at BM25 rank 1 of their
	// individual ranked list, so the RRF total is twice the rank-1 term.
	want := 2 * rrfContribution(DefaultBMWeight, 1)
	if !almostEqual(out.Citations[0].Score, want) {
		t.Errorf("Score = %v, want %v (two keywords, BM25 rank 1 each)", out.Citations[0].Score, want)
	}
	if len(out.Citations[0].Sources) != 2 {
		t.Errorf("Sources length = %d, want 2", len(out.Citations[0].Sources))
	}
}

func TestSearch_SortsByRankInRRF(t *testing.T) {
	t.Parallel()
	// RRF is rank-only: a citation's contribution is weight/(K+rank),
	// independent of the backend's native score. So the rank-1 entry
	// in the BM25 list always lands at the head of Stage2Output even
	// when its native score is the smallest.
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{
			bm25Hit("rank1.go", 1, 10, 0.1), // rank 1 — smallest native score
			bm25Hit("rank2.go", 1, 10, 9.9), // rank 2 — largest native score
			bm25Hit("rank3.go", 1, 10, 5.0), // rank 3
		},
	}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"k"}, nil, contract.IntentBugFix)
	if len(out.Citations) != 3 {
		t.Fatalf("Citations count = %d, want 3", len(out.Citations))
	}
	want := []string{"rank1.go", "rank2.go", "rank3.go"}
	for i, sc := range out.Citations {
		if sc.Citation.File != want[i] {
			t.Errorf("Citations[%d].File = %q, want %q", i, sc.Citation.File, want[i])
		}
	}
}

func TestAggregator_TiesBrokenByFileThenStartLine(t *testing.T) {
	t.Parallel()
	// Force an exact RRF tie by feeding each citation as a rank-1
	// hit on its own single-element list. All three end up with the
	// same total contribution, and the deterministic tiebreaker
	// (file path, then start line) decides the final order.
	a := newAggregator(DefaultRRFK, DefaultBMWeight, DefaultSymbolWeight, DefaultCkvWeight)
	a.addBM25List("k1", []contract.Hit{bm25Hit("b.go", 30, 40, 5.0)})
	a.addBM25List("k2", []contract.Hit{bm25Hit("a.go", 1, 10, 5.0)})
	a.addBM25List("k3", []contract.Hit{bm25Hit("b.go", 1, 10, 5.0)})

	out := a.results(0)
	want := []contract.Citation{
		cit("a.go", 1, 10),
		cit("b.go", 1, 10),
		cit("b.go", 30, 40),
	}
	for i, sc := range out {
		if sc.Citation != want[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, sc.Citation, want[i])
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
		RRFK:         DefaultRRFK,
		BMWeight:     DefaultBMWeight,
		SymbolWeight: DefaultSymbolWeight,
	}))

	out, _ := s.Search(context.Background(), []string{"k"}, nil, contract.IntentBugFix)
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

	out, _ := s.Search(context.Background(), []string{"a", "b", "c"}, nil, contract.IntentBugFix)
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
	out, _ := s.Search(context.Background(), []string{"a", "b"}, nil, contract.IntentBugFix)
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

	out, _ := s.Search(context.Background(), []string{"k"}, nil, contract.IntentBugFix)
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

	out, _ := s.Search(context.Background(), []string{"k"}, nil, contract.IntentBugFix)
	if len(out.FailedKeywords) != 0 {
		t.Errorf("FailedKeywords = %v, want empty (BM25 succeeded)", out.FailedKeywords)
	}
	want := rrfContribution(DefaultBMWeight, 1)
	if !almostEqual(out.Citations[0].Score, want) {
		t.Errorf("Score = %v, want %v (BM25 rank 1 only)", out.Citations[0].Score, want)
	}
}

func TestSearch_BothErrorsKeywordFails(t *testing.T) {
	t.Parallel()
	ckg := &ckgclient.Fake{
		BM25Err:   errors.New("bm25 down"),
		SymbolErr: errors.New("symbol down"),
	}
	s, _ := New(ckg)

	out, _ := s.Search(context.Background(), []string{"k"}, nil, contract.IntentBugFix)
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

	_, _ = s.Search(context.Background(), []string{"X"}, nil, contract.IntentArchExplain)

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
		contract.IntentArchExplain:       {"type", "interface", "const", "function", "method"},
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

	_, _ = s.Search(context.Background(), []string{"Login"}, nil, contract.IntentBugFix)
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
	// Error counters present even when all calls succeed (value 0).
	if _, ok := rec["bm25_errors"]; !ok {
		t.Error("footprint missing bm25_errors field")
	}
	if _, ok := rec["symbol_errors"]; !ok {
		t.Error("footprint missing symbol_errors field")
	}
}

func TestSearch_FootprintRecordsErrorCounts(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	// Both backends error -> bm25_errors == 1, symbol_errors == 1.
	ckg := &ckgclient.Fake{
		BM25Err:   errors.New("bm25 down"),
		SymbolErr: errors.New("symbol down"),
	}
	s, _ := New(ckg, WithFootprint(fp))
	_, _ = s.Search(context.Background(), []string{"X"}, nil, contract.IntentBugFix)
	_ = fp.Sync()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if got, _ := rec["bm25_errors"].(float64); got != 1 {
		t.Errorf("bm25_errors = %v, want 1", rec["bm25_errors"])
	}
	if got, _ := rec["symbol_errors"].(float64); got != 1 {
		t.Errorf("symbol_errors = %v, want 1", rec["symbol_errors"])
	}
}

// --- Aggregator unit tests ---

func TestAggregator_DedupsByCitationKey(t *testing.T) {
	t.Parallel()
	a := newAggregator(DefaultRRFK, DefaultBMWeight, DefaultSymbolWeight, DefaultCkvWeight)
	// Three single-element ranked lists, all rank 1, all on the same
	// citation. RRF total = 2 BM25 contributions + 1 Symbol contribution.
	a.addBM25List("k1", []contract.Hit{bm25Hit("x.go", 1, 10, 3.0)})
	a.addBM25List("k2", []contract.Hit{bm25Hit("x.go", 1, 10, 4.0)})
	a.addSymbolList("k3", []contract.Citation{cit("x.go", 1, 10)})

	out := a.results(0)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1 (dedup)", len(out))
	}
	want := 2*rrfContribution(DefaultBMWeight, 1) + rrfContribution(DefaultSymbolWeight, 1)
	if !almostEqual(out[0].Score, want) {
		t.Errorf("Score = %v, want %v", out[0].Score, want)
	}
	if len(out[0].Sources) != 3 {
		t.Errorf("Sources length = %d, want 3", len(out[0].Sources))
	}
}

func TestAggregator_RankAffectsContribution(t *testing.T) {
	t.Parallel()
	// A single BM25 list of length 3 should give the head citation
	// strictly more weight than the tail, regardless of native score.
	a := newAggregator(DefaultRRFK, DefaultBMWeight, DefaultSymbolWeight, DefaultCkvWeight)
	a.addBM25List("k", []contract.Hit{
		bm25Hit("a.go", 1, 1, 0.1), // rank 1
		bm25Hit("b.go", 1, 1, 0.9), // rank 2
		bm25Hit("c.go", 1, 1, 0.5), // rank 3
	})
	out := a.results(0)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	// Head wins despite its tiny native score — that is exactly the
	// property RRF promises and the old score-sum aggregator failed.
	if out[0].Citation.File != "a.go" {
		t.Errorf("head = %s, want a.go", out[0].Citation.File)
	}
	if !almostEqual(out[0].Score, rrfContribution(DefaultBMWeight, 1)) {
		t.Errorf("head score = %v, want rank-1 contribution", out[0].Score)
	}
	if !almostEqual(out[2].Score, rrfContribution(DefaultBMWeight, 3)) {
		t.Errorf("tail score = %v, want rank-3 contribution", out[2].Score)
	}
}

func TestAggregator_EmptyResultsNil(t *testing.T) {
	t.Parallel()
	a := newAggregator(DefaultRRFK, DefaultBMWeight, DefaultSymbolWeight, DefaultCkvWeight)
	if got := a.results(10); got != nil {
		t.Errorf("results on empty aggregator = %v, want nil", got)
	}
}
