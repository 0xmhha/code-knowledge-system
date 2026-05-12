// Package stage3 implements the composer pipeline's Stage 3: expand the
// citations from Stage 2 with their graph neighbors via ckg.Neighbors,
// adding caller/callee/imports/etc. context that Stage 2 (BM25 + Symbol)
// cannot reach on its own.
//
// Strategy:
//
//  1. Take Stage 2 citations and pass them through as Seeds (B.6 budget
//     and B.8 wire-up see the full Stage 2 work).
//  2. Expand only the top N seeds by score (default 10). Lower-ranked
//     seeds are weak signals — expanding them mostly amplifies noise.
//  3. For each expanded seed, call ckg.Neighbors with an Intent-specific
//     Relation set and Hop depth.
//  4. Aggregate by Target citation: dedup, taking the MAX score across
//     multiple paths (closest-path evidence wins). Skip neighbors that
//     are already in the Seed set — we don't double-count.
//  5. Cap and rank for downstream consumption.
//
// Intent influence at this stage is the strongest in the pipeline:
// "BugFix" wants caller chains; "ArchExplain" wants imports/implements/
// embeds; "TestAdd" wants tested_by. See intent_relations.go for the
// full mapping. Stage 3 deliberately leans on Intent because graph
// traversal is the place where "which structural relationships matter"
// differs most across user intents.
package stage3

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Default tuning. Phase E revisits with real-prompt data.
const (
	DefaultMaxSeedsToExpand  = 10
	DefaultNeighborsPerSeed  = 50
	DefaultMaxFinalNeighbors = 50
)

// Config tunes the expander's call budget and ranking weights.
type Config struct {
	// MaxSeedsToExpand caps how many of Stage 2's citations get expanded.
	// Lower-ranked seeds become passthrough only.
	MaxSeedsToExpand int

	// NeighborsPerSeed is the per-ckg-call MaxTotal passed to
	// ckg.NeighborsOpts. Bounds the work ckg does per seed.
	NeighborsPerSeed int

	// MaxFinalNeighbors caps the deduped Neighbors slice that Stage 3
	// emits. Stage 4 (B.6 budget) may trim further.
	MaxFinalNeighbors int
}

// DefaultConfig returns the Phase-0 tuning baseline.
func DefaultConfig() Config {
	return Config{
		MaxSeedsToExpand:  DefaultMaxSeedsToExpand,
		NeighborsPerSeed:  DefaultNeighborsPerSeed,
		MaxFinalNeighbors: DefaultMaxFinalNeighbors,
	}
}

// ScoredNeighbor is one graph-expanded edge with derived score and provenance.
type ScoredNeighbor struct {
	// Edge carries the canonical neighbor data (source seed citation,
	// target citation, relation, distance).
	Edge contract.Neighbor

	// Score is derived from the originating seed's score, discounted by
	// distance: seed.Score / (1 + distance). When multiple seeds reach
	// the same target, the maximum is kept (closest-path evidence wins).
	Score float64

	// Sources records every (seed, relation, distance) that produced this
	// target. Multi-path neighbors have multiple entries.
	Sources []string
}

// Stage3Output captures the result of one Expand call.
type Stage3Output struct {
	// Seeds is the full Stage 2 passthrough. Preserves the work that
	// Stage 2 did so B.6 budget and B.8 wire-up have access to BM25 +
	// Symbol evidence even when Stage 3 declined to expand a seed.
	Seeds []stage2.ScoredCitation

	// Neighbors is the deduped, score-sorted graph expansion. Each entry
	// is a citation NOT already present in Seeds — Stage 3 skips
	// self-loops via the seed-key set.
	Neighbors []ScoredNeighbor

	// SeedsExpanded is how many seeds were actually expanded (the rest
	// are passthrough-only). Bounded by Config.MaxSeedsToExpand.
	SeedsExpanded int

	// RelationCoverage counts how many neighbors arrived via each
	// relation type. Footprint and Phase E eval use this to spot
	// "Intent X always produces 0 implements" type signals.
	RelationCoverage map[contract.Relation]int

	// FailedSeeds lists seeds whose ckg.Neighbors call errored. The
	// rest of expansion proceeds; the operator can investigate why ckg
	// rejected specific citations.
	FailedSeeds []contract.Citation
}

// Expander runs Stage 3 of the composer pipeline.
type Expander struct {
	ckg    ckgclient.Client
	fp     *footprint.Logger
	config Config
}

// Option configures an Expander.
type Option func(*Expander)

// WithFootprint attaches a footprint.Logger; Expand emits
// composer.stage3_expanded on completion. Nil-safe.
func WithFootprint(fp *footprint.Logger) Option {
	return func(e *Expander) { e.fp = fp }
}

// WithConfig overrides the default tuning.
func WithConfig(cfg Config) Option {
	return func(e *Expander) { e.config = cfg }
}

// New constructs an Expander. Returns an error if ckg is nil.
func New(ckg ckgclient.Client, opts ...Option) (*Expander, error) {
	if ckg == nil {
		return nil, errors.New("stage3: nil ckg client")
	}
	e := &Expander{
		ckg:    ckg,
		config: DefaultConfig(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e, nil
}

// Expand takes Stage 2's citations and produces a Stage3Output that pairs
// them with their graph neighbors. Intent shapes both the Relation set
// passed to ckg and the hop depth.
//
// Empty seeds returns an empty (non-error) Stage3Output — the composer
// can decide whether a Stage-2 miss should abort the pipeline.
func (e *Expander) Expand(ctx context.Context, seeds []stage2.ScoredCitation, intent contract.Intent) (Stage3Output, error) {
	out := Stage3Output{
		Seeds:            seeds,
		RelationCoverage: make(map[contract.Relation]int),
	}
	if len(seeds) == 0 {
		e.emitFootprint(ctx, intent, out, 0)
		return out, nil
	}

	// Cap how many seeds we expand. Lower-ranked seeds pass through but
	// don't burn ckg calls.
	toExpand := seeds
	if len(toExpand) > e.config.MaxSeedsToExpand {
		toExpand = toExpand[:e.config.MaxSeedsToExpand]
	}
	out.SeedsExpanded = len(toExpand)

	relations := intentToRelations(intent)
	hops := intentToHops(intent)

	seedKeys := buildSeedKeys(seeds)
	agg := newNeighborAggregator()
	failedCount := 0

	for _, seed := range toExpand {
		nopts := ckgclient.NeighborsOpts{
			Relations: relations,
			Hops:      hops,
			MaxTotal:  e.config.NeighborsPerSeed,
		}
		neighbors, err := e.ckg.Neighbors(ctx, seed.Citation, nopts)
		if err != nil {
			out.FailedSeeds = append(out.FailedSeeds, seed.Citation)
			failedCount++
			continue
		}

		for _, n := range neighbors {
			// Skip self-loops: target is already a seed, so it carries
			// stronger evidence from Stage 2 — no need to demote it to
			// graph-derived score.
			if _, isSeed := seedKeys[n.Target.Key()]; isSeed {
				continue
			}
			// Closer paths get higher scores. Distance is 1-based per
			// ckgclient doc; the +1 guard avoids divide-by-zero if a
			// backend ever returns distance 0.
			score := seed.Score / float64(1+n.Distance)
			source := fmt.Sprintf("seed:%s:%s:dist=%d",
				seed.Citation.File, n.Relation, n.Distance)
			agg.add(n, score, source)
			out.RelationCoverage[n.Relation]++
		}
	}

	out.Neighbors = agg.results(e.config.MaxFinalNeighbors)
	e.emitFootprint(ctx, intent, out, failedCount)
	return out, nil
}

func (e *Expander) emitFootprint(ctx context.Context, intent contract.Intent, out Stage3Output, failedCount int) {
	if e.fp == nil {
		return
	}
	// Convert RelationCoverage to sorted-key string list for stable
	// JSON output (zap.Any on map produces unstable ordering).
	relTypes := make([]string, 0, len(out.RelationCoverage))
	for r := range out.RelationCoverage {
		relTypes = append(relTypes, string(r))
	}
	sortStrings(relTypes)

	fields := []zap.Field{
		zap.String("intent", string(intent)),
		zap.Int("seeds_total", len(out.Seeds)),
		zap.Int("seeds_expanded", out.SeedsExpanded),
		zap.Int("neighbor_count", len(out.Neighbors)),
		zap.Int("failed_seeds", failedCount),
		zap.Strings("relation_types", relTypes),
	}
	if len(out.Neighbors) > 0 {
		fields = append(fields,
			zap.String("top_neighbor", out.Neighbors[0].Edge.Target.String()),
			zap.Float64("top_neighbor_score", out.Neighbors[0].Score),
		)
	}
	e.fp.Event(ctx, "composer.stage3_expanded", fields...)
}

// buildSeedKeys returns the dedup set of seed citation keys for fast
// self-loop detection.
func buildSeedKeys(seeds []stage2.ScoredCitation) map[string]struct{} {
	out := make(map[string]struct{}, len(seeds))
	for _, sc := range seeds {
		out[sc.Citation.Key()] = struct{}{}
	}
	return out
}
