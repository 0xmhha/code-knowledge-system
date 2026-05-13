package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- helpers ---

func cit(file string, start, end int) contract.Citation {
	return contract.Citation{File: file, StartLine: start, EndLine: end, CommitHash: "abc"}
}

func seed(file string, score float64) stage2.ScoredCitation {
	return stage2.ScoredCitation{
		Citation: cit(file, 1, 10),
		Score:    score,
		Sources:  []string{"bm25:" + file + "=" + "score"},
	}
}

func neighbor(src, tgt string, rel contract.Relation, dist int, score float64) stage3.ScoredNeighbor {
	return stage3.ScoredNeighbor{
		Edge: contract.Neighbor{
			Source:   cit(src, 1, 10),
			Target:   cit(tgt, 100, 110),
			Relation: rel,
			Distance: dist,
		},
		Score:   score,
		Sources: []string{"seed:" + src + ":" + string(rel)},
	}
}

func bodyN(n int) string {
	// Each rune ~= 4 chars per token, so N chars ~= N/4 tokens.
	return strings.Repeat("a", n)
}

// --- New / construction ---

func TestNew_NilFetcherErrors(t *testing.T) {
	t.Parallel()
	_, err := New(nil)
	if err == nil {
		t.Fatal("expected error for nil fetcher")
	}
}

func TestNew_AppliesDefaults(t *testing.T) {
	t.Parallel()
	a, err := New(&FakeFetcher{})
	if err != nil {
		t.Fatal(err)
	}
	if a.config.MaxTokens != DefaultMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", a.config.MaxTokens, DefaultMaxTokens)
	}
	if a.config.OverheadReserve != DefaultOverheadReserve {
		t.Errorf("OverheadReserve = %v, want %v", a.config.OverheadReserve, DefaultOverheadReserve)
	}
}

func TestNew_RejectsNegativeMaxTokens(t *testing.T) {
	t.Parallel()
	_, err := New(&FakeFetcher{}, WithConfig(Config{MaxTokens: -1, OverheadReserve: 0.1}))
	if err == nil {
		t.Fatal("expected error for negative MaxTokens")
	}
}

func TestNew_RejectsOutOfRangeOverhead(t *testing.T) {
	t.Parallel()
	for _, v := range []float64{-0.1, 1.1, 2.0} {
		_, err := New(&FakeFetcher{}, WithConfig(Config{MaxTokens: 1000, OverheadReserve: v}))
		if err == nil {
			t.Errorf("expected error for OverheadReserve=%v", v)
		}
	}
}

// --- Allocate / happy paths ---

func TestAllocate_EmptyInputsReturnsEmpty(t *testing.T) {
	t.Parallel()
	a, _ := New(&FakeFetcher{})
	out, err := a.Allocate(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Selected) != 0 || len(out.Skipped) != 0 {
		t.Errorf("expected empty output, got %+v", out)
	}
	if out.BudgetTokens == 0 {
		t.Error("BudgetTokens = 0 even with defaults; expected ~7200")
	}
}

func TestAllocate_SeedsAndNeighborsMerged(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("a.go", 10.0)}
	neighbors := []stage3.ScoredNeighbor{neighbor("a.go", "b.go", contract.RelationCalls, 1, 5.0)}
	fetcher := &FakeFetcher{
		Bodies: map[string]string{
			cit("a.go", 1, 10).Key():    bodyN(40),
			cit("b.go", 100, 110).Key(): bodyN(40),
		},
	}
	a, _ := New(fetcher)
	out, err := a.Allocate(context.Background(), seeds, neighbors)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Selected) != 2 {
		t.Fatalf("Selected count = %d, want 2", len(out.Selected))
	}
	// Higher score first.
	if out.Selected[0].Origin != "seed" || out.Selected[0].Citation.File != "a.go" {
		t.Errorf("Selected[0] = %+v, want seed/a.go", out.Selected[0])
	}
	if out.Selected[1].Origin != "neighbor" || out.Selected[1].Citation.File != "b.go" {
		t.Errorf("Selected[1] = %+v, want neighbor/b.go", out.Selected[1])
	}
}

func TestAllocate_SortsByScoreDescending(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{
		seed("low.go", 1.0),
		seed("high.go", 9.0),
		seed("mid.go", 5.0),
	}
	fetcher := &FakeFetcher{
		Bodies: map[string]string{
			cit("low.go", 1, 10).Key():  bodyN(40),
			cit("high.go", 1, 10).Key(): bodyN(40),
			cit("mid.go", 1, 10).Key():  bodyN(40),
		},
	}
	a, _ := New(fetcher)
	out, _ := a.Allocate(context.Background(), seeds, nil)
	want := []string{"high.go", "mid.go", "low.go"}
	for i, item := range out.Selected {
		if item.Citation.File != want[i] {
			t.Errorf("Selected[%d].File = %q, want %q", i, item.Citation.File, want[i])
		}
	}
}

func TestAllocate_GreedyContinuesAfterSkipping(t *testing.T) {
	t.Parallel()
	// High-score seed has a huge body that won't fit; low-score seed has
	// a tiny body that will. Greedy should skip the big one and include
	// the small one — the loop must continue, not break.
	seeds := []stage2.ScoredCitation{
		seed("huge.go", 10.0),
		seed("tiny.go", 1.0),
	}
	fetcher := &FakeFetcher{
		Bodies: map[string]string{
			cit("huge.go", 1, 10).Key(): bodyN(1_000_000), // way over budget
			cit("tiny.go", 1, 10).Key(): bodyN(40),        // ~10 tokens
		},
	}
	a, _ := New(fetcher, WithConfig(Config{MaxTokens: 100, OverheadReserve: 0.10}))
	// body budget = 100 * 0.90 = 90 tokens

	out, _ := a.Allocate(context.Background(), seeds, nil)
	if len(out.Selected) != 1 {
		t.Fatalf("Selected count = %d, want 1", len(out.Selected))
	}
	if out.Selected[0].Citation.File != "tiny.go" {
		t.Errorf("Selected = %q, want tiny.go", out.Selected[0].Citation.File)
	}
	if len(out.Skipped) != 1 || out.Skipped[0].File != "huge.go" {
		t.Errorf("Skipped = %v, want [huge.go]", out.Skipped)
	}
}

func TestAllocate_BudgetTokensAccountsForOverhead(t *testing.T) {
	t.Parallel()
	a, _ := New(&FakeFetcher{}, WithConfig(Config{MaxTokens: 1000, OverheadReserve: 0.2}))
	out, _ := a.Allocate(context.Background(), nil, nil)
	if out.BudgetTokens != 800 { // 1000 * (1 - 0.2)
		t.Errorf("BudgetTokens = %d, want 800", out.BudgetTokens)
	}
}

func TestAllocate_UtilizationCalculation(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("a.go", 5.0)}
	fetcher := &FakeFetcher{
		Bodies: map[string]string{cit("a.go", 1, 10).Key(): bodyN(40)}, // 10 tokens
	}
	a, _ := New(fetcher, WithConfig(Config{MaxTokens: 100, OverheadReserve: 0.0})) // budget=100
	out, _ := a.Allocate(context.Background(), seeds, nil)
	if out.UsedTokens != 10 {
		t.Errorf("UsedTokens = %d, want 10", out.UsedTokens)
	}
	if out.Utilization != 0.10 {
		t.Errorf("Utilization = %v, want 0.10", out.Utilization)
	}
}

// --- Skip paths ---

func TestAllocate_EmptyBodySkipped(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("missing.go", 5.0)}
	// fetcher returns "" for unknown key
	a, _ := New(&FakeFetcher{})
	out, _ := a.Allocate(context.Background(), seeds, nil)
	if len(out.Selected) != 0 {
		t.Errorf("expected nothing selected, got %v", out.Selected)
	}
	if out.EmptyBodies != 1 {
		t.Errorf("EmptyBodies = %d, want 1", out.EmptyBodies)
	}
	if len(out.Skipped) != 1 {
		t.Errorf("Skipped count = %d, want 1", len(out.Skipped))
	}
}

func TestAllocate_FetchErrorSkipped(t *testing.T) {
	t.Parallel()
	seeds := []stage2.ScoredCitation{seed("a.go", 5.0)}
	a, _ := New(&FakeFetcher{Err: errors.New("disk failure")})
	out, _ := a.Allocate(context.Background(), seeds, nil)
	if out.FetchErrors != 1 {
		t.Errorf("FetchErrors = %d, want 1", out.FetchErrors)
	}
	if len(out.Skipped) != 1 {
		t.Errorf("Skipped count = %d, want 1", len(out.Skipped))
	}
	if len(out.Selected) != 0 {
		t.Errorf("expected nothing selected on fetcher error")
	}
}

func TestAllocate_DedupesAcrossSeedsAndNeighbors(t *testing.T) {
	t.Parallel()
	// Same citation in both seeds and neighbors — defensive dedup
	// should keep it only once (the seed version wins by stable sort
	// because it's appended first at equal score).
	c := cit("dup.go", 1, 10)
	seeds := []stage2.ScoredCitation{
		{Citation: c, Score: 5.0, Sources: []string{"seed-src"}},
	}
	neighbors := []stage3.ScoredNeighbor{
		{
			Edge:    contract.Neighbor{Source: cit("x.go", 1, 1), Target: c, Relation: contract.RelationCalls, Distance: 1},
			Score:   5.0, // same score
			Sources: []string{"neighbor-src"},
		},
	}
	fetcher := &FakeFetcher{Bodies: map[string]string{c.Key(): bodyN(40)}}
	a, _ := New(fetcher)
	out, _ := a.Allocate(context.Background(), seeds, neighbors)
	if len(out.Selected) != 1 {
		t.Fatalf("Selected count = %d, want 1 (dedup)", len(out.Selected))
	}
	if out.Selected[0].Origin != "seed" {
		t.Errorf("Origin = %q, want seed (stable sort keeps seed first)", out.Selected[0].Origin)
	}
}

// --- Footprint ---

func TestAllocate_EmitsFootprintEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	seeds := []stage2.ScoredCitation{seed("a.go", 5.0)}
	fetcher := &FakeFetcher{Bodies: map[string]string{cit("a.go", 1, 10).Key(): bodyN(40)}}
	a, _ := New(fetcher, WithFootprint(fp))
	_, _ = a.Allocate(context.Background(), seeds, nil)
	_ = fp.Sync()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if rec["event"] != "composer.stage4_allocated" {
		t.Errorf("event = %v", rec["event"])
	}
	if rec["selected_count"].(float64) != 1 {
		t.Errorf("selected_count = %v, want 1", rec["selected_count"])
	}
	for _, k := range []string{"budget_tokens", "candidate_count", "skipped_count", "used_tokens", "utilization", "fetch_errors", "empty_bodies"} {
		if _, ok := rec[k]; !ok {
			t.Errorf("footprint missing field %q", k)
		}
	}
}

// --- EstimateTokens unit tests ---

func TestEstimateTokens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 0},          // 3 chars / 4 = 0
		{"abcd", 1},         // 4 chars / 4 = 1
		{"abcdefgh", 2},     // 8 chars / 4 = 2
		{"한국어 코드", 1},       // 6 runes / 4 = 1 (rune count, not byte)
		{"hello world!", 3}, // 12 chars / 4 = 3
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := EstimateTokens(tc.in); got != tc.want {
				t.Errorf("EstimateTokens(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// --- FakeFetcher unit tests ---

func TestFakeFetcher_RecordsCalls(t *testing.T) {
	t.Parallel()
	f := &FakeFetcher{Bodies: map[string]string{cit("a.go", 1, 10).Key(): "body"}}
	_, _ = f.Fetch(context.Background(), cit("a.go", 1, 10))
	_, _ = f.Fetch(context.Background(), cit("b.go", 1, 10))
	if len(f.Calls) != 2 {
		t.Errorf("Calls count = %d, want 2", len(f.Calls))
	}
	f.ResetCalls()
	if len(f.Calls) != 0 {
		t.Errorf("ResetCalls did not clear, got %d", len(f.Calls))
	}
}

func TestFakeFetcher_MissingKeyReturnsEmpty(t *testing.T) {
	t.Parallel()
	f := &FakeFetcher{}
	body, err := f.Fetch(context.Background(), cit("nowhere.go", 1, 10))
	if err != nil {
		t.Fatalf("Fetch err = %v, want nil", err)
	}
	if body != "" {
		t.Errorf("body = %q, want empty for missing key", body)
	}
}

func TestFakeFetcher_ErrTakesPrecedence(t *testing.T) {
	t.Parallel()
	want := errors.New("backend down")
	f := &FakeFetcher{
		Bodies: map[string]string{cit("a.go", 1, 10).Key(): "body"},
		Err:    want,
	}
	_, err := f.Fetch(context.Background(), cit("a.go", 1, 10))
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
