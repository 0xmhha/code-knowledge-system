package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// sanitizeRulesetVersion is the only supported sanitization_rules.yaml
// schema version.
const sanitizeRulesetVersion = 1

// SanitizeSeverity records how serious a sanitize-rule hit is. Severity is
// advisory: it does not change RedactionAction directly, but downstream
// alerting (Phase 3 policy + audit) routes alerts by severity.
type SanitizeSeverity string

const (
	SeverityCritical SanitizeSeverity = "critical"
	SeverityHigh     SanitizeSeverity = "high"
	SeverityMedium   SanitizeSeverity = "medium"
	SeverityLow      SanitizeSeverity = "low"
)

// SanitizeRule is one entry in the SanitizeRuleset.
//
// Pattern is a Go regexp/syntax expression. Validation compiles it and
// stores the compiled form in `re`; callers obtain it via Regexp().
//
// Action must be one of contract.RedactionMask / Drop / FailClosed. The
// Phase-0 base ruleset does NOT use Mask (per the project policy that
// matched secrets must not cross the LLM boundary). Operators who choose
// to enable Mask for development must do so via a custom ruleset.
type SanitizeRule struct {
	ID          string                   `yaml:"id"`
	Description string                   `yaml:"description"`
	Pattern     string                   `yaml:"pattern"`
	Action      contract.RedactionAction `yaml:"action"`
	Severity    SanitizeSeverity         `yaml:"severity"`

	re *regexp.Regexp // populated by Validate; not serialized
}

// Regexp returns the compiled pattern. Returns nil if the rule has not
// passed through Validate yet.
func (r *SanitizeRule) Regexp() *regexp.Regexp { return r.re }

// SanitizeRuleset is the catalog of sanitize rules loaded from a YAML file.
type SanitizeRuleset struct {
	Version int            `yaml:"version"`
	Rules   []SanitizeRule `yaml:"rules"`
}

// LoadSanitizeRuleset reads a sanitization_rules.yaml file, validates it,
// and returns the parsed ruleset with each rule's regex pre-compiled.
func LoadSanitizeRuleset(path string) (*SanitizeRuleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sanitize: read %q: %w", path, err)
	}
	return LoadSanitizeRulesetBytes(data)
}

// LoadSanitizeRulesetBytes parses raw YAML bytes into a validated ruleset.
func LoadSanitizeRulesetBytes(data []byte) (*SanitizeRuleset, error) {
	var rs SanitizeRuleset
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&rs); err != nil {
		if errors.Is(err, ErrEmptyDocument) {
			return nil, err
		}
		return nil, fmt.Errorf("sanitize: decode: %w", err)
	}
	if err := rs.Validate(); err != nil {
		return nil, err
	}
	return &rs, nil
}

// Validate checks the ruleset for structural problems, compiles each rule's
// regex, and detects duplicate IDs. Mutates rs: each rule gains its
// compiled regex.
//
// Mutation is intentional — callers should never use a rule whose regex
// has not been compiled. Validate is idempotent and safe to call multiple
// times on the same ruleset.
func (rs *SanitizeRuleset) Validate() error {
	if rs == nil {
		return fmt.Errorf("sanitize: nil ruleset")
	}
	if rs.Version != sanitizeRulesetVersion {
		return fmt.Errorf("sanitize: ruleset version=%d, want %d", rs.Version, sanitizeRulesetVersion)
	}
	if len(rs.Rules) == 0 {
		return fmt.Errorf("sanitize: ruleset has no rules")
	}

	seen := make(map[string]struct{}, len(rs.Rules))
	for i := range rs.Rules {
		r := &rs.Rules[i]

		if strings.TrimSpace(r.ID) == "" {
			return fmt.Errorf("sanitize: rule[%d] missing id", i)
		}
		if _, dup := seen[r.ID]; dup {
			return fmt.Errorf("sanitize: duplicate rule id %q", r.ID)
		}
		seen[r.ID] = struct{}{}

		if r.Pattern == "" {
			return fmt.Errorf("sanitize: rule %q missing pattern", r.ID)
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return fmt.Errorf("sanitize: rule %q pattern compile: %w", r.ID, err)
		}
		r.re = re

		switch r.Action {
		case contract.RedactionMask, contract.RedactionDrop, contract.RedactionFailClosed:
		default:
			return fmt.Errorf("sanitize: rule %q unknown action %q (want mask|drop|fail_closed)", r.ID, r.Action)
		}
		switch r.Severity {
		case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow:
		default:
			return fmt.Errorf("sanitize: rule %q unknown severity %q (want critical|high|medium|low)", r.ID, r.Severity)
		}
	}
	return nil
}

// Lookup returns the rule with the given ID, or nil if absent. O(n) — the
// rule count is small (Phase-0 baseline = 8). Switch to a map if that
// changes materially.
func (rs *SanitizeRuleset) Lookup(id string) *SanitizeRule {
	if rs == nil {
		return nil
	}
	for i := range rs.Rules {
		if rs.Rules[i].ID == id {
			return &rs.Rules[i]
		}
	}
	return nil
}

// FailClosedRules returns the rules whose action is fail_closed. Composer
// uses this for fast-path checks: any fail-closed hit aborts pack release.
func (rs *SanitizeRuleset) FailClosedRules() []SanitizeRule {
	if rs == nil {
		return nil
	}
	out := make([]SanitizeRule, 0)
	for i := range rs.Rules {
		if rs.Rules[i].Action == contract.RedactionFailClosed {
			out = append(out, rs.Rules[i])
		}
	}
	return out
}
