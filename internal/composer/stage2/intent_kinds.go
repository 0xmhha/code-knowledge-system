package stage2

import "github.com/0xmhha/code-knowledge-system/pkg/contract"

// intentToKinds returns the SymbolKinds filter for ckg.FindSymbol given
// the user's Intent. Empty result means "any kind" — the safe default
// for Intents where filtering would over-constrain.
//
// The mapping is intentionally narrow: each Intent points at the symbol
// kinds the user is overwhelmingly likely to act on. BM25Search still
// runs unfiltered so conceptual matches survive even when the kind
// filter rules out the obvious symbol form. Phase E will measure
// whether tightening or loosening individual entries improves recall.
func intentToKinds(intent contract.Intent) []string {
	switch intent {
	case contract.IntentBugFix:
		// Bugs surface in callable code (the place the runtime executes).
		return []string{"function", "method"}
	case contract.IntentFeatureAdd:
		// New features add callable surface plus new type/interface
		// boundaries.
		return []string{"function", "method", "type", "interface"}
	case contract.IntentArchExplain:
		// "How does X work" applies to whatever the user named — function,
		// method, type, interface, or const. FindSymbol returns LOCATIONS,
		// not bodies, so including callable kinds does not pollute results
		// with hot-path implementations; it just ensures the definition
		// receives the SymbolBonus when the user asks about a function or
		// method (a common case: "how does HandleRequest work").
		// Excluded: var (locals are weak architecture signals).
		return []string{"type", "interface", "const", "function", "method"}
	case contract.IntentTestAdd:
		// Tests target callable units.
		return []string{"function", "method"}
	case contract.IntentConcurrencySafety:
		// Concurrency issues live in callable code paths (synchronization
		// is enacted in functions, not in type declarations).
		return []string{"function", "method"}
	case contract.IntentSecurity:
		// Security audits trace input boundaries (handlers, validators)
		// and trust boundaries (interfaces).
		return []string{"function", "method", "interface"}
	case contract.IntentDocsUpdate:
		// Documentation describes the API surface: types, interfaces,
		// and callable signatures.
		return []string{"type", "interface", "function", "method"}
	case contract.IntentRefactor, contract.IntentQAReview, contract.IntentUnknown:
		// No filter — refactor and review touch anything, and Unknown
		// must stay broad on purpose.
		return nil
	}
	return nil
}
