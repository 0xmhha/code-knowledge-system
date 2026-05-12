package contract

import "time"

// RedactionAction names the disposition of a sanitize-rule hit.
type RedactionAction string

const (
	// RedactionMask replaces matched text with a placeholder (e.g. "***")
	// before the EvidencePack is released. The citation remains intact.
	RedactionMask RedactionAction = "mask"

	// RedactionDrop removes the offending Body entirely. Its Citation may
	// remain (so downstream knows a citation existed) or be dropped per
	// the rule configuration.
	RedactionDrop RedactionAction = "drop"

	// RedactionFailClosed refuses to release the pack at all. The composer
	// returns an error to the caller; no partial content is exposed.
	RedactionFailClosed RedactionAction = "fail_closed"
)

// Redaction records one sanitize-rule hit. Used in EvidencePack.SanitizeReport.
type Redaction struct {
	RuleID  string          `json:"rule_id"`
	Path    string          `json:"path,omitempty"` // JSON-pointer-ish location within the pack
	Action  RedactionAction `json:"action"`
	Excerpt string          `json:"excerpt,omitempty"` // safe-to-log excerpt; never raw secret
}

// Body is the code text for a Citation. Held separately from Hits so that
// many Hits can reference the same Body without duplication.
type Body struct {
	Citation Citation `json:"citation"`
	Text     string   `json:"text"`
	// TokenEstimate is the composer's approximate token cost of Text;
	// used by the budget manager. Zero when not yet computed.
	TokenEstimate int `json:"token_estimate,omitempty"`
}

// PackMetadata carries provenance and budgeting state for an EvidencePack.
type PackMetadata struct {
	BudgetTokens     int       `json:"budget_tokens"`
	UsedTokens       int       `json:"used_tokens"`
	UtilizationRatio float64   `json:"utilization_ratio,omitempty"`
	BuiltAt          time.Time `json:"built_at"`
	BuilderVersion   string    `json:"builder_version,omitempty"`
	// CKGSchemaVersion and CKVStatsHash are opaque pin values that an
	// evaluation harness can compare across runs to confirm the same
	// index snapshot was used. Empty when the backend did not supply them.
	CKGSchemaVersion string `json:"ckg_schema_version,omitempty"`
	CKVStatsHash     string `json:"ckv_stats_hash,omitempty"`
}

// EvidencePack is the cks output unit: an intent-classified, token-budgeted,
// sanitized bundle of citations and code bodies that an upper-layer LLM
// client can consume directly.
//
// Phase-0 scope omits graph-neighbor edges; those land in Phase B.5 when
// the composer's expander module is wired up.
type EvidencePack struct {
	Intent         Intent       `json:"intent,omitempty"`
	Query          string       `json:"query"`
	Citations      []Citation   `json:"citations"`
	Bodies         []Body       `json:"bodies,omitempty"`
	SanitizeReport []Redaction  `json:"sanitize_report,omitempty"`
	Metadata       PackMetadata `json:"metadata"`
}

// IsValid reports whether p is structurally sound:
//   - non-empty Query
//   - every Citation valid
//   - every Body's Citation valid and present in Citations
//   - SanitizeReport actions are recognized
//
// Token-budget validity (UsedTokens <= BudgetTokens) is checked separately
// by the composer; IsValid is intentionally permissive on budgeting because
// over-budget packs are still useful for debugging.
func (p EvidencePack) IsValid() bool {
	if p.Query == "" {
		return false
	}
	if !p.Intent.IsValid() {
		return false
	}

	citationKeys := make(map[string]struct{}, len(p.Citations))
	for _, c := range p.Citations {
		if !c.IsValid() {
			return false
		}
		citationKeys[c.Key()] = struct{}{}
	}
	for _, b := range p.Bodies {
		if !b.Citation.IsValid() {
			return false
		}
		if _, ok := citationKeys[b.Citation.Key()]; !ok {
			return false
		}
	}
	for _, r := range p.SanitizeReport {
		switch r.Action {
		case RedactionMask, RedactionDrop, RedactionFailClosed:
		default:
			return false
		}
	}
	return true
}
