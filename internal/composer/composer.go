// Package composer is the Phase-B wire-up that ties the six composer
// stages into one Compose entry point.
//
// Pipeline (sequential):
//
//	prompt
//	  -> intent.Classify        (vibe -> Intent; degrades to Unknown on error)
//	  -> stage1.Extract         (Intent + prompt -> keywords)
//	  -> stage2.Search          (keywords -> ScoredCitations)
//	  -> stage3.Expand          (citations -> Seeds + Neighbors)
//	  -> budget.Allocate        (greedy fit within token budget)
//	  -> sanitize.Sanitize      (apply policies; may fail_closed)
//	  -> assemble EvidencePack  (citation/body/neighbor filtering)
//	  -> contract.StampIntegrity
//
// The pipeline is strictly sequential — each stage consumes the prior
// stage's output. No goroutines (caller controls request-level
// concurrency).
//
// Error policy:
//   - intent.Classify failure: degrade to IntentUnknown and continue.
//     A miss-classified Intent is recoverable (composer just uses
//     broader fan-out); a hard error here would abort otherwise-fine
//     pipelines.
//   - Any other stage error: return immediately with composer.<stage>
//     prefix on the error message.
//   - sanitize.FailClosed: return ErrFailClosed wrapping the rule ID.
//     The caller MUST treat this as "pack refused" and not retry without
//     understanding why the rule fired.
package composer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/composer/budget"
	"github.com/0xmhha/code-knowledge-system/internal/composer/intent"
	"github.com/0xmhha/code-knowledge-system/internal/composer/sanitize"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage1"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// DefaultBuilderVersion is stamped into PackMetadata when no override
// is supplied via WithBuilderVersion.
const DefaultBuilderVersion = "cks-composer/0.0.1-dev"

// ErrFailClosed is returned by Compose when a sanitize fail_closed rule
// matched. Wraps the rule ID; callers should errors.Is(err, ErrFailClosed)
// to branch on the boundary refusal.
var ErrFailClosed = errors.New("composer: sanitize fail_closed")

// Composer orchestrates the six-stage pipeline.
type Composer struct {
	intent         *intent.Classifier
	stage1         *stage1.Extractor
	stage2         *stage2.Searcher
	stage3         *stage3.Expander
	budget         *budget.Allocator
	sanitize       *sanitize.Engine
	fp             *footprint.Logger
	builderVersion string
}

// Option configures a Composer.
type Option func(*Composer)

// WithFootprint attaches a footprint.Logger. The composer emits one
// composer.compose_complete event per Compose call summarizing the
// end-to-end pipeline result.
func WithFootprint(fp *footprint.Logger) Option {
	return func(c *Composer) { c.fp = fp }
}

// WithBuilderVersion overrides the version string stamped into
// PackMetadata.BuilderVersion. Useful in tests and for deployments
// that want to surface a git SHA or build tag.
func WithBuilderVersion(v string) Option {
	return func(c *Composer) { c.builderVersion = v }
}

// New constructs a Composer. Every stage dependency is required; nil
// for any of them is rejected because the pipeline cannot meaningfully
// degrade if a stage component is missing.
func New(
	intentClassifier *intent.Classifier,
	stage1Extractor *stage1.Extractor,
	stage2Searcher *stage2.Searcher,
	stage3Expander *stage3.Expander,
	budgetAllocator *budget.Allocator,
	sanitizeEngine *sanitize.Engine,
	opts ...Option,
) (*Composer, error) {
	if intentClassifier == nil {
		return nil, errors.New("composer: nil intent classifier")
	}
	if stage1Extractor == nil {
		return nil, errors.New("composer: nil stage1 extractor")
	}
	if stage2Searcher == nil {
		return nil, errors.New("composer: nil stage2 searcher")
	}
	if stage3Expander == nil {
		return nil, errors.New("composer: nil stage3 expander")
	}
	if budgetAllocator == nil {
		return nil, errors.New("composer: nil budget allocator")
	}
	if sanitizeEngine == nil {
		return nil, errors.New("composer: nil sanitize engine")
	}
	c := &Composer{
		intent:         intentClassifier,
		stage1:         stage1Extractor,
		stage2:         stage2Searcher,
		stage3:         stage3Expander,
		budget:         budgetAllocator,
		sanitize:       sanitizeEngine,
		builderVersion: DefaultBuilderVersion,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Compose runs the full pipeline. Returns a stamped, IsValid()-passing
// EvidencePack on success, or an error on hard failure / fail_closed.
//
// Compose discards the RetrievalTrace; callers that want to inspect how the
// pack's seeds were selected (e.g. the evaluation harness) call ComposeTraced.
func (c *Composer) Compose(ctx context.Context, prompt string) (contract.EvidencePack, error) {
	pack, _, err := c.ComposeTraced(ctx, prompt)
	return pack, err
}

// ComposeTraced is Compose plus a RetrievalTrace describing the ckv→ckg funnel
// that produced the pack (Producer="composer"). The trace shares its shape
// with the LLM agent's trace so the two retrieval algorithms can be scored
// against each other. On any error the returned trace is the zero value.
func (c *Composer) ComposeTraced(ctx context.Context, prompt string) (contract.EvidencePack, contract.RetrievalTrace, error) {
	if prompt == "" {
		return contract.EvidencePack{}, contract.RetrievalTrace{}, errors.New("composer: empty prompt")
	}
	start := time.Now()

	// Attach an InstructionCollector so dummy ckv/ckg clients can record
	// the calls they would have made; real clients ignore the collector.
	// The collected instructions ride out on EvidencePack.Instructions so
	// the upstream LLM (coding-agent) can execute the corresponding
	// skills against go-stablenet source.
	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	// 1. Intent classification — degrade to Unknown on error.
	// Per-stage latency: captured at each stage boundary so §9's validation
	// spike can attribute cost (only total elapsed was emitted before). The
	// markers are cheap time.Now() reads on the success path.
	var tm stageTimings
	mark := time.Now()
	next := func(d *time.Duration) { now := time.Now(); *d = now.Sub(mark); mark = now }

	cls, err := c.intent.Classify(ctx, prompt)
	if err != nil {
		// Don't return: composer with IntentUnknown still produces a
		// usable pack (broader fan-out, default sanitize policy).
		cls = intent.Classification{Intent: contract.IntentUnknown}
	}
	intentVal := cls.Intent
	next(&tm.intent)

	// 2. Stage 1 — keyword extraction.
	s1Out, err := c.stage1.Extract(ctx, prompt, intentVal)
	if err != nil {
		return contract.EvidencePack{}, contract.RetrievalTrace{}, fmt.Errorf("composer: stage1: %w", err)
	}
	next(&tm.stage1)

	// 3. Stage 2 — ckg citation search.
	s2Out, err := c.stage2.Search(ctx, s1Out.Keywords, s1Out.Hits, intentVal)
	if err != nil {
		return contract.EvidencePack{}, contract.RetrievalTrace{}, fmt.Errorf("composer: stage2: %w", err)
	}
	next(&tm.stage2)

	// Build the retrieval trace from the funnel's provenance (Stage 1 ckv
	// recall rounds + Stage 2 ckg seeds) before the graph stage consumes them.
	trace := buildComposerTrace(prompt, intentVal, s1Out, s2Out)

	// 4. Stage 3 — graph expansion.
	s3Out, err := c.stage3.Expand(ctx, s2Out.Citations, intentVal)
	if err != nil {
		return contract.EvidencePack{}, contract.RetrievalTrace{}, fmt.Errorf("composer: stage3: %w", err)
	}
	next(&tm.stage3)

	// 5. Stage 4 — budget allocation (fetches bodies + greedy fits).
	s4Out, err := c.budget.Allocate(ctx, s3Out.Seeds, s3Out.Neighbors)
	if err != nil {
		return contract.EvidencePack{}, contract.RetrievalTrace{}, fmt.Errorf("composer: stage4: %w", err)
	}
	next(&tm.stage4)

	// 6. Stage 5 — sanitize the selected bodies.
	sanIn := make([]sanitize.Sanitizable, 0, len(s4Out.Selected))
	for _, item := range s4Out.Selected {
		sanIn = append(sanIn, sanitize.Sanitizable{
			Citation: item.Citation,
			Body:     item.Body,
		})
	}
	s5Out, err := c.sanitize.Sanitize(ctx, sanIn)
	if err != nil {
		return contract.EvidencePack{}, contract.RetrievalTrace{}, fmt.Errorf("composer: stage5: %w", err)
	}
	next(&tm.stage5)
	if s5Out.FailClosed {
		return contract.EvidencePack{}, contract.RetrievalTrace{}, fmt.Errorf("%w: rule=%s", ErrFailClosed, s5Out.FailClosedRule)
	}

	// 7. Assemble EvidencePack — drop sanitized-out items, filter
	// neighbors whose endpoints are missing from the final citation
	// set (required for EvidencePack.IsValid).
	pack := assemblePack(prompt, intentVal, s3Out, s4Out, s5Out, c.builderVersion)

	// Attach any dummy-backend instructions accumulated during the run
	// (empty when the wired backends are real).
	pack.Instructions = collector.All()

	// 8. Integrity stamp — SHA-256 over the canonical pack form.
	if err := contract.StampIntegrity(&pack); err != nil {
		return contract.EvidencePack{}, contract.RetrievalTrace{}, fmt.Errorf("composer: stamp integrity: %w", err)
	}

	c.emitFootprint(ctx, prompt, intentVal, pack, time.Since(start), s4Out, s5Out, tm)
	return pack, trace, nil
}

// buildComposerTrace assembles a producer="composer" RetrievalTrace from the
// Stage-1 (ckv recall/rerank) and Stage-2 (ckg seed search) provenance. Each
// ckv recall round becomes a step; the final round carries the distilled
// keywords, confidence, and the union of ckv hits. A trailing ckg.bm25 step
// records the keyword search that seeds the graph stage.
//
// CKVCalls is exact (one ckv.SemanticSearch per Stage-1 round). CKGCalls is
// left zero pending backend-level call instrumentation — the structural
// signal (steps, seeds, keywords, rounds) is the present value.
func buildComposerTrace(prompt string, intentVal contract.Intent, s1 stage1.Stage1Output, s2 stage2.Stage2Output) contract.RetrievalTrace {
	steps := make([]contract.RetrievalStep, 0, len(s1.AugmentedQueries)+1)
	for i, q := range s1.AugmentedQueries {
		st := contract.RetrievalStep{
			N:        i + 1,
			Kind:     contract.StepCKVRecall,
			Query:    q,
			Source:   contract.HitSourceCKV,
			Decision: "augment",
		}
		if i == len(s1.AugmentedQueries)-1 {
			st.Keywords = s1.Keywords
			st.Confidence = s1.Confidence
			st.TopHits = s1.Hits
			st.Decision = "accept"
		}
		steps = append(steps, st)
	}
	steps = append(steps, contract.RetrievalStep{
		N:        len(steps) + 1,
		Kind:     contract.StepCKGBM25,
		Query:    strings.Join(s1.Keywords, " "),
		Source:   contract.HitSourceCKG,
		Keywords: s1.Keywords,
		TopHits:  s2.Hits,
		Decision: "expand",
	})

	seeds := make([]contract.Citation, 0, len(s2.Citations))
	for _, sc := range s2.Citations {
		seeds = append(seeds, sc.Citation)
	}

	return contract.RetrievalTrace{
		Producer:       "composer",
		Intent:         intentVal,
		Prompt:         prompt,
		VocabExpanded:  s1.VocabExpanded,
		VocabKeywords:  s1.VocabKeywords,
		Steps:          steps,
		FinalSeeds:     seeds,
		FailedKeywords: s2.FailedKeywords,
		Rounds:         s1.Rounds,
		CKVCalls:       s1.Rounds,
		CKGCalls:       0, // TODO(trace): exact ckg call count needs ckgclient instrumentation
	}
}

// stageTimings holds the wall-clock duration of each composer stage. Emitted
// per-stage in the footprint so the §9 validation spike can see where time
// goes (intent vs. ckg search vs. body fetch vs. sanitize).
type stageTimings struct {
	intent, stage1, stage2, stage3, stage4, stage5 time.Duration
}

// assemblePack builds the final EvidencePack from the per-stage outputs.
// Splitting out the assembly keeps Compose focused on flow control
// (each "if err return") and assembly focused on data shaping.
func assemblePack(
	prompt string,
	intentVal contract.Intent,
	s3Out stage3.Stage3Output,
	s4Out budget.Stage4Output,
	s5Out sanitize.Stage5Output,
	builderVersion string,
) contract.EvidencePack {
	// Build the (citations, bodies) pair from sanitized non-dropped
	// items. Maintain a citation key set so neighbor filtering can
	// check endpoint membership.
	citationKeys := make(map[string]struct{}, len(s5Out.Items))
	citations := make([]contract.Citation, 0, len(s5Out.Items))
	bodies := make([]contract.Body, 0, len(s5Out.Items))

	for _, item := range s5Out.Items {
		if item.Dropped {
			continue
		}
		citationKeys[item.Citation.Key()] = struct{}{}
		citations = append(citations, item.Citation)
		// Re-estimate tokens against the sanitized body (mask may have
		// changed length; pre-sanitize estimate from Stage 4 is stale).
		bodies = append(bodies, contract.Body{
			Citation:      item.Citation,
			Text:          item.Body,
			TokenEstimate: budget.EstimateTokens(item.Body),
		})
	}

	// Filter graph neighbors to those whose Source and Target both
	// appear in the final citation set. contract.EvidencePack.IsValid
	// requires this; any orphan neighbor would invalidate the pack.
	validNeighbors := make([]contract.Neighbor, 0, len(s3Out.Neighbors))
	for _, sn := range s3Out.Neighbors {
		if _, ok := citationKeys[sn.Edge.Source.Key()]; !ok {
			continue
		}
		if _, ok := citationKeys[sn.Edge.Target.Key()]; !ok {
			continue
		}
		validNeighbors = append(validNeighbors, sn.Edge)
	}

	usedTokens := 0
	for _, b := range bodies {
		usedTokens += b.TokenEstimate
	}
	var utilization float64
	if s4Out.BudgetTokens > 0 {
		utilization = float64(usedTokens) / float64(s4Out.BudgetTokens)
	}

	return contract.EvidencePack{
		Intent:         intentVal,
		Query:          prompt,
		Citations:      citations,
		Bodies:         bodies,
		GraphNeighbors: validNeighbors,
		SanitizeReport: s5Out.Redactions,
		Metadata: contract.PackMetadata{
			BudgetTokens:     s4Out.BudgetTokens,
			UsedTokens:       usedTokens,
			UtilizationRatio: utilization,
			BuiltAt:          time.Now().UTC(),
			BuilderVersion:   builderVersion,
		},
	}
}

func (c *Composer) emitFootprint(
	ctx context.Context,
	prompt string,
	intentVal contract.Intent,
	pack contract.EvidencePack,
	elapsed time.Duration,
	s4 budget.Stage4Output,
	s5 sanitize.Stage5Output,
	tm stageTimings,
) {
	if c.fp == nil {
		return
	}
	c.fp.Event(ctx, "composer.compose_complete",
		zap.String("intent", string(intentVal)),
		zap.Int("prompt_runes", len([]rune(prompt))),
		zap.Int("citation_count", len(pack.Citations)),
		zap.Int("body_count", len(pack.Bodies)),
		zap.Int("neighbor_count", len(pack.GraphNeighbors)),
		zap.Int("redaction_count", len(pack.SanitizeReport)),
		zap.Int("used_tokens", pack.Metadata.UsedTokens),
		zap.Int("budget_tokens", pack.Metadata.BudgetTokens),
		zap.Float64("utilization", pack.Metadata.UtilizationRatio),
		zap.Int("dropped_count", countDropped(s5)),
		zap.Int("budget_skipped", len(s4.Skipped)),
		zap.Duration("elapsed", elapsed),
		// Per-stage breakdown (00 §9 cost attribution).
		zap.Duration("intent_ms", tm.intent),
		zap.Duration("stage1_ms", tm.stage1),
		zap.Duration("stage2_ms", tm.stage2),
		zap.Duration("stage3_ms", tm.stage3),
		zap.Duration("stage4_ms", tm.stage4),
		zap.Duration("stage5_ms", tm.stage5),
	)
}

func countDropped(s5 sanitize.Stage5Output) int {
	n := 0
	for _, it := range s5.Items {
		if it.Dropped {
			n++
		}
	}
	return n
}
