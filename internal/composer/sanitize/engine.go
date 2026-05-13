// Package sanitize implements the composer pipeline's Stage 5: apply the
// sanitize ruleset (policies/sanitization_rules.yaml) to body text and
// produce the redaction report that downstream EvidencePack assembly
// embeds in SanitizeReport.
//
// Rule action precedence (per body, in order):
//
//  1. fail_closed: ANY match aborts the entire sanitize pass. The pack
//     is refused before it can cross the LLM boundary. Implements the
//     "matched data MUST NOT cross the cks → caller → LLM boundary"
//     policy for the strictest rules (e.g., PRIVATE_KEY_PEM).
//  2. drop: ANY match clears the body and marks the item Dropped. Mask
//     rules are not evaluated on a dropped body. The Citation remains
//     in the output so the caller knows a citation existed; only the
//     text is suppressed.
//  3. mask: each match is replaced with MaskToken (default "***").
//     Phase-0 baseline ruleset uses no mask rules; operators can opt
//     in via a custom ruleset, accepting the documented LLM-exposure
//     trade-off (see contract.RedactionMask doc).
//
// Stage 5 takes a generic []Sanitizable input (not budget.SelectedItem)
// so B.8 wire-up can choose either sanitize-before-budget or
// sanitize-after-budget ordering.
package sanitize

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/config"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// DefaultMaskToken is the placeholder substituted for masked matches.
// Three asterisks is short, ASCII-only (no tokenizer surprises), and
// clearly non-secret to anyone reading the output.
const DefaultMaskToken = "***"

// Config tunes the engine's behavior.
type Config struct {
	// MaskToken is substituted for each match when a mask rule fires.
	// Empty string is rejected by New (an explicit MaskToken="" would
	// silently delete matched data with no replacement, which is
	// indistinguishable from drop and surprising).
	MaskToken string
}

// DefaultConfig returns the Phase-0 tuning baseline.
func DefaultConfig() Config {
	return Config{MaskToken: DefaultMaskToken}
}

// Sanitizable is one body to sanitize. Stage 5 deliberately takes a
// generic shape so it composes with either Stage 4 (budget.SelectedItem)
// or directly with Stage 3 output (citations + freshly-fetched bodies),
// depending on the wire-up order B.8 chooses.
type Sanitizable struct {
	Citation contract.Citation
	Body     string
}

// SanitizedItem is the per-input result of one sanitize pass.
type SanitizedItem struct {
	Citation contract.Citation
	// Body is the sanitized text. Empty when Dropped is true.
	Body string
	// Dropped is true when at least one drop rule matched this body.
	Dropped bool
	// Redactions records every rule×body match on this item. Same
	// rule matching N times counts as one Redaction entry with a
	// count in Excerpt.
	Redactions []contract.Redaction
}

// Stage5Output captures the result of one Sanitize call.
type Stage5Output struct {
	// Items mirrors the input order. When FailClosed is true, Items
	// may be truncated at the offending item.
	Items []SanitizedItem
	// Redactions is the flat union of all per-item redactions, ready
	// for direct insertion into EvidencePack.SanitizeReport.
	Redactions []contract.Redaction
	// FailClosed is true when a fail_closed rule matched anywhere.
	// The caller MUST NOT release the EvidencePack when this is set.
	FailClosed bool
	// FailClosedRule names the rule that triggered the abort. Empty
	// when FailClosed is false.
	FailClosedRule string
}

// Engine runs Stage 5 of the composer pipeline.
type Engine struct {
	ruleset *config.SanitizeRuleset
	fp      *footprint.Logger
	config  Config

	// Pre-partitioned rules for ordered evaluation. Populated in New
	// once so per-item passes don't re-filter the ruleset.
	failClosedRules []*config.SanitizeRule
	dropRules       []*config.SanitizeRule
	maskRules       []*config.SanitizeRule
}

// Option configures an Engine.
type Option func(*Engine)

// WithFootprint attaches a footprint.Logger.
func WithFootprint(fp *footprint.Logger) Option {
	return func(e *Engine) { e.fp = fp }
}

// WithConfig overrides the default tuning.
func WithConfig(cfg Config) Option {
	return func(e *Engine) { e.config = cfg }
}

// New constructs an Engine and pre-partitions the ruleset by action.
// Returns an error if ruleset is nil, MaskToken is empty, or any rule's
// regex has not been compiled (Validate must run on the ruleset before
// it reaches the engine).
func New(ruleset *config.SanitizeRuleset, opts ...Option) (*Engine, error) {
	if ruleset == nil {
		return nil, errors.New("sanitize: nil ruleset")
	}
	e := &Engine{
		ruleset: ruleset,
		config:  DefaultConfig(),
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.config.MaskToken == "" {
		return nil, errors.New("sanitize: empty MaskToken (use drop instead of silent deletion)")
	}

	// Partition by action and capture pointers so Regexp() (pointer
	// receiver) works without re-finding the rule by index.
	for i := range ruleset.Rules {
		r := &ruleset.Rules[i]
		if r.Regexp() == nil {
			return nil, fmt.Errorf("sanitize: rule %q has no compiled regex (call SanitizeRuleset.Validate first)", r.ID)
		}
		switch r.Action {
		case contract.RedactionFailClosed:
			e.failClosedRules = append(e.failClosedRules, r)
		case contract.RedactionDrop:
			e.dropRules = append(e.dropRules, r)
		case contract.RedactionMask:
			e.maskRules = append(e.maskRules, r)
		default:
			return nil, fmt.Errorf("sanitize: rule %q has unrecognized action %q", r.ID, r.Action)
		}
	}
	return e, nil
}

// Sanitize applies the ruleset to each item. See the package doc for
// rule action precedence.
//
// When a fail_closed rule matches anywhere, Sanitize returns immediately
// with FailClosed=true. The Items slice contains the items processed so
// far (with the offending item included and its Redactions populated).
func (e *Engine) Sanitize(ctx context.Context, items []Sanitizable) (Stage5Output, error) {
	out := Stage5Output{}

	for _, item := range items {
		sanitized := SanitizedItem{
			Citation: item.Citation,
			Body:     item.Body,
		}

		// 1. fail_closed rules — any match aborts the whole pass.
		for _, rule := range e.failClosedRules {
			matches := rule.Regexp().FindAllStringIndex(item.Body, -1)
			if len(matches) == 0 {
				continue
			}
			red := makeRedaction(rule, item.Citation, len(matches))
			sanitized.Redactions = append(sanitized.Redactions, red)
			out.Redactions = append(out.Redactions, red)
			out.FailClosed = true
			out.FailClosedRule = rule.ID
			out.Items = append(out.Items, sanitized)
			e.emitFootprint(ctx, out)
			return out, nil
		}

		// 2. drop rules — clear the body if any matches.
		for _, rule := range e.dropRules {
			matches := rule.Regexp().FindAllStringIndex(item.Body, -1)
			if len(matches) == 0 {
				continue
			}
			sanitized.Dropped = true
			sanitized.Body = "" // body MUST NOT cross the boundary
			red := makeRedaction(rule, item.Citation, len(matches))
			sanitized.Redactions = append(sanitized.Redactions, red)
			out.Redactions = append(out.Redactions, red)
			// Continue checking other drop rules to record every hit
			// (operators want to know all the trip-wires that fired).
		}

		// 3. mask rules — only if not dropped. Substring replacement.
		if !sanitized.Dropped {
			body := item.Body
			for _, rule := range e.maskRules {
				matches := rule.Regexp().FindAllStringIndex(body, -1)
				if len(matches) == 0 {
					continue
				}
				body = rule.Regexp().ReplaceAllString(body, e.config.MaskToken)
				red := makeRedaction(rule, item.Citation, len(matches))
				sanitized.Redactions = append(sanitized.Redactions, red)
				out.Redactions = append(out.Redactions, red)
			}
			sanitized.Body = body
		}

		out.Items = append(out.Items, sanitized)
	}

	e.emitFootprint(ctx, out)
	return out, nil
}

// makeRedaction builds the Redaction record for one (rule, body) hit
// pair. Excerpt deliberately encodes match count and rule id only —
// never the matched bytes. Anything secret in the body must NOT round-
// trip through the redaction report itself.
func makeRedaction(rule *config.SanitizeRule, c contract.Citation, count int) contract.Redaction {
	return contract.Redaction{
		RuleID:  rule.ID,
		Path:    c.String(),
		Action:  rule.Action,
		Excerpt: matchExcerpt(rule.ID, count),
	}
}

func matchExcerpt(ruleID string, count int) string {
	if count == 1 {
		return fmt.Sprintf("%s matched 1 occurrence", ruleID)
	}
	return fmt.Sprintf("%s matched %d occurrences", ruleID, count)
}

func (e *Engine) emitFootprint(ctx context.Context, out Stage5Output) {
	if e.fp == nil {
		return
	}
	droppedCount := 0
	for _, it := range out.Items {
		if it.Dropped {
			droppedCount++
		}
	}
	fields := []zap.Field{
		zap.Int("item_count", len(out.Items)),
		zap.Int("redaction_count", len(out.Redactions)),
		zap.Int("dropped_count", droppedCount),
		zap.Bool("fail_closed", out.FailClosed),
	}
	if out.FailClosed {
		fields = append(fields, zap.String("fail_closed_rule", out.FailClosedRule))
	}
	e.fp.Event(ctx, "composer.stage5_sanitized", fields...)
}
