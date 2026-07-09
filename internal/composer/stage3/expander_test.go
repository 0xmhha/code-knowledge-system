package stage3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- helpers ---

func cit(file string, start, end int) contract.Citation {
	return contract.Citation{File: file, StartLine: start, EndLine: end, CommitHash: "abc"}
}

func seed(file string, start, end int, score float64) stage2.ScoredCitation {
	return stage2.ScoredCitation{
		Citation: cit(file, start, end),
		Score:    score,
		Sources:  []string{"test"},
	}
}

func neighbor(srcFile, tgtFile string, rel contract.Relation, dist int) contract.Neighbor {
	return contract.Neighbor{
		Source:   cit(srcFile, 1, 10),
		Target:   cit(tgtFile, 100, 110),
		Relation: rel,
		Distance: dist,
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
	e, err := New(&ckgclient.Fake{})
	if err != nil {
		t.Fatal(err)
	}
	if e.config.MaxSeedsToExpand != DefaultMaxSeedsToExpand {
		t.Errorf("MaxSeedsToExpand = %d, want %d", e.config.MaxSeedsToExpand, DefaultMaxSeedsToExpand)
	}
}

// --- Expand / happy paths ---

func TestExpand_EmptySeedsReturnsEmpty(t *testing.T) {
	t.Parallel()
	e, _ := New(&ckgclient.Fake{})
	out, err := e.Expand(context.Background(), nil, contract.IntentBugFix)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(out.Seeds) != 0 || len(out.Neighbors) != 0 {
		t.Fatalf("expected empty output, got %+v", out)
	}
}

func TestExpand_PassesSeedsThrough(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{
		seed("a.go", 1, 10, 5.0),
		seed("b.go", 1, 10, 4.0),
	}
	e, _ := New(&ckgclient.Fake{})
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)
	if len(out.Seeds) != 2 {
		t.Fatalf("Seeds count = %d, want 2", len(out.Seeds))
	}
	if out.Seeds[0].Citation.File != "a.go" {
		t.Errorf("Seeds[0].File = %q, want a.go (passthrough order preserved)", out.Seeds[0].Citation.File)
	}
}

func TestExpand_AddsNeighbors(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("login.go", 10, 30, 10.0)}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{
			neighbor("login.go", "validator.go", contract.RelationCalls, 1),
			neighbor("login.go", "handler.go", contract.RelationCalledBy, 1),
		},
	}
	e, _ := New(ckg)
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)

	if len(out.Neighbors) != 2 {
		t.Fatalf("Neighbors count = %d, want 2", len(out.Neighbors))
	}
	// score = 10.0 / (1 + dist=1) = 5.0
	if out.Neighbors[0].Score != 5.0 {
		t.Errorf("score = %v, want 5.0", out.Neighbors[0].Score)
	}
}

func TestExpand_SkipsSelfLoopNeighbors(t *testing.T) {
	t.Parallel()
	// Seed = a.go; ckg returns a neighbor whose Target is also a.go.
	// Stage 3 must skip it (already in seeds).
	seeds := []stage2.ScoredCitation{seed("a.go", 1, 10, 10.0)}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{
			{Source: cit("a.go", 1, 10), Target: cit("a.go", 1, 10), Relation: contract.RelationCalls, Distance: 1},
			neighbor("a.go", "b.go", contract.RelationCalls, 1),
		},
	}
	e, _ := New(ckg)
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)
	if len(out.Neighbors) != 1 {
		t.Fatalf("Neighbors count = %d, want 1 (self-loop skipped)", len(out.Neighbors))
	}
	if out.Neighbors[0].Edge.Target.File != "b.go" {
		t.Errorf("kept self-loop instead of b.go: %+v", out.Neighbors[0])
	}
}

func TestExpand_DistanceDecaysScore(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("a.go", 1, 10, 10.0)}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{
			neighbor("a.go", "b.go", contract.RelationCalls, 1), // 10/2 = 5.0
			neighbor("a.go", "c.go", contract.RelationCalls, 2), // 10/3 = 3.33
			neighbor("a.go", "d.go", contract.RelationCalls, 3), // 10/4 = 2.5
		},
	}
	e, _ := New(ckg)
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)
	if len(out.Neighbors) != 3 {
		t.Fatalf("Neighbors count = %d, want 3", len(out.Neighbors))
	}
	if out.Neighbors[0].Edge.Target.File != "b.go" {
		t.Errorf("top neighbor = %q, want b.go (lowest distance)", out.Neighbors[0].Edge.Target.File)
	}
	if out.Neighbors[2].Edge.Target.File != "d.go" {
		t.Errorf("bottom neighbor = %q, want d.go (highest distance)", out.Neighbors[2].Edge.Target.File)
	}
}

func TestExpand_MultiPathMaxScore(t *testing.T) {
	t.Parallel()
	// Two seeds; ckg.Fake returns the same canned neighbor for both.
	// The neighbor's score should be max(seed1.score/2, seed2.score/2)
	// = the higher seed's discounted score.
	seeds := []stage2.ScoredCitation{
		seed("a.go", 1, 10, 10.0),
		seed("b.go", 1, 10, 4.0),
	}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{
			// Target is c.go; reached from both seeds.
			{Source: cit("seed", 1, 10), Target: cit("c.go", 1, 10), Relation: contract.RelationCalls, Distance: 1},
		},
	}
	e, _ := New(ckg)
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)
	if len(out.Neighbors) != 1 {
		t.Fatalf("Neighbors count = %d, want 1 (dedup)", len(out.Neighbors))
	}
	// max(10/2, 4/2) = 5.0
	if out.Neighbors[0].Score != 5.0 {
		t.Errorf("Score = %v, want 5.0 (max of seed scores)", out.Neighbors[0].Score)
	}
	// Both seed paths recorded.
	if len(out.Neighbors[0].Sources) != 2 {
		t.Errorf("Sources length = %d, want 2", len(out.Neighbors[0].Sources))
	}
}

func TestExpand_RespectsMaxSeedsToExpand(t *testing.T) {
	t.Parallel()
	// Give 5 seeds, cap expansion at 2.
	seeds := []stage2.ScoredCitation{
		seed("a.go", 1, 10, 5.0),
		seed("b.go", 1, 10, 4.0),
		seed("c.go", 1, 10, 3.0),
		seed("d.go", 1, 10, 2.0),
		seed("e.go", 1, 10, 1.0),
	}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{
			neighbor("seed", "x.go", contract.RelationCalls, 1),
		},
	}
	e, _ := New(ckg, WithConfig(Config{
		MaxSeedsToExpand:  2,
		NeighborsPerSeed:  DefaultNeighborsPerSeed,
		MaxFinalNeighbors: DefaultMaxFinalNeighbors,
	}))
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)
	if out.SeedsExpanded != 2 {
		t.Errorf("SeedsExpanded = %d, want 2", out.SeedsExpanded)
	}
	if len(out.Seeds) != 5 {
		t.Errorf("Seeds count = %d, want 5 (all passthrough)", len(out.Seeds))
	}
	// ckg.Neighbors runs once per direction group per expanded seed.
	// BugFix splits into [calls] + [called_by], so 2 seeds -> 4 calls.
	if len(ckg.Calls.Neighbors) != 4 {
		t.Errorf("ckg.Neighbors called %d times, want 4 (2 seeds x 2 direction groups)", len(ckg.Calls.Neighbors))
	}
}

func TestExpand_RespectsMaxFinalNeighbors(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("a.go", 1, 10, 5.0)}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{
			neighbor("a.go", "b.go", contract.RelationCalls, 1),
			neighbor("a.go", "c.go", contract.RelationCalls, 1),
			neighbor("a.go", "d.go", contract.RelationCalls, 1),
		},
	}
	e, _ := New(ckg, WithConfig(Config{
		MaxSeedsToExpand:  DefaultMaxSeedsToExpand,
		NeighborsPerSeed:  DefaultNeighborsPerSeed,
		MaxFinalNeighbors: 2,
	}))
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)
	if len(out.Neighbors) != 2 {
		t.Errorf("Neighbors count = %d, want 2 (capped)", len(out.Neighbors))
	}
}

// --- RelationCoverage ---

func TestExpand_TracksRelationCoverage(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("a.go", 1, 10, 5.0)}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{
			neighbor("a.go", "b.go", contract.RelationCalls, 1),
			neighbor("a.go", "c.go", contract.RelationCalls, 1),
			neighbor("a.go", "d.go", contract.RelationCalledBy, 1),
		},
	}
	e, _ := New(ckg)
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)
	if out.RelationCoverage[contract.RelationCalls] != 2 {
		t.Errorf("calls coverage = %d, want 2", out.RelationCoverage[contract.RelationCalls])
	}
	if out.RelationCoverage[contract.RelationCalledBy] != 1 {
		t.Errorf("called_by coverage = %d, want 1", out.RelationCoverage[contract.RelationCalledBy])
	}
}

// --- Error tolerance ---

func TestExpand_NeighborsErrorRecordsFailedSeed(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{
		seed("a.go", 1, 10, 5.0),
		seed("b.go", 1, 10, 4.0),
	}
	ckg := &ckgclient.Fake{
		NeighborErr: errors.New("ckg.Neighbors down"),
	}
	e, _ := New(ckg)
	out, _ := e.Expand(context.Background(), seeds, contract.IntentBugFix)
	if len(out.FailedSeeds) != 2 {
		t.Errorf("FailedSeeds count = %d, want 2", len(out.FailedSeeds))
	}
	if len(out.Neighbors) != 0 {
		t.Errorf("Neighbors count = %d, want 0", len(out.Neighbors))
	}
}

// --- Intent mapping ---

func TestIntentToRelations(t *testing.T) {
	t.Parallel()
	cases := map[contract.Intent]int{
		contract.IntentBugFix:            2,
		contract.IntentFeatureAdd:        3,
		contract.IntentRefactor:          4,
		contract.IntentArchExplain:       5,
		contract.IntentTestAdd:           3,
		contract.IntentConcurrencySafety: 3,
		contract.IntentSecurity:          3,
		contract.IntentDocsUpdate:        2,
		contract.IntentQAReview:          0, // nil
		contract.IntentUnknown:           0, // nil
	}
	for in, want := range cases {
		t.Run(string(in), func(t *testing.T) {
			t.Parallel()
			got := intentToRelations(in)
			if len(got) != want {
				t.Errorf("intentToRelations(%v) = %v (len %d), want len %d", in, got, len(got), want)
			}
		})
	}
}

func TestIntentToHops(t *testing.T) {
	t.Parallel()
	traces := []contract.Intent{
		contract.IntentBugFix,
		contract.IntentRefactor,
		contract.IntentArchExplain,
		contract.IntentConcurrencySafety,
		contract.IntentSecurity,
	}
	for _, i := range traces {
		if got := intentToHops(i); got != 2 {
			t.Errorf("intentToHops(%v) = %d, want 2 (trace intent)", i, got)
		}
	}
	surface := []contract.Intent{
		contract.IntentFeatureAdd,
		contract.IntentTestAdd,
		contract.IntentDocsUpdate,
		contract.IntentQAReview,
		contract.IntentUnknown,
	}
	for _, i := range surface {
		if got := intentToHops(i); got != 1 {
			t.Errorf("intentToHops(%v) = %d, want 1 (surface intent)", i, got)
		}
	}
}

func TestExpand_PassesIntentRelationsToCKG(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("a.go", 1, 10, 5.0)}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{neighbor("a.go", "b.go", contract.RelationCalls, 1)},
	}
	e, _ := New(ckg)
	_, _ = e.Expand(context.Background(), seeds, contract.IntentBugFix)
	// BugFix's relation set mixes directions, so the expander issues one
	// direction-homogeneous call per group (ckgclient rejects mixed sets).
	if len(ckg.Calls.Neighbors) != 2 {
		t.Fatalf("ckg.Neighbors called %d times, want 2 direction groups", len(ckg.Calls.Neighbors))
	}
	var gotRels []contract.Relation
	for _, call := range ckg.Calls.Neighbors {
		hasFwd, hasRev := false, false
		for _, r := range call.Opts.Relations {
			if r == contract.RelationCalledBy {
				hasRev = true
			} else {
				hasFwd = true
			}
		}
		if hasFwd && hasRev {
			t.Fatalf("mixed-direction call leaked: %v", call.Opts.Relations)
		}
		gotRels = append(gotRels, call.Opts.Relations...)
	}
	opts := ckgclient.NeighborsOpts{Relations: gotRels, Hops: ckg.Calls.Neighbors[0].Opts.Hops}
	wantRels := intentToRelations(contract.IntentBugFix)
	if len(opts.Relations) != len(wantRels) {
		t.Errorf("Relations passed = %v, want %v", opts.Relations, wantRels)
	}
	if opts.Hops != intentToHops(contract.IntentBugFix) {
		t.Errorf("Hops = %d, want %d", opts.Hops, intentToHops(contract.IntentBugFix))
	}
}

// --- Footprint ---

func TestExpand_EmitsFootprintEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	seeds := []stage2.ScoredCitation{seed("a.go", 1, 10, 5.0)}
	ckg := &ckgclient.Fake{
		NeighborEdges: []contract.Neighbor{neighbor("a.go", "b.go", contract.RelationCalls, 1)},
	}
	e, _ := New(ckg, WithFootprint(fp))
	_, _ = e.Expand(context.Background(), seeds, contract.IntentBugFix)
	_ = fp.Sync()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if rec["event"] != "composer.stage3_expanded" {
		t.Errorf("event = %v", rec["event"])
	}
	if rec["intent"] != "bug_fix" {
		t.Errorf("intent = %v", rec["intent"])
	}
	if rec["neighbor_count"].(float64) != 1 {
		t.Errorf("neighbor_count = %v, want 1", rec["neighbor_count"])
	}
}

// --- Aggregator unit tests ---

func TestAggregator_DedupsByTargetMax(t *testing.T) {
	t.Parallel()
	a := newNeighborAggregator()
	a.add(neighbor("seed1", "x.go", contract.RelationCalls, 1), 3.0, "s1")
	a.add(neighbor("seed2", "x.go", contract.RelationCalls, 1), 5.0, "s2")
	a.add(neighbor("seed3", "x.go", contract.RelationCalls, 2), 2.0, "s3")

	out := a.results(0)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1 (dedup)", len(out))
	}
	if out[0].Score != 5.0 {
		t.Errorf("Score = %v, want 5.0 (max)", out[0].Score)
	}
	if len(out[0].Sources) != 3 {
		t.Errorf("Sources len = %d, want 3", len(out[0].Sources))
	}
}

func TestAggregator_EmptyResultsNil(t *testing.T) {
	t.Parallel()
	a := newNeighborAggregator()
	if got := a.results(10); got != nil {
		t.Errorf("empty results = %v, want nil", got)
	}
}
