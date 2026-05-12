package contract

import "slices"

// Intent is a coarse classification of a vibe prompt that drives the cks
// composer pipeline. Each Intent's doc comment names:
//
//   - Trigger    : example user phrasings that classify to this Intent.
//   - Stage 1    : work to perform with ckv + BM25 (semantic stage) —
//     derive concrete keywords/symbols from the natural-
//     language prompt.
//   - Stage 2    : work to perform with ckg + BM25 (precise stage) —
//     use the Stage-1 keywords to look up exact code locations
//     and traverse the appropriate graph relations.
//   - Sanitize   : which sanitize-rule subset is escalated for this Intent.
//     Most Intents inherit the base ruleset; Security raises it.
//
// Pipeline model (not fan-out): the composer is a two-stage funnel, not a
// parallel fan-out followed by RRF fusion. Stage 1 (ckv) infers meaning,
// Stage 2 (ckg) finds exact code. The two backends have distinct, sequential
// roles; they do not compete on the same task.
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
	// or input is empty. Composer falls back to a broad keyword search with
	// the base sanitize ruleset.
	IntentUnknown Intent = ""

	// IntentBugFix — "this function returns the wrong value", "X crashes on
	// input Y", "Z is intermittently failing".
	//
	// Stage 1 (ckv): extract error/log keywords, return-value patterns,
	//   panic/exception messages, and the symbol name in question.
	// Stage 2 (ckg): BM25 the extracted keywords against logs/error sites,
	//   then ckg find_callers from the target symbol to surface upstream
	//   callers that may trigger the bug.
	// Sanitize: base ruleset.
	IntentBugFix Intent = "bug_fix"

	// IntentFeatureAdd — "add support for X", "implement Y end-to-end".
	//
	// Stage 1 (ckv): extract domain nouns/verbs and any similar existing
	//   feature names (so the composer can show how analogous features
	//   were built).
	// Stage 2 (ckg): locate type/interface definitions matching the
	//   keywords, plus the package boundaries the new feature must respect.
	// Sanitize: base ruleset.
	IntentFeatureAdd Intent = "feature_add"

	// IntentRefactor — "clean up X", "improve performance of Y", "extract
	// helper from Z".
	//
	// Stage 1 (ckv): extract the target symbol name and any pattern
	//   keywords (e.g. "duplication", "long function") implied by the
	//   prompt.
	// Stage 2 (ckg): every usage of the target symbol (find_callers and
	//   find_references) plus structurally similar patterns elsewhere.
	// Sanitize: base ruleset (broader body release is acceptable because
	//   refactor targets are internal code).
	IntentRefactor Intent = "refactor"

	// IntentArchExplain — "how does X work", "explain the flow of Y",
	// "what calls this".
	//
	// Stage 1 (ckv): extract component/package nouns and high-level
	//   keywords from the prompt.
	// Stage 2 (ckg): package-level types, exported symbol signatures, doc
	//   comments, and the root of the call graph. Avoid drilling into
	//   hot-path implementation bodies for arch-explain prompts.
	// Sanitize: base ruleset.
	IntentArchExplain Intent = "arch_explain"

	// IntentTestAdd — "add tests for X", "cover Y", "table-driven tests
	// for Z".
	//
	// Stage 1 (ckv): extract the target function/method name and infer
	//   test-style keywords (table-driven, mock, golden file) from the
	//   prompt.
	// Stage 2 (ckg): the target symbol plus the same-package *_test.go
	//   files (tested_by relation) so the composer learns existing test
	//   patterns and helpers.
	// Sanitize: base ruleset.
	IntentTestAdd Intent = "test_add"

	// IntentConcurrencySafety — "is X safe under concurrent calls", "race
	// in Y", "data race on Z".
	//
	// Stage 1 (ckv): extract concurrency-primitive keywords (mutex,
	//   channel, sync, atomic, goroutine, select) and the target symbol.
	// Stage 2 (ckg): all sites using those primitives in the symbol's
	//   neighborhood, plus shared-state access patterns and lock-ordering
	//   documentation.
	// Sanitize: base ruleset.
	IntentConcurrencySafety Intent = "concurrency_safety"

	// IntentSecurity — "check X for vulnerabilities", "audit Y", "is Z
	// safe against untrusted input".
	//
	// Stage 1 (ckv): extract input-boundary keywords (HTTP handler, RPC
	//   handler, file/network read), validation keywords, and crypto
	//   primitives.
	// Stage 2 (ckg): input-receiving functions, the validation layers
	//   they call, and any crypto usage. Cross-reference with PII/secret
	//   handling sites.
	// Sanitize: strictest — secret patterns are NEVER masked (mask is
	//   forbidden for this intent). Use drop or fail_closed.
	IntentSecurity Intent = "security"

	// IntentDocsUpdate — "update docs for X", "explain Y in README".
	//
	// Stage 1 (ckv): extract package and exported-symbol names.
	// Stage 2 (ckg): existing godoc comments, README, and exported symbol
	//   signatures. Implementation bodies are rarely needed.
	// Sanitize: base ruleset.
	IntentDocsUpdate Intent = "docs_update"

	// IntentQAReview — "review this PR", "check code standards on X",
	// "pre-merge review". Mirrors the .claude/skills/pr-review pattern in
	// go-stablenet, where a PR diff is checked against project-specific
	// review rules.
	//
	// Stage 1 (ckv): extract the review scope (PR diff, file list, or
	//   directory) and any project-specific review-rule keywords from the
	//   prompt.
	// Stage 2 (ckg): the scope symbols, their callers (impact analysis),
	//   plus project-skill docs (e.g., .claude/docs/REVIEW_GUIDE.md,
	//   CLAUDE_DEV_GUIDE.md) loaded by the project skill loader.
	// Sanitize: base ruleset (review needs full body context).
	IntentQAReview Intent = "qa_review"
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
	IntentQAReview,
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
