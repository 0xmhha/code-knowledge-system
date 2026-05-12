package stage1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- helpers ---

func hit(file string, rank int, score float64, source contract.HitSource) contract.Hit {
	return contract.Hit{
		Citation: contract.Citation{File: file, StartLine: 1, EndLine: 10, CommitHash: "abc"},
		Rank:     rank,
		Score:    score,
		Source:   source,
	}
}

// --- New / construction ---

func TestNew_NilCKVErrors(t *testing.T) {
	t.Parallel()
	_, err := New(nil, &ckgclient.Fake{})
	if err == nil {
		t.Fatal("expected error for nil ckv")
	}
}

func TestNew_NilCKGErrors(t *testing.T) {
	t.Parallel()
	_, err := New(&ckvclient.Fake{}, nil)
	if err == nil {
		t.Fatal("expected error for nil ckg")
	}
}

func TestNew_AppliesDefaultConfig(t *testing.T) {
	t.Parallel()
	e, err := New(&ckvclient.Fake{}, &ckgclient.Fake{})
	if err != nil {
		t.Fatal(err)
	}
	if e.config.MaxRounds != DefaultMaxRounds {
		t.Errorf("MaxRounds = %d, want %d", e.config.MaxRounds, DefaultMaxRounds)
	}
	if e.config.MinConfidence != DefaultMinConfidence {
		t.Errorf("MinConfidence = %v, want %v", e.config.MinConfidence, DefaultMinConfidence)
	}
}

func TestNew_ClampsMaxRoundsToAtLeastOne(t *testing.T) {
	t.Parallel()
	e, err := New(&ckvclient.Fake{}, &ckgclient.Fake{}, WithConfig(Config{MaxRounds: 0}))
	if err != nil {
		t.Fatal(err)
	}
	if e.config.MaxRounds < 1 {
		t.Fatalf("MaxRounds = %d, want >= 1", e.config.MaxRounds)
	}
}

// --- Extract / happy path ---

func TestExtract_EmptyPromptErrors(t *testing.T) {
	t.Parallel()
	e, _ := New(&ckvclient.Fake{}, &ckgclient.Fake{})
	_, err := e.Extract(context.Background(), "  \t\n", contract.IntentBugFix)
	if err == nil {
		t.Fatal("expected error for empty/whitespace prompt")
	}
}

func TestExtract_SingleRoundWithMaxRoundsOne(t *testing.T) {
	t.Parallel()
	// MaxRounds=1 forces the extractor to stop after a single recall
	// regardless of confidence. Used to verify the basic single-round
	// happy path (Hits, Keywords, footprint all populated).
	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{hit("handlers.go", 1, 0.9, contract.HitSourceCKV)},
	}
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{hit("handlers.go", 1, 10.0, contract.HitSourceCKG)},
	}
	e, _ := New(ckv, ckg, WithConfig(Config{
		MaxRounds:     1,
		InitialK:      DefaultInitialK,
		RerankPerKW:   DefaultRerankPerKW,
		MinConfidence: DefaultMinConfidence,
		MaxKeywords:   DefaultMaxKeywords,
		AugmentTopN:   DefaultAugmentTopN,
	}))

	out, err := e.Extract(context.Background(), "fix the Login handler", contract.IntentBugFix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if out.Rounds != 1 {
		t.Errorf("Rounds = %d, want 1 (MaxRounds=1)", out.Rounds)
	}
	if len(out.Keywords) == 0 {
		t.Fatal("Keywords empty; expected at least one")
	}
	if len(out.Hits) != 1 {
		t.Errorf("Hits = %d, want 1", len(out.Hits))
	}
	if out.Confidence <= 0 {
		t.Errorf("Confidence = %v, want > 0", out.Confidence)
	}
	if len(out.AugmentedQueries) != 1 {
		t.Errorf("AugmentedQueries = %d, want 1", len(out.AugmentedQueries))
	}
}

func TestExtract_LowConfidenceTriggersSecondRound(t *testing.T) {
	t.Parallel()
	// Set up: ckg gives every keyword the SAME small score, so
	// concentration = 1/N (low confidence). Extractor should run a
	// second ckv round.
	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{
			hit("alpha.go", 1, 0.5, contract.HitSourceCKV),
			hit("beta.go", 2, 0.5, contract.HitSourceCKV),
			hit("gamma.go", 3, 0.5, contract.HitSourceCKV),
			hit("delta.go", 4, 0.5, contract.HitSourceCKV),
			hit("epsilon.go", 5, 0.5, contract.HitSourceCKV),
		},
	}
	ckg := &ckgclient.Fake{
		// One score per candidate -> uniform -> low confidence.
		BM25Hits: []contract.Hit{hit("any.go", 1, 1.0, contract.HitSourceCKG)},
	}
	e, _ := New(ckv, ckg, WithConfig(Config{
		InitialK:      DefaultInitialK,
		RerankPerKW:   DefaultRerankPerKW,
		MaxRounds:     2,
		MinConfidence: 0.5, // need top-1 >= 50% of total to skip round 2
		MaxKeywords:   DefaultMaxKeywords,
		AugmentTopN:   3,
	}))

	out, err := e.Extract(context.Background(), "look at these files", contract.IntentArchExplain)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if out.Rounds != 2 {
		t.Errorf("Rounds = %d, want 2 (uniform scores => low confidence)", out.Rounds)
	}
	if len(out.AugmentedQueries) != 2 {
		t.Errorf("AugmentedQueries count = %d, want 2", len(out.AugmentedQueries))
	}
	if out.AugmentedQueries[0] != "look at these files" {
		t.Errorf("Round 1 query should be raw prompt, got %q", out.AugmentedQueries[0])
	}
	// Round 2 query should be prompt + " " + top reranked keywords.
	if !strings.HasPrefix(out.AugmentedQueries[1], "look at these files ") {
		t.Errorf("Round 2 query should prefix with prompt, got %q", out.AugmentedQueries[1])
	}
}

func TestExtract_CapsMaxRounds(t *testing.T) {
	t.Parallel()
	// Force always-low confidence; ensure we stop at MaxRounds=2.
	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{
			hit("a.go", 1, 0.5, contract.HitSourceCKV),
			hit("b.go", 2, 0.5, contract.HitSourceCKV),
		},
	}
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{hit("any.go", 1, 1.0, contract.HitSourceCKG)},
	}
	e, _ := New(ckv, ckg, WithConfig(Config{
		MaxRounds:     2,
		MinConfidence: 0.99, // very high so we always want to loop
		InitialK:      DefaultInitialK,
		RerankPerKW:   DefaultRerankPerKW,
		MaxKeywords:   DefaultMaxKeywords,
		AugmentTopN:   3,
	}))
	out, _ := e.Extract(context.Background(), "ambiguous prompt", contract.IntentBugFix)
	if out.Rounds != 2 {
		t.Errorf("Rounds = %d, want 2 (capped by MaxRounds)", out.Rounds)
	}
}

func TestExtract_CapsKeywordCount(t *testing.T) {
	t.Parallel()
	// 8 candidate files; cap output at 3 keywords.
	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{
			hit("f1.go", 1, 0.9, contract.HitSourceCKV),
			hit("f2.go", 2, 0.8, contract.HitSourceCKV),
			hit("f3.go", 3, 0.7, contract.HitSourceCKV),
			hit("f4.go", 4, 0.6, contract.HitSourceCKV),
			hit("f5.go", 5, 0.5, contract.HitSourceCKV),
			hit("f6.go", 6, 0.4, contract.HitSourceCKV),
			hit("f7.go", 7, 0.3, contract.HitSourceCKV),
			hit("f8.go", 8, 0.2, contract.HitSourceCKV),
		},
	}
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{hit("hit.go", 1, 1.0, contract.HitSourceCKG)},
	}
	e, _ := New(ckv, ckg, WithConfig(Config{
		MaxKeywords:   3,
		MaxRounds:     1,
		InitialK:      DefaultInitialK,
		RerankPerKW:   DefaultRerankPerKW,
		MinConfidence: 0.0, // accept anything in round 1
		AugmentTopN:   3,
	}))
	out, err := e.Extract(context.Background(), "find files", contract.IntentArchExplain)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Keywords) > 3 {
		t.Errorf("Keywords count = %d, want <= 3", len(out.Keywords))
	}
}

// --- Error paths ---

func TestExtract_CKVRound1FailureAborts(t *testing.T) {
	t.Parallel()
	ckv := &ckvclient.Fake{SearchErr: errors.New("ckv down")}
	ckg := &ckgclient.Fake{}
	e, _ := New(ckv, ckg)
	_, err := e.Extract(context.Background(), "anything", contract.IntentBugFix)
	if err == nil {
		t.Fatal("expected error when ckv fails on round 1")
	}
	if !strings.Contains(err.Error(), "ckv") {
		t.Errorf("error = %v, want 'ckv' context", err)
	}
}

func TestExtract_CKGAllFailReturnsEmptyKeywords(t *testing.T) {
	t.Parallel()
	// ckv works, but every ckg rerank call errors. The extractor should
	// degrade gracefully — return what we have (Hits) with empty
	// keywords and zero confidence, no error.
	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{hit("handlers.go", 1, 0.9, contract.HitSourceCKV)},
	}
	ckg := &ckgclient.Fake{BM25Err: errors.New("ckg down")}
	e, _ := New(ckv, ckg, WithConfig(Config{MaxRounds: 1, InitialK: 10, RerankPerKW: 5, MinConfidence: 0.5, MaxKeywords: 5, AugmentTopN: 3}))

	out, err := e.Extract(context.Background(), "fix Login", contract.IntentBugFix)
	if err != nil {
		t.Fatalf("Extract returned err: %v", err)
	}
	if len(out.Keywords) != 0 {
		t.Errorf("Keywords = %v, want empty (all reranks failed)", out.Keywords)
	}
	if out.Confidence != 0 {
		t.Errorf("Confidence = %v, want 0", out.Confidence)
	}
	if len(out.Hits) != 1 {
		t.Errorf("Hits = %d, want 1 (preserved from ckv)", len(out.Hits))
	}
}

// --- Footprint ---

func TestExtract_EmitsFootprintEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	ckv := &ckvclient.Fake{SearchHits: []contract.Hit{hit("h.go", 1, 1.0, contract.HitSourceCKV)}}
	ckg := &ckgclient.Fake{BM25Hits: []contract.Hit{hit("h.go", 1, 5.0, contract.HitSourceCKG)}}
	e, _ := New(ckv, ckg, WithFootprint(fp))

	_, _ = e.Extract(context.Background(), "fix Login", contract.IntentBugFix)
	_ = fp.Sync()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if rec["event"] != "composer.stage1_extracted" {
		t.Errorf("event = %v", rec["event"])
	}
	if rec["intent"] != "bug_fix" {
		t.Errorf("intent = %v", rec["intent"])
	}
	if _, ok := rec["keywords"].([]any); !ok {
		t.Errorf("keywords field missing or wrong type: %v", rec["keywords"])
	}
}

// --- Helpers / unit-level ---

func TestExtractIdentifiers_FindsCommonForms(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		// "fix" is not a stopword — it's a likely user verb that often
		// names code paths ("FixOrder"). BM25 rerank filters real noise.
		"fix Login handler":                {"fix", "Login", "handler"},
		"check ProcessRequest for race":    {"check", "ProcessRequest", "race"},
		"snake_case_var in module":         {"snake_case_var", "module"},
		"the and for but":                  nil, // all stopwords
		"PROCESS_REQUEST constant matters": {"PROCESS_REQUEST", "constant", "matters"},
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			got := extractIdentifiers(in)
			if len(got) != len(want) {
				t.Errorf("got %v, want %v", got, want)
				return
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestExtractKeywords_DedupsAcrossPromptAndHits(t *testing.T) {
	t.Parallel()
	prompt := "fix Login function"
	hits := []contract.Hit{
		hit("Login.go", 1, 1.0, contract.HitSourceCKV),
		hit("auth/Login.go", 2, 1.0, contract.HitSourceCKV), // same basename
		hit("session.go", 3, 1.0, contract.HitSourceCKV),
	}
	out := extractKeywords(prompt, hits, contract.IntentBugFix)
	// "Login" appears both in prompt and in two hit basenames — must
	// appear only once in output.
	count := 0
	for _, k := range out {
		if k == "Login" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Login appeared %d times, want 1", count)
	}
}

func TestExtractKeywords_StripsTestSuffix(t *testing.T) {
	t.Parallel()
	hits := []contract.Hit{hit("foo_test.go", 1, 1.0, contract.HitSourceCKV)}
	out := extractKeywords("", hits, contract.IntentTestAdd)
	want := "foo"
	found := false
	for _, k := range out {
		if k == want {
			found = true
		}
		if k == "foo_test" {
			t.Errorf("did not strip _test suffix from %q", k)
		}
	}
	if !found {
		t.Errorf("output %v does not contain %q", out, want)
	}
}

func TestComputeConfidence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		scored []scoredKeyword
		want   float64
	}{
		{"empty", nil, 0},
		{"single", []scoredKeyword{{Score: 5}}, 1.0},
		{"dominant", []scoredKeyword{{Score: 9}, {Score: 1}}, 0.9},
		{"uniform 4", []scoredKeyword{{Score: 1}, {Score: 1}, {Score: 1}, {Score: 1}}, 0.25},
		{"all zero", []scoredKeyword{{Score: 0}, {Score: 0}}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeConfidence(tc.scored)
			diff := got - tc.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 1e-9 {
				t.Errorf("computeConfidence(%v) = %v, want %v", tc.scored, got, tc.want)
			}
		})
	}
}

func TestMergeHits_Dedupes(t *testing.T) {
	t.Parallel()
	a := []contract.Hit{hit("a.go", 1, 1.0, contract.HitSourceCKV)}
	b := []contract.Hit{
		hit("a.go", 5, 0.5, contract.HitSourceCKV), // dup citation
		hit("b.go", 6, 0.5, contract.HitSourceCKV),
	}
	out := mergeHits(a, b)
	if len(out) != 2 {
		t.Errorf("merged length = %d, want 2", len(out))
	}
	// First hit (from a) wins on dup; rank should remain 1.
	if out[0].Rank != 1 {
		t.Errorf("dup-merge dropped original; rank = %d, want 1", out[0].Rank)
	}
}

func TestAugmentQuery(t *testing.T) {
	t.Parallel()
	got := augmentQuery("fix login", []string{"Login", "Session", "Auth", "User"}, 2)
	want := "fix login Login Session"
	if got != want {
		t.Errorf("augmentQuery = %q, want %q", got, want)
	}
}

func TestAugmentQuery_NoKeywordsReturnsPromptUnchanged(t *testing.T) {
	t.Parallel()
	got := augmentQuery("fix login", nil, 3)
	if got != "fix login" {
		t.Errorf("augmentQuery with empty reranked = %q, want unchanged", got)
	}
}
