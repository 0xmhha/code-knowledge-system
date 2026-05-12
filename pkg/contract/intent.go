package contract

import "slices"

// Intent is a coarse classification of a vibe prompt that drives composer
// fan-out (which backends to query, what budget split) and sanitization
// policy (some intents allow broader code-body release than others).
//
// The set below is the Phase-0 default classification. As cks runs against
// real workloads, intent distribution should be measured (see the
// composer.intent_classified footprint event in Phase B) and the set
// revised — new intents added when distinct routing patterns emerge,
// marginal intents merged.
//
// Backward compatibility: Intent values are persisted to footprint logs,
// audit records, and evaluation reports. Renaming a value is a breaking
// change. To deprecate one, keep the constant in place and emit a warning
// from any classifier that produces it.
type Intent string

const (
	// IntentUnknown is the safe default when the classifier cannot decide
	// or input is empty. Composer treats this as "broad fan-out, default
	// sanitization", trading precision for recall.
	IntentUnknown Intent = ""

	// IntentBugFix — "this function returns the wrong value", "X crashes
	// on input Y". Composer should favor BM25 hits on error/log paths and
	// ckg find_callers expansion.
	IntentBugFix Intent = "bug_fix"

	// IntentFeatureAdd — "add support for X", "implement Y". Composer
	// favors semantic search (concepts more than exact tokens) and
	// architectural neighbors.
	IntentFeatureAdd Intent = "feature_add"

	// IntentRefactor — "clean up X", "improve performance of Y". Composer
	// favors structural ckg query (find all usages) and may release
	// broader code bodies for context.
	IntentRefactor Intent = "refactor"

	// IntentArchExplain — "how does X work", "what is the flow of Y".
	// Composer favors high-level types, package docs, and call graph
	// roots; rarely needs hot-path implementation bodies.
	IntentArchExplain Intent = "arch_explain"

	// IntentTestAdd — "add tests for X", "cover Y". Composer favors
	// existing test files (table-driven patterns) and the symbols under
	// test plus their direct callers.
	IntentTestAdd Intent = "test_add"

	// IntentConcurrencySafety — "is X safe under concurrent calls",
	// "race in Y". Composer favors mutex/channel/goroutine code paths,
	// atomics, and lock-ordering documentation.
	IntentConcurrencySafety Intent = "concurrency_safety"

	// IntentSecurity — "check X for vulnerabilities", "audit Y".
	// Composer applies the strictest sanitization (no secrets in pack)
	// and favors input boundaries, validation layers, and crypto usage.
	IntentSecurity Intent = "security"

	// IntentDocsUpdate — "update docs for X", "explain Y in README".
	// Composer favors existing docs, godoc comments, and symbol surfaces;
	// rarely needs implementation bodies.
	IntentDocsUpdate Intent = "docs_update"
)

// allIntents lists every defined Intent in deterministic order. Used by
// IsValid and tests; do not export.
var allIntents = []Intent{
	IntentUnknown,
	IntentBugFix,
	IntentFeatureAdd,
	IntentRefactor,
	IntentArchExplain,
	IntentTestAdd,
	IntentConcurrencySafety,
	IntentSecurity,
	IntentDocsUpdate,
}

// String returns the wire form of i (the value of the constant).
func (i Intent) String() string { return string(i) }

// IsValid reports whether i is a known Intent constant (including the
// IntentUnknown empty string).
func (i Intent) IsValid() bool {
	return slices.Contains(allIntents, i)
}

// ParseIntent normalizes s to a known Intent. The lookup is exact (no case
// folding) to keep persisted values stable. Returns (IntentUnknown, false)
// for unrecognized input.
func ParseIntent(s string) (Intent, bool) {
	i := Intent(s)
	if i.IsValid() {
		return i, true
	}
	return IntentUnknown, false
}

// AllIntents returns a copy of the canonical Intent list. The order is
// stable and matches the constant declarations.
func AllIntents() []Intent {
	out := make([]Intent, len(allIntents))
	copy(out, allIntents)
	return out
}
