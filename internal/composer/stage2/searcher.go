// Package stage2 implements the composer pipeline's Stage 2: turn the
// keyword list from Stage 1 into precise code citations using ckg.
//
// Strategy: for each keyword, fan out to two ckg endpoints in parallel
// (semantically — actual calls are sequential per keyword, parallel work
// is across the keyword list inside the BM25/Symbol pair):
//
//  1. BM25Search — catches "tokens that look like the keyword appear in
//     this chunk of code". Good for conceptual matches and call-site
//     detection ("Login" matches a chunk that calls Login()).
//  2. FindSymbol — catches "the keyword IS a Go symbol name". Good for
//     pointing at the canonical definition site, which BM25 may miss
//     when the definition file has fewer mentions than call sites.
//
// Results are aggregated per-citation: BM25 hits contribute their native
// score; FindSymbol hits contribute a fixed SymbolBonus. Same citation
// hit by multiple keywords sums its evidence. The output is a
// score-sorted, capped list of ScoredCitation plus debugging metadata.
//
// Intent influences only the FindSymbol filter (which SymbolKinds to
// consider), per the design decision documented in stage1: the lexical
// extraction in Stage 1 is universal, the precise filter in Stage 2 is
// intent-shaped, and the relation-graph expansion in Stage 3 (B.5) is
// also intent-shaped.
package stage2

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Default tuning. Phase E will revisit with real-prompt data.
const (
	DefaultBM25K        = 10
	DefaultMaxCitations = 30
	DefaultSymbolBonus  = 5.0
)

// Config tunes the searcher's call budget and ranking weights.
type Config struct {
	// BM25K is the per-keyword K passed to ckg.BM25Search.
	BM25K int
	// MaxCitations caps the final ScoredCitation slice length. The
	// downstream graph expander (B.5) sees at most this many seeds.
	MaxCitations int
	// SymbolBonus is the score increment a citation receives for each
	// FindSymbol match. Comparable to a single strong BM25 hit so exact
	// matches weigh meaningfully without dominating semantic evidence.
	SymbolBonus float64
}

// DefaultConfig returns the Phase-0 tuning baseline.
func DefaultConfig() Config {
	return Config{
		BM25K:        DefaultBM25K,
		MaxCitations: DefaultMaxCitations,
		SymbolBonus:  DefaultSymbolBonus,
	}
}

// Stage2Output captures the result of one Search call.
//
// Citations vs Hits: Citations is the canonical, deduped, score-ranked
// output that downstream stages (B.5 graph expander, evaluation) consume.
// Hits is a raw audit trail — it preserves every BM25 result returned by
// ckg, including duplicates when the same citation hits via multiple
// keywords. Precision/recall computation should use Citations; Hits
// exists for "why did Stage 2 land on this citation set" debugging.
type Stage2Output struct {
	// Citations is the deduped, score-sorted list of citation candidates
	// the graph expander should explore. Cap-bounded by Config.MaxCitations.
	// Use this for evaluation metrics (precision/recall vs human baselines).
	Citations []ScoredCitation

	// Hits is the raw BM25-hit audit trail. Not deduped: the same citation
	// can appear multiple times if multiple keywords matched it. Useful for
	// footprint analysis and per-keyword evidence reconstruction; do not
	// use directly for precision/recall (use Citations instead).
	Hits []contract.Hit

	// Symbols records the FindSymbol results per keyword. Useful for
	// footprint debugging ("which keyword resolved to which symbol?").
	Symbols map[string][]contract.Citation

	// FailedKeywords lists keywords whose BM25 AND FindSymbol both
	// produced zero results (or both errored). Surfaces the keywords
	// that Stage 1 surfaced but ckg can't ground.
	FailedKeywords []string

	// Coverage is the fraction of input keywords that produced at least
	// one ckg hit. 1.0 = every keyword is grounded; 0 = none are.
	Coverage float64
}

// Searcher runs Stage 2 of the composer pipeline.
type Searcher struct {
	ckg    ckgclient.Client
	fp     *footprint.Logger
	config Config
}

// Option configures a Searcher.
type Option func(*Searcher)

// WithFootprint attaches a footprint.Logger; Search emits
// composer.stage2_searched on completion. Nil-safe.
func WithFootprint(fp *footprint.Logger) Option {
	return func(s *Searcher) { s.fp = fp }
}

// WithConfig overrides the default tuning.
func WithConfig(cfg Config) Option {
	return func(s *Searcher) { s.config = cfg }
}

// New constructs a Searcher. Returns an error if ckg is nil.
func New(ckg ckgclient.Client, opts ...Option) (*Searcher, error) {
	if ckg == nil {
		return nil, errors.New("stage2: nil ckg client")
	}
	s := &Searcher{
		ckg:    ckg,
		config: DefaultConfig(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Search runs BM25Search + FindSymbol for each keyword and aggregates the
// results. Intent shapes the FindSymbol filter; keywords drive everything
// else. Per-keyword ckg errors are tolerated — that keyword may end up in
// FailedKeywords, the rest proceed.
//
// Empty keyword input returns an empty (but non-error) Stage2Output:
// composer can decide what to do with a Stage 1 that produced nothing.
func (s *Searcher) Search(ctx context.Context, keywords []string, intent contract.Intent) (Stage2Output, error) {
	out := Stage2Output{
		Symbols: make(map[string][]contract.Citation),
	}
	if len(keywords) == 0 {
		s.emitFootprint(ctx, intent, keywords, out, 0, 0, "")
		return out, nil
	}

	kinds := intentToKinds(intent)
	pathGlob := intentPathGlob(intent)
	agg := newAggregator(s.config.SymbolBonus)
	hitCount := 0
	bm25Errors := 0
	symbolErrors := 0

	for _, kw := range keywords {
		anyHit := false

		bm25Hits, bm25Err := s.ckg.BM25Search(ctx, kw, ckgclient.SearchOpts{K: s.config.BM25K})
		if bm25Err != nil {
			bm25Errors++
		} else if len(bm25Hits) > 0 {
			anyHit = true
			out.Hits = append(out.Hits, bm25Hits...)
			for _, h := range bm25Hits {
				agg.addBM25Hit(kw, h)
			}
		}

		symbolCits, symErr := s.ckg.FindSymbol(ctx, kw, ckgclient.SymbolOpts{Kinds: kinds})
		if symErr != nil {
			symbolErrors++
		} else if len(symbolCits) > 0 {
			anyHit = true
			out.Symbols[kw] = symbolCits
			for _, c := range symbolCits {
				agg.addSymbolHit(kw, c)
			}
		}

		if !anyHit {
			out.FailedKeywords = append(out.FailedKeywords, kw)
			continue
		}
		hitCount++
	}

	// Intent-driven supplemental BM25 pass: pulls in extra hits from
	// a path subset the intent doc names (e.g. *_test.go for TestAdd).
	// Results feed the same aggregator so a path-filtered match that
	// also appeared in the unfiltered pass double-counts on purpose —
	// that overlap IS the boost the intent promises.
	if pathGlob != "" {
		for _, kw := range keywords {
			extraHits, extraErr := s.ckg.BM25Search(ctx, kw, ckgclient.SearchOpts{
				K:      s.config.BM25K,
				Filter: ckgclient.SearchFilter{PathGlob: pathGlob},
			})
			if extraErr != nil {
				bm25Errors++
				continue
			}
			if len(extraHits) == 0 {
				continue
			}
			out.Hits = append(out.Hits, extraHits...)
			for _, h := range extraHits {
				agg.addBM25Hit(kw, h)
			}
		}
	}

	out.Citations = agg.results(s.config.MaxCitations)
	out.Coverage = float64(hitCount) / float64(len(keywords))

	s.emitFootprint(ctx, intent, keywords, out, bm25Errors, symbolErrors, pathGlob)
	return out, nil
}

func (s *Searcher) emitFootprint(ctx context.Context, intent contract.Intent, keywords []string, out Stage2Output, bm25Errors, symbolErrors int, pathGlob string) {
	if s.fp == nil {
		return
	}
	fields := []zap.Field{
		zap.String("intent", string(intent)),
		zap.String("intent_path_glob", pathGlob),
		zap.Int("keyword_count", len(keywords)),
		zap.Int("hit_keywords", len(keywords)-len(out.FailedKeywords)),
		zap.Strings("failed_keywords", out.FailedKeywords),
		zap.Float64("coverage", out.Coverage),
		zap.Int("bm25_total_hits", len(out.Hits)),
		zap.Int("symbol_total_hits", countSymbolHits(out.Symbols)),
		zap.Int("citation_count", len(out.Citations)),
		// Backend error visibility — when these jump in production, ckg
		// is degraded and Stage 2 quality silently drops. Surface for
		// alerting and trend analysis.
		zap.Int("bm25_errors", bm25Errors),
		zap.Int("symbol_errors", symbolErrors),
	}
	if len(out.Citations) > 0 {
		fields = append(fields, zap.String("top_citation", out.Citations[0].Citation.String()))
		fields = append(fields, zap.Float64("top_score", out.Citations[0].Score))
	}
	s.fp.Event(ctx, "composer.stage2_searched", fields...)
}

func countSymbolHits(m map[string][]contract.Citation) int {
	n := 0
	for _, cs := range m {
		n += len(cs)
	}
	return n
}
