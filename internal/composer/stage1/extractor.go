// Package stage1 implements the composer pipeline's Stage 1: turn a
// natural-language vibe prompt into a short list of precise keywords that
// the ckg BM25 search (Stage 2) can use to locate exact code.
//
// Strategy (RAG-style iterative retrieval + rerank):
//
//  1. Broad recall via ckv.SemanticSearch — surfaces semantically related
//     code chunks regardless of exact wording.
//  2. Candidate keyword extraction — combine identifiers parsed from the
//     prompt itself with file basenames of the hit citations.
//  3. BM25 rerank via ckg — score each candidate by how strongly it hits
//     real code in the graph backend. Zero-score candidates are dropped.
//  4. Confidence gate — if top-1 BM25 score dominates the candidates,
//     accept; otherwise augment the prompt with the top reranked keywords
//     and loop (bounded by MaxRounds).
//
// The strategy mirrors what coding agents like Cursor do internally: when
// the first search is ambiguous, refine the query with what was learned
// and try again. Multi-round retrieval is bounded so latency is predictable.
//
// All decisions surface on the composer.stage1_extracted footprint event
// so debugging "why was this keyword picked" stays cheap.
package stage1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/internal/vocab"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Default tuning values. Phase E tuning revisits with real-prompt data.
const (
	DefaultInitialK      = 20  // ckv recall breadth per round
	DefaultRerankPerKW   = 5   // ckg BM25 K per keyword
	DefaultMaxRounds     = 2   // initial + 1 refinement
	DefaultMinConfidence = 0.5 // stop iterating above this
	DefaultMaxKeywords   = 5   // final keyword count cap
	DefaultAugmentTopN   = 3   // how many top keywords feed back into the augmented query
)

// Config tunes the extractor's retrieval behavior.
type Config struct {
	// InitialK is how many candidates ckv returns per recall round.
	InitialK int
	// RerankPerKW is how many ckg BM25 hits to fetch per candidate keyword.
	RerankPerKW int
	// MaxRounds caps the number of recall-rerank iterations
	// (always >= 1; the first call counts as round 1).
	MaxRounds int
	// MinConfidence is the BM25 concentration threshold below which the
	// extractor augments the prompt and runs another round.
	MinConfidence float64
	// MaxKeywords caps the final Keywords slice length.
	MaxKeywords int
	// AugmentTopN is how many top-reranked keywords to splice into the
	// augmented query for the next round.
	AugmentTopN int
}

// DefaultConfig returns the Phase-0 tuning baseline.
func DefaultConfig() Config {
	return Config{
		InitialK:      DefaultInitialK,
		RerankPerKW:   DefaultRerankPerKW,
		MaxRounds:     DefaultMaxRounds,
		MinConfidence: DefaultMinConfidence,
		MaxKeywords:   DefaultMaxKeywords,
		AugmentTopN:   DefaultAugmentTopN,
	}
}

// Stage1Output captures the result of one Extract call. The composer
// pipeline's Stage 2 consumes Keywords; everything else is provenance and
// observability data for debugging and evaluation.
type Stage1Output struct {
	// Keywords are the final, BM25-validated terms Stage 2 should send to
	// ckg. Order is from highest BM25 score (most precise) to lowest.
	Keywords []string

	// Hits are the union of ckv search results across all rounds. The
	// composer's evidence assembler uses these as citation candidates.
	Hits []contract.Hit

	// Confidence is the BM25 concentration of the final round
	// (top1_score / total_score). 1.0 = single dominant keyword,
	// 1/N = uniform distribution across candidates.
	Confidence float64

	// Rounds is the number of recall-rerank iterations actually run
	// (>=1, <=Config.MaxRounds).
	Rounds int

	// AugmentedQueries records the actual query strings sent to ckv in
	// each round. Round 0 is the raw prompt; later entries are
	// prompt + " " + top reranked keywords.
	AugmentedQueries []string

	// VocabExpanded reports whether the vocabulary resolver matched at
	// least one glossary entry against the original prompt. False means
	// either no resolver was wired or no alias hit; in both cases the
	// ckv query and the prompt match verbatim.
	VocabExpanded bool

	// VocabKeywords are the code keywords appended to the original prompt
	// by the vocabulary resolver. Empty when VocabExpanded is false.
	// Stage 2 can union these into its keyword candidate set so BM25
	// search lands on identifiers the prompt itself never mentioned.
	VocabKeywords []string
}

// Extractor runs Stage 1 of the composer pipeline.
type Extractor struct {
	ckv    ckvclient.Client
	ckg    ckgclient.Client
	vocab  *vocab.Resolver
	fp     *footprint.Logger
	config Config
}

// Option configures an Extractor.
type Option func(*Extractor)

// WithFootprint attaches a footprint.Logger; Extract emits a
// composer.stage1_extracted event on completion.
func WithFootprint(fp *footprint.Logger) Option {
	return func(e *Extractor) { e.fp = fp }
}

// WithConfig overrides the default tuning.
func WithConfig(cfg Config) Option {
	return func(e *Extractor) { e.config = cfg }
}

// WithVocab attaches a vocabulary resolver. When set, Extract expands the
// incoming prompt with project-specific code keywords (e.g., "쿼럼 미달"
// -> "QuorumSize F() WBFTPrepares") before issuing the ckv semantic
// search, so retrieval lands on the right symbols even when the user
// types Korean or domain-vague English. nil resolver is a no-op:
// vocab.Resolver.Resolve already returns the input unchanged when the
// glossary is empty or the receiver is nil.
func WithVocab(r *vocab.Resolver) Option {
	return func(e *Extractor) { e.vocab = r }
}

// New constructs an Extractor. Returns an error if either client is nil.
func New(ckv ckvclient.Client, ckg ckgclient.Client, opts ...Option) (*Extractor, error) {
	if ckv == nil {
		return nil, errors.New("stage1: nil ckv client")
	}
	if ckg == nil {
		return nil, errors.New("stage1: nil ckg client")
	}
	e := &Extractor{
		ckv:    ckv,
		ckg:    ckg,
		config: DefaultConfig(),
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.config.MaxRounds < 1 {
		e.config.MaxRounds = 1
	}
	return e, nil
}

// Extract runs the iterative retrieval+rerank pipeline. Intent is used to
// shape candidate extraction (different Intents emphasize different
// identifier patterns); the prompt drives the actual search.
func (e *Extractor) Extract(ctx context.Context, prompt string, intent contract.Intent) (Stage1Output, error) {
	if strings.TrimSpace(prompt) == "" {
		return Stage1Output{}, errors.New("stage1: empty prompt")
	}

	out := Stage1Output{}

	// Vocabulary expansion (pre-round): rewrite the user's prompt by
	// appending project glossary code keywords whose aliases match the
	// prompt text. The expanded query rides into ckv.SemanticSearch so
	// retrieval has the literal identifiers to match against. Nil
	// resolver or empty glossary degrade to a verbatim pass-through.
	expansion := e.vocab.Resolve(prompt)
	currentQuery := expansion.Expanded
	out.VocabExpanded = len(expansion.MatchedKeywords) > 0
	out.VocabKeywords = expansion.MatchedKeywords

	for round := 1; round <= e.config.MaxRounds; round++ {
		// Recall: broad ckv semantic search with the current (possibly
		// augmented) query.
		hits, err := e.ckv.SemanticSearch(ctx, currentQuery, ckvclient.SearchOpts{K: e.config.InitialK})
		if err != nil {
			// First-round ckv failure aborts; later-round failures stop
			// iteration but return what we already have.
			if round == 1 {
				return out, fmt.Errorf("stage1: ckv semantic search round %d: %w", round, err)
			}
			break
		}

		out.Hits = mergeHits(out.Hits, hits)
		out.AugmentedQueries = append(out.AugmentedQueries, currentQuery)
		out.Rounds = round

		// Extract candidate keywords from the prompt + all hits so far.
		candidates := extractKeywords(prompt, out.Hits, intent)

		// Rerank via ckg BM25.
		reranked, confidence := e.rerank(ctx, candidates)
		out.Keywords = reranked
		out.Confidence = confidence

		// Confidence gate: accept if dominant, otherwise augment and loop.
		if confidence >= e.config.MinConfidence || round == e.config.MaxRounds {
			break
		}
		// Re-augment from the vocab-expanded prompt so subsequent rounds
		// keep the glossary keywords on top of the reranked additions.
		currentQuery = augmentQuery(expansion.Expanded, reranked, e.config.AugmentTopN)
	}

	// Cap the final keyword count.
	if len(out.Keywords) > e.config.MaxKeywords {
		out.Keywords = out.Keywords[:e.config.MaxKeywords]
	}

	e.emitFootprint(ctx, intent, out)
	return out, nil
}

func (e *Extractor) emitFootprint(ctx context.Context, intent contract.Intent, out Stage1Output) {
	if e.fp == nil {
		return
	}
	e.fp.Event(ctx, "composer.stage1_extracted",
		zap.String("intent", string(intent)),
		zap.Int("rounds", out.Rounds),
		zap.Int("hit_count", len(out.Hits)),
		zap.Int("keyword_count", len(out.Keywords)),
		zap.Float64("confidence", out.Confidence),
		zap.Strings("keywords", out.Keywords),
		zap.Strings("augmented_queries", out.AugmentedQueries),
		zap.Bool("vocab_expanded", out.VocabExpanded),
		zap.Strings("vocab_keywords", out.VocabKeywords),
	)
}

// augmentQuery splices the top N reranked keywords into the original prompt
// to produce a refined query for the next ckv round.
//
// We keep the original prompt as the leading text so the embedder still
// sees the user's intent in natural form; the keywords are appended as
// extra context, the way one might ask a colleague to re-search with
// specific terms in mind.
func augmentQuery(prompt string, reranked []string, topN int) string {
	if len(reranked) == 0 {
		return prompt
	}
	if topN > len(reranked) {
		topN = len(reranked)
	}
	return prompt + " " + strings.Join(reranked[:topN], " ")
}

// mergeHits appends src into dst, deduplicating by Citation.Key.
func mergeHits(dst, src []contract.Hit) []contract.Hit {
	if len(src) == 0 {
		return dst
	}
	seen := make(map[string]struct{}, len(dst))
	for _, h := range dst {
		seen[h.Citation.Key()] = struct{}{}
	}
	for _, h := range src {
		if _, dup := seen[h.Citation.Key()]; dup {
			continue
		}
		seen[h.Citation.Key()] = struct{}{}
		dst = append(dst, h)
	}
	return dst
}
