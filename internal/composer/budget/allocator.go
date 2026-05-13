package budget

import (
	"context"
	"errors"
	"sort"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Defaults. Phase E will revisit with real-prompt data.
const (
	// DefaultMaxTokens approximates "typical Claude/GPT context budget
	// allocated to evidence" — composer is one of several inputs the
	// downstream caller will combine with prompts and instructions.
	DefaultMaxTokens = 8000

	// DefaultOverheadReserve is the fraction of MaxTokens held back for
	// pack metadata (sanitize_report, citations index, integrity_hash).
	// Bodies share the remaining 90%.
	DefaultOverheadReserve = 0.10
)

// Config tunes the allocator's budget and reserve.
type Config struct {
	// MaxTokens is the total token budget for the produced bodies plus
	// pack metadata overhead.
	MaxTokens int

	// OverheadReserve is the fraction of MaxTokens NOT available for
	// bodies (range 0..1). Defaults to 0.10 — bodies get 90% of MaxTokens.
	OverheadReserve float64
}

// DefaultConfig returns the Phase-0 tuning baseline.
func DefaultConfig() Config {
	return Config{
		MaxTokens:       DefaultMaxTokens,
		OverheadReserve: DefaultOverheadReserve,
	}
}

// SelectedItem is one body that made it into the budget.
//
// Body intentionally holds a copy of the fetched text (not a reference
// into the fetcher's storage). This costs memory for large allocations
// but keeps every stage's state fully inspectable from a single
// Stage4Output value — critical for debugging the Phase-B composer
// while the pipeline is still being tuned. A streaming variant that
// pipes bodies directly into EvidencePack assembly is a candidate
// optimization once the pipeline has stabilized and Phase E has
// finished measuring the quality of the current design.
type SelectedItem struct {
	Citation      contract.Citation
	Body          string
	TokenEstimate int
	Score         float64
	// Origin is "seed" (from Stage 2) or "neighbor" (from Stage 3).
	Origin string
	// Sources is copied from the originating ScoredCitation/ScoredNeighbor
	// for full evidence-trail preservation.
	Sources []string
}

// Stage4Output captures the result of one Allocate call.
type Stage4Output struct {
	// Selected is the score-ordered list of bodies that fit in the
	// budget. EvidencePack assembly (B.8) consumes this directly.
	Selected []SelectedItem

	// Skipped lists citations that were ranked but didn't fit (over
	// budget) or had no body (fetcher returned "" or errored). Useful
	// for footprint debugging.
	Skipped []contract.Citation

	// BudgetTokens is the body-share of MaxTokens (MaxTokens * (1-Overhead)).
	BudgetTokens int

	// UsedTokens is the sum of TokenEstimate across Selected.
	UsedTokens int

	// Utilization is UsedTokens / BudgetTokens (0..1). Tight allocations
	// approach 1.0; sparse ones stay low.
	Utilization float64

	// FetchErrors counts how many BodyFetcher.Fetch calls returned a
	// non-nil error. Empty-body responses are counted separately below.
	FetchErrors int

	// EmptyBodies counts how many Fetch calls returned ("", nil) — the
	// "available but vanished" signal (deleted file, missing chunk, etc.).
	EmptyBodies int
}

// Allocator runs Stage 4 of the composer pipeline.
type Allocator struct {
	fetcher BodyFetcher
	fp      *footprint.Logger
	config  Config
}

// Option configures an Allocator.
type Option func(*Allocator)

// WithFootprint attaches a footprint.Logger. Nil-safe.
func WithFootprint(fp *footprint.Logger) Option {
	return func(a *Allocator) { a.fp = fp }
}

// WithConfig overrides the default tuning.
func WithConfig(cfg Config) Option {
	return func(a *Allocator) { a.config = cfg }
}

// New constructs an Allocator. Returns an error if fetcher is nil or
// the config has out-of-range values.
func New(fetcher BodyFetcher, opts ...Option) (*Allocator, error) {
	if fetcher == nil {
		return nil, errors.New("budget: nil fetcher")
	}
	a := &Allocator{
		fetcher: fetcher,
		config:  DefaultConfig(),
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.config.MaxTokens < 0 {
		return nil, errors.New("budget: MaxTokens must be >= 0")
	}
	if a.config.OverheadReserve < 0 || a.config.OverheadReserve > 1 {
		return nil, errors.New("budget: OverheadReserve must be in [0, 1]")
	}
	return a, nil
}

// Allocate merges seeds and neighbors into a single ranked list, fetches
// each candidate's body in order, and greedily fits within the budget.
//
// Candidates whose Fetcher returns an error are counted in FetchErrors;
// empty-body responses are counted in EmptyBodies. Both appear in Skipped.
// A candidate that doesn't fit (body too large for remaining budget) is
// skipped — the loop continues, since a smaller later candidate might fit.
//
// Empty inputs are not an error; the output is empty (with a non-zero
// BudgetTokens so callers can still inspect what budget was set).
func (a *Allocator) Allocate(ctx context.Context, seeds []stage2.ScoredCitation, neighbors []stage3.ScoredNeighbor) (Stage4Output, error) {
	candidates := mergeCandidates(seeds, neighbors)

	bodyBudget := int(float64(a.config.MaxTokens) * (1.0 - a.config.OverheadReserve))
	out := Stage4Output{
		BudgetTokens: bodyBudget,
	}

	used := 0
	for _, c := range candidates {
		body, err := a.fetcher.Fetch(ctx, c.Citation)
		if err != nil {
			out.FetchErrors++
			out.Skipped = append(out.Skipped, c.Citation)
			continue
		}
		if body == "" {
			out.EmptyBodies++
			out.Skipped = append(out.Skipped, c.Citation)
			continue
		}
		tokens := EstimateTokens(body)
		if used+tokens > bodyBudget {
			out.Skipped = append(out.Skipped, c.Citation)
			continue
		}
		out.Selected = append(out.Selected, SelectedItem{
			Citation:      c.Citation,
			Body:          body,
			TokenEstimate: tokens,
			Score:         c.Score,
			Origin:        c.Origin,
			Sources:       c.Sources,
		})
		used += tokens
	}

	out.UsedTokens = used
	if bodyBudget > 0 {
		out.Utilization = float64(used) / float64(bodyBudget)
	}

	a.emitFootprint(ctx, out, len(candidates))
	return out, nil
}

func (a *Allocator) emitFootprint(ctx context.Context, out Stage4Output, candidateCount int) {
	if a.fp == nil {
		return
	}
	fields := []zap.Field{
		zap.Int("budget_tokens", out.BudgetTokens),
		zap.Int("candidate_count", candidateCount),
		zap.Int("selected_count", len(out.Selected)),
		zap.Int("skipped_count", len(out.Skipped)),
		zap.Int("used_tokens", out.UsedTokens),
		zap.Float64("utilization", out.Utilization),
		zap.Int("fetch_errors", out.FetchErrors),
		zap.Int("empty_bodies", out.EmptyBodies),
	}
	if len(out.Selected) > 0 {
		fields = append(fields,
			zap.String("top_selected", out.Selected[0].Citation.String()),
			zap.Float64("top_score", out.Selected[0].Score),
		)
	}
	a.fp.Event(ctx, "composer.stage4_allocated", fields...)
}

// candidate is the unified shape used by greedy allocation. Either
// origin (seed or neighbor) contributes the same fields, so the
// allocator doesn't need parallel branches.
type candidate struct {
	Citation contract.Citation
	Score    float64
	Origin   string
	Sources  []string
}

// mergeCandidates combines seeds and neighbors into one score-sorted
// list. Stage 3's self-loop guard already prevents a citation from
// appearing in both, but this function dedupes by Citation.Key() as a
// defensive backstop — if a future Stage 3 change breaks the guard,
// the allocator still produces sensible output.
func mergeCandidates(seeds []stage2.ScoredCitation, neighbors []stage3.ScoredNeighbor) []candidate {
	out := make([]candidate, 0, len(seeds)+len(neighbors))
	for _, s := range seeds {
		out = append(out, candidate{
			Citation: s.Citation,
			Score:    s.Score,
			Origin:   "seed",
			Sources:  s.Sources,
		})
	}
	for _, n := range neighbors {
		out = append(out, candidate{
			Citation: n.Edge.Target,
			Score:    n.Score,
			Origin:   "neighbor",
			Sources:  n.Sources,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Citation.File != out[j].Citation.File {
			return out[i].Citation.File < out[j].Citation.File
		}
		return out[i].Citation.StartLine < out[j].Citation.StartLine
	})

	seen := make(map[string]struct{}, len(out))
	dedup := make([]candidate, 0, len(out))
	for _, c := range out {
		key := c.Citation.Key()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		dedup = append(dedup, c)
	}
	return dedup
}
