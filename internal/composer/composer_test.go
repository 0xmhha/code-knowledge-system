package composer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/composer/budget"
	"github.com/0xmhha/code-knowledge-system/internal/composer/intent"
	"github.com/0xmhha/code-knowledge-system/internal/composer/sanitize"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage1"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/internal/config"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- test wiring helper ---

type fixture struct {
	ckv      *ckvclient.Fake
	ckg      *ckgclient.Fake
	embedder *intent.FakeEmbedder
	fetcher  *budget.FakeFetcher
	ruleset  *config.SanitizeRuleset
	composer *Composer
}

// newFixture wires every stage with the fakes configured from the
// passed-in setup callback. Tests use this to declare their interesting
// state (canned hits, bodies, embedder vectors, ruleset) before
// constructing the composer.
func newFixture(t *testing.T, setup func(f *fixture)) *fixture {
	t.Helper()
	f := &fixture{
		ckv:      &ckvclient.Fake{},
		ckg:      &ckgclient.Fake{},
		embedder: &intent.FakeEmbedder{Dim: 16},
		fetcher:  &budget.FakeFetcher{Bodies: map[string]string{}},
	}
	// Default empty ruleset so most tests don't trip sanitize rules.
	// Caller can override via setup.
	f.ruleset = mustRuleset(t,
		config.SanitizeRule{ID: "NOOP", Pattern: `__no_match__`, Action: contract.RedactionDrop, Severity: config.SeverityLow},
	)
	if setup != nil {
		setup(f)
	}

	intentClassifier, err := intent.New(context.Background(), f.embedder)
	if err != nil {
		t.Fatalf("intent.New: %v", err)
	}
	s1, err := stage1.New(f.ckv, f.ckg)
	if err != nil {
		t.Fatalf("stage1.New: %v", err)
	}
	s2, err := stage2.New(f.ckg)
	if err != nil {
		t.Fatalf("stage2.New: %v", err)
	}
	s3, err := stage3.New(f.ckg)
	if err != nil {
		t.Fatalf("stage3.New: %v", err)
	}
	b, err := budget.New(f.fetcher)
	if err != nil {
		t.Fatalf("budget.New: %v", err)
	}
	san, err := sanitize.New(f.ruleset)
	if err != nil {
		t.Fatalf("sanitize.New: %v", err)
	}
	c, err := New(intentClassifier, s1, s2, s3, b, san)
	if err != nil {
		t.Fatalf("composer.New: %v", err)
	}
	f.composer = c
	return f
}

func mustRuleset(t *testing.T, rules ...config.SanitizeRule) *config.SanitizeRuleset {
	t.Helper()
	rs := &config.SanitizeRuleset{Version: 1, Rules: rules}
	if err := rs.Validate(); err != nil {
		t.Fatalf("ruleset.Validate: %v", err)
	}
	return rs
}

func cit(file string, start, end int) contract.Citation {
	return contract.Citation{File: file, StartLine: start, EndLine: end, CommitHash: "abc"}
}

func hit(file string, start, end int, score float64, src contract.HitSource) contract.Hit {
	return contract.Hit{Citation: cit(file, start, end), Rank: 1, Score: score, Source: src}
}

// --- New / construction ---

func TestNew_RejectsNilStages(t *testing.T) {
	t.Parallel()
	embedder := &intent.FakeEmbedder{Dim: 16}
	ic, _ := intent.New(context.Background(), embedder)
	ckv := &ckvclient.Fake{}
	ckg := &ckgclient.Fake{}
	s1, _ := stage1.New(ckv, ckg)
	s2, _ := stage2.New(ckg)
	s3, _ := stage3.New(ckg)
	b, _ := budget.New(&budget.FakeFetcher{})
	rs := mustRuleset(t, config.SanitizeRule{ID: "X", Pattern: "x", Action: contract.RedactionDrop, Severity: config.SeverityLow})
	san, _ := sanitize.New(rs)

	cases := []struct {
		name string
		f    func() (*Composer, error)
	}{
		{"nil intent", func() (*Composer, error) { return New(nil, s1, s2, s3, b, san) }},
		{"nil stage1", func() (*Composer, error) { return New(ic, nil, s2, s3, b, san) }},
		{"nil stage2", func() (*Composer, error) { return New(ic, s1, nil, s3, b, san) }},
		{"nil stage3", func() (*Composer, error) { return New(ic, s1, s2, nil, b, san) }},
		{"nil budget", func() (*Composer, error) { return New(ic, s1, s2, s3, nil, san) }},
		{"nil sanitize", func() (*Composer, error) { return New(ic, s1, s2, s3, b, nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tc.f(); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestNew_DefaultBuilderVersion(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	if f.composer.builderVersion != DefaultBuilderVersion {
		t.Errorf("builderVersion = %q, want %q", f.composer.builderVersion, DefaultBuilderVersion)
	}
}

// --- Compose / happy paths ---

func TestCompose_EmptyPromptErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	_, err := f.composer.Compose(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestCompose_FullPipelinePopulatesPack(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.SearchHits = []contract.Hit{hit("login.go", 10, 30, 0.9, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("login.go", 10, 30, 8.0, contract.HitSourceCKG)}
		f.ckg.NeighborEdges = []contract.Neighbor{
			{Source: cit("login.go", 10, 30), Target: cit("auth.go", 5, 25), Relation: contract.RelationCalls, Distance: 1},
		}
		// Body fetched by both citations the pipeline ends up with.
		f.fetcher.Bodies = map[string]string{
			cit("login.go", 10, 30).Key(): "func Login() { return validate() }",
			cit("auth.go", 5, 25).Key():   "func validate() bool { return true }",
		}
	})

	pack, err := f.composer.Compose(context.Background(), "find the Login handler")
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	// Pack must pass its own validation (per the contract).
	if !pack.IsValid() {
		t.Errorf("pack.IsValid()=false; pack=%+v", pack)
	}
	if pack.Query != "find the Login handler" {
		t.Errorf("Query = %q, want input prompt", pack.Query)
	}
	if !pack.Intent.IsValid() {
		t.Errorf("Intent invalid: %q", pack.Intent)
	}
	if len(pack.Citations) == 0 {
		t.Error("Citations empty after successful pipeline")
	}
	if len(pack.Bodies) == 0 {
		t.Error("Bodies empty after successful pipeline")
	}
	if pack.Metadata.IntegrityHash == "" {
		t.Error("IntegrityHash not stamped")
	}
	if pack.Metadata.BuilderVersion == "" {
		t.Error("BuilderVersion empty")
	}
}

func TestComposeTraced_ProducesTrace(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.SearchHits = []contract.Hit{hit("login.go", 10, 30, 0.9, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("login.go", 10, 30, 8.0, contract.HitSourceCKG)}
		f.fetcher.Bodies = map[string]string{
			cit("login.go", 10, 30).Key(): "func Login() { return validate() }",
		}
	})

	pack, trace, err := f.composer.ComposeTraced(context.Background(), "find the Login handler")
	if err != nil {
		t.Fatalf("ComposeTraced: %v", err)
	}
	if !pack.IsValid() {
		t.Error("pack.IsValid()=false")
	}
	if !trace.IsValid() {
		t.Fatalf("trace.IsValid()=false; trace=%+v", trace)
	}
	if trace.Producer != "composer" {
		t.Errorf("Producer=%q, want composer", trace.Producer)
	}
	if trace.Prompt != "find the Login handler" {
		t.Errorf("Prompt=%q", trace.Prompt)
	}
	if trace.Rounds < 1 || trace.CKVCalls != trace.Rounds {
		t.Errorf("Rounds=%d CKVCalls=%d (want CKVCalls==Rounds>=1)", trace.Rounds, trace.CKVCalls)
	}
	// First step is a ckv recall; the trailing step is the ckg seed search.
	if trace.Steps[0].Kind != contract.StepCKVRecall {
		t.Errorf("first step kind=%q, want %q", trace.Steps[0].Kind, contract.StepCKVRecall)
	}
	if last := trace.Steps[len(trace.Steps)-1]; last.Kind != contract.StepCKGBM25 {
		t.Errorf("last step kind=%q, want %q", last.Kind, contract.StepCKGBM25)
	}
	if len(trace.FinalSeeds) == 0 {
		t.Error("FinalSeeds empty after successful pipeline")
	}

	// Compose delegates to ComposeTraced — same pack, trace discarded.
	pack2, err := f.composer.Compose(context.Background(), "find the Login handler")
	if err != nil {
		t.Fatalf("Compose (delegation): %v", err)
	}
	if pack2.Query != pack.Query {
		t.Errorf("Compose vs ComposeTraced Query mismatch: %q vs %q", pack2.Query, pack.Query)
	}
}

func TestCompose_IntegrityHashVerifies(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.SearchHits = []contract.Hit{hit("a.go", 1, 10, 0.9, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("a.go", 1, 10, 8.0, contract.HitSourceCKG)}
		f.fetcher.Bodies = map[string]string{cit("a.go", 1, 10).Key(): "func F() {}"}
	})
	pack, err := f.composer.Compose(context.Background(), "find F")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := contract.VerifyIntegrity(pack)
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	if !ok {
		t.Fatal("stamped pack failed VerifyIntegrity")
	}
}

func TestCompose_IntentDegradeOnClassifierError(t *testing.T) {
	t.Parallel()
	// FakeEmbedder.Err makes Classify fail. Composer should not abort
	// — IntentUnknown is the safe fallback.
	//
	// Set Err AFTER newFixture so the anchor pre-embedding inside
	// intent.New succeeds; we only want to break the Classify call
	// the composer makes during Compose.
	f := newFixture(t, func(f *fixture) {
		f.ckv.SearchHits = []contract.Hit{hit("a.go", 1, 10, 0.5, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("a.go", 1, 10, 1.0, contract.HitSourceCKG)}
		f.fetcher.Bodies = map[string]string{cit("a.go", 1, 10).Key(): "body"}
	})
	f.embedder.Err = errors.New("embedder down")

	pack, err := f.composer.Compose(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Compose should not error on classifier failure: %v", err)
	}
	if pack.Intent != contract.IntentUnknown {
		t.Errorf("Intent = %q, want IntentUnknown after degrade", pack.Intent)
	}
}

func TestCompose_NeighborsFilteredToFinalCitations(t *testing.T) {
	t.Parallel()
	// Stage 3 emits a neighbor whose target (orphan.go) is NOT in the
	// budget/sanitize output. Composer must filter it out so the pack
	// passes IsValid.
	f := newFixture(t, func(f *fixture) {
		f.ckv.SearchHits = []contract.Hit{hit("seed.go", 1, 10, 0.5, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("seed.go", 1, 10, 5.0, contract.HitSourceCKG)}
		f.ckg.NeighborEdges = []contract.Neighbor{
			// Target is "orphan.go" but fetcher returns "" for it.
			{Source: cit("seed.go", 1, 10), Target: cit("orphan.go", 1, 10), Relation: contract.RelationCalls, Distance: 1},
		}
		// Only seed.go has a body; orphan.go fetch returns empty -> dropped.
		f.fetcher.Bodies = map[string]string{cit("seed.go", 1, 10).Key(): "body"}
	})

	pack, err := f.composer.Compose(context.Background(), "find seed")
	if err != nil {
		t.Fatal(err)
	}
	if !pack.IsValid() {
		t.Fatal("pack invalid; neighbors not filtered correctly")
	}
	for _, n := range pack.GraphNeighbors {
		if n.Target.File == "orphan.go" {
			t.Errorf("orphan neighbor leaked: %+v", n)
		}
	}
}

// --- Sanitize fail_closed ---

func TestCompose_FailClosedReturnsErrFailClosed(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		// Replace ruleset with one that fail-closes on "PRIVATE KEY".
		f.ruleset = mustRuleset(t,
			config.SanitizeRule{ID: "PK", Pattern: `PRIVATE KEY`, Action: contract.RedactionFailClosed, Severity: config.SeverityCritical},
		)
		f.ckv.SearchHits = []contract.Hit{hit("a.go", 1, 10, 0.5, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("a.go", 1, 10, 5.0, contract.HitSourceCKG)}
		// Body matches the fail-closed pattern.
		f.fetcher.Bodies = map[string]string{cit("a.go", 1, 10).Key(): "-----BEGIN PRIVATE KEY-----"}
	})

	_, err := f.composer.Compose(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected fail_closed error")
	}
	if !errors.Is(err, ErrFailClosed) {
		t.Errorf("err = %v, want ErrFailClosed wrap", err)
	}
	if !strings.Contains(err.Error(), "rule=PK") {
		t.Errorf("err missing rule id: %v", err)
	}
}

// --- Sanitize drop filters bodies out ---

func TestCompose_DroppedBodiesExcluded(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ruleset = mustRuleset(t,
			config.SanitizeRule{ID: "DROP", Pattern: `dropme`, Action: contract.RedactionDrop, Severity: config.SeverityHigh},
		)
		f.ckv.SearchHits = []contract.Hit{
			hit("safe.go", 1, 10, 0.9, contract.HitSourceCKV),
			hit("bad.go", 1, 10, 0.8, contract.HitSourceCKV),
		}
		f.ckg.BM25Hits = []contract.Hit{
			hit("safe.go", 1, 10, 9.0, contract.HitSourceCKG),
			hit("bad.go", 1, 10, 8.0, contract.HitSourceCKG),
		}
		f.fetcher.Bodies = map[string]string{
			cit("safe.go", 1, 10).Key(): "clean body",
			cit("bad.go", 1, 10).Key():  "contains dropme keyword",
		}
	})

	pack, err := f.composer.Compose(context.Background(), "search")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range pack.Citations {
		if c.File == "bad.go" {
			t.Errorf("dropped citation leaked into pack: %+v", c)
		}
	}
	for _, b := range pack.Bodies {
		if b.Citation.File == "bad.go" {
			t.Errorf("dropped body leaked into pack: %+v", b)
		}
	}
	if len(pack.SanitizeReport) == 0 {
		t.Error("SanitizeReport empty; drop should record a redaction")
	}
}

// --- Empty / no-result paths ---

func TestCompose_EmptyStage1ProducesEmptyButValidPack(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil) // ckv/ckg empty by default
	pack, err := f.composer.Compose(context.Background(), "prompt with no matches")
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if !pack.IsValid() {
		t.Errorf("empty pack should still be valid: %+v", pack)
	}
	if pack.Metadata.IntegrityHash == "" {
		t.Error("empty pack should still be stamped")
	}
	if len(pack.Citations) != 0 || len(pack.Bodies) != 0 {
		t.Errorf("expected empty citations/bodies, got %d/%d", len(pack.Citations), len(pack.Bodies))
	}
}

// --- Footprint ---

func TestCompose_EmitsComposeCompleteEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	f := newFixture(t, func(f *fixture) {
		f.ckv.SearchHits = []contract.Hit{hit("a.go", 1, 10, 0.5, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("a.go", 1, 10, 5.0, contract.HitSourceCKG)}
		f.fetcher.Bodies = map[string]string{cit("a.go", 1, 10).Key(): "body"}
	})
	// Re-wire composer with footprint attached.
	intentClassifier, _ := intent.New(context.Background(), f.embedder)
	s1, _ := stage1.New(f.ckv, f.ckg)
	s2, _ := stage2.New(f.ckg)
	s3, _ := stage3.New(f.ckg)
	b, _ := budget.New(f.fetcher)
	san, _ := sanitize.New(f.ruleset)
	c, _ := New(intentClassifier, s1, s2, s3, b, san, WithFootprint(fp))

	_, _ = c.Compose(context.Background(), "find a")
	_ = fp.Sync()

	// Scan footprint buffer for the compose_complete event (other
	// stages also emit events; we need to pick the right line).
	var found map[string]any
	for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec["event"] == "composer.compose_complete" {
			found = rec
			break
		}
	}
	if found == nil {
		t.Fatal("composer.compose_complete event not emitted")
	}
	for _, k := range []string{"intent", "prompt_runes", "citation_count", "body_count", "neighbor_count", "redaction_count", "used_tokens", "budget_tokens", "utilization", "dropped_count", "budget_skipped", "elapsed"} {
		if _, ok := found[k]; !ok {
			t.Errorf("compose_complete missing field %q", k)
		}
	}
}

// --- BuilderVersion option ---

func TestCompose_BuilderVersionOverride(t *testing.T) {
	t.Parallel()
	embedder := &intent.FakeEmbedder{Dim: 16}
	ic, _ := intent.New(context.Background(), embedder)
	ckv := &ckvclient.Fake{}
	ckg := &ckgclient.Fake{}
	s1, _ := stage1.New(ckv, ckg)
	s2, _ := stage2.New(ckg)
	s3, _ := stage3.New(ckg)
	b, _ := budget.New(&budget.FakeFetcher{})
	rs := mustRuleset(t, config.SanitizeRule{ID: "X", Pattern: "x", Action: contract.RedactionDrop, Severity: config.SeverityLow})
	san, _ := sanitize.New(rs)

	c, err := New(ic, s1, s2, s3, b, san, WithBuilderVersion("cks-test/1.2.3"))
	if err != nil {
		t.Fatal(err)
	}
	pack, err := c.Compose(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Metadata.BuilderVersion != "cks-test/1.2.3" {
		t.Errorf("BuilderVersion = %q, want override", pack.Metadata.BuilderVersion)
	}
}
