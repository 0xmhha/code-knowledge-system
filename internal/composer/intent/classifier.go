package intent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// DefaultUnknownThreshold is the cosine-similarity cutoff below which
// Classify returns IntentUnknown. 0.6 fits cks's "when in doubt, prefer
// IntentUnknown (composer falls back to broad fan-out)" policy — modern
// multilingual embedders typically map paraphrased same-meaning texts at
// 0.7+ cosine, so 0.6 rejects loosely-related prompts while accepting
// natural variation. Phase E tuning will revisit with real-prompt data.
const DefaultUnknownThreshold = 0.6

// Classification is the result of one Classify call.
type Classification struct {
	// Intent is the chosen Intent, or IntentUnknown when no anchor is
	// close enough.
	Intent contract.Intent
	// Confidence is the cosine similarity to the winning anchor
	// (1.0 = identical direction, 0 = orthogonal). For IntentUnknown
	// results this is the best similarity observed (still below the
	// threshold), useful for debugging.
	Confidence float64
	// BestAnchor is the text of the anchor that won. Empty when Intent
	// is IntentUnknown.
	BestAnchor string
}

// Option configures a Classifier.
type Option func(*Classifier)

// WithFootprint attaches a footprint.Logger; Classify emits a
// composer.intent_classified event on each call. Nil-safe.
func WithFootprint(fp *footprint.Logger) Option {
	return func(c *Classifier) { c.fp = fp }
}

// WithUnknownThreshold overrides the default cosine cutoff that maps
// "no close anchor" to IntentUnknown.
func WithUnknownThreshold(t float64) Option {
	return func(c *Classifier) { c.unknownThreshold = t }
}

// WithAnchors overrides the default anchor map. Useful for tests and for
// project-specific anchor injection (Phase D.1 project skill loader can
// add domain anchors).
func WithAnchors(anchors map[contract.Intent][]string) Option {
	return func(c *Classifier) { c.anchors = anchors }
}

// Classifier maps a vibe prompt to an Intent by comparing the prompt's
// embedding to a set of per-Intent anchor embeddings.
type Classifier struct {
	embedder         Embedder
	fp               *footprint.Logger
	anchors          map[contract.Intent][]string // raw anchors, set via option or default
	embedded         map[contract.Intent][]anchor // embedded at New() time
	unknownThreshold float64
}

type anchor struct {
	Text string
	Vec  []float32
}

// New constructs a Classifier and eagerly embeds every configured anchor.
// Failures during anchor embedding abort construction so callers see the
// problem at startup, not on the first user prompt.
func New(ctx context.Context, embedder Embedder, opts ...Option) (*Classifier, error) {
	if embedder == nil {
		return nil, errors.New("intent: nil embedder")
	}
	c := &Classifier{
		embedder:         embedder,
		anchors:          defaultAnchors,
		unknownThreshold: DefaultUnknownThreshold,
	}
	for _, opt := range opts {
		opt(c)
	}

	if err := validateAnchors(c.anchors); err != nil {
		return nil, err
	}

	c.embedded = make(map[contract.Intent][]anchor, len(c.anchors))
	for intentVal, texts := range c.anchors {
		for _, text := range texts {
			vec, err := embedder.Embed(ctx, text)
			if err != nil {
				return nil, fmt.Errorf("intent: pre-embed anchor %q for %s: %w", text, intentVal, err)
			}
			c.embedded[intentVal] = append(c.embedded[intentVal], anchor{Text: text, Vec: vec})
		}
	}
	return c, nil
}

// validateAnchors rejects anchor maps with structural problems before any
// embedder call is made:
//   - unknown contract.Intent values (typos, deleted enum members),
//   - empty anchor list for a declared Intent (would never match),
//   - empty / whitespace-only anchor text (would embed to noise).
//
// Runs before pre-embedding so failures surface without burning embedder
// quota on bad input.
func validateAnchors(m map[contract.Intent][]string) error {
	for intentVal, texts := range m {
		if !intentVal.IsValid() {
			return fmt.Errorf("intent: anchor map contains unknown Intent %q", intentVal)
		}
		if len(texts) == 0 {
			return fmt.Errorf("intent: anchor list for %s is empty", intentVal)
		}
		for i, t := range texts {
			if strings.TrimSpace(t) == "" {
				return fmt.Errorf("intent: anchor text for %s[%d] is empty or whitespace", intentVal, i)
			}
		}
	}
	return nil
}

// Classify embeds prompt and returns the Intent whose closest anchor has
// the highest cosine similarity, or IntentUnknown when the best similarity
// is below the threshold.
//
// Errors: empty prompt, embedder failure. Composer callers should fall
// back to IntentUnknown handling (broad fan-out) on error rather than
// aborting the whole pipeline.
func (c *Classifier) Classify(ctx context.Context, prompt string) (Classification, error) {
	if prompt == "" {
		return Classification{Intent: contract.IntentUnknown}, errors.New("intent: empty prompt")
	}

	promptVec, err := c.embedder.Embed(ctx, prompt)
	if err != nil {
		return Classification{Intent: contract.IntentUnknown}, fmt.Errorf("intent: embed prompt: %w", err)
	}

	bestIntent := contract.IntentUnknown
	bestSim := math.Inf(-1) // start at -inf so any real cosine wins on first pass
	bestAnchor := ""

	for intentVal, anchors := range c.embedded {
		for _, a := range anchors {
			sim := cosine(promptVec, a.Vec)
			if sim > bestSim {
				bestSim = sim
				bestIntent = intentVal
				bestAnchor = a.Text
			}
		}
	}

	// Normalize the "no anchors at all" edge case (would leave -inf).
	if math.IsInf(bestSim, -1) {
		bestSim = 0
	}

	below := bestSim < c.unknownThreshold
	result := Classification{
		Intent:     bestIntent,
		Confidence: bestSim,
		BestAnchor: bestAnchor,
	}
	if below {
		result.Intent = contract.IntentUnknown
		result.BestAnchor = ""
	}

	c.emitFootprint(ctx, result, below)
	return result, nil
}

func (c *Classifier) emitFootprint(ctx context.Context, r Classification, below bool) {
	if c.fp == nil {
		return
	}
	c.fp.Event(ctx, "composer.intent_classified",
		zap.String("intent", string(r.Intent)),
		zap.Float64("confidence", r.Confidence),
		zap.String("best_anchor", r.BestAnchor),
		zap.Bool("below_threshold", below),
	)
}

// cosine returns the cosine similarity of two equal-length vectors, or 0
// when either is empty, length-mismatched, or zero-magnitude.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		magA += ai * ai
		magB += bi * bi
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
