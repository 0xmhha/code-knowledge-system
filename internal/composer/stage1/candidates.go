package stage1

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// extractKeywords builds the candidate keyword list for ckg BM25 rerank.
//
// Sources, in priority order:
//  1. Identifiers parsed from the prompt itself (CamelCase, PascalCase,
//     snake_case). These are the user's explicit references —
//     "fix Login function", "race condition in goroutinePool".
//  2. File basenames from ckv hits. These represent what the semantic
//     search judged relevant; their names are often the package or
//     module the answer lives in.
//
// Duplicates are removed (case-sensitive on the assumption that Go's
// case-significant identifier rules apply). Short tokens (<=2 chars) are
// dropped — they're either noise ("a", "of") or single-letter Go vars
// that BM25 will rerank near zero anyway.
//
// Intent is deliberately ignored here. Stage 1 stays intent-agnostic
// because the differentiation belongs downstream, where it actually
// changes behavior:
//
//   - B.4 ckg searcher uses Intent to pick SymbolKind filters.
//   - B.5 graph expander uses Intent to pick Relation sets (BugFix ->
//     callers, ArchExplain -> imports, Security -> input boundaries).
//   - B.7 sanitize uses Intent to pick rule severity (Security stricter).
//
// The intent parameter is kept on the signature so Phase E can measure
// whether intent-aware extraction (e.g., forcing a "test" candidate for
// IntentTestAdd) improves recall enough to justify the added complexity.
// Until that data exists, splitting candidate extraction by Intent would
// be premature: the BM25 rerank already filters Intent-irrelevant
// keywords by giving them zero scores.
func extractKeywords(prompt string, hits []contract.Hit, intent contract.Intent) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)

	add := func(s string) {
		if len(s) <= 2 {
			return
		}
		if _, dup := seen[s]; dup {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	for _, id := range extractIdentifiers(prompt) {
		add(id)
	}

	for _, h := range hits {
		base := filepath.Base(h.Citation.File)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		// Strip Go test suffix so a hit on "foo_test.go" becomes the
		// keyword "foo" — the production code the test exercises.
		name = strings.TrimSuffix(name, "_test")
		add(name)
	}

	_ = intent // see doc: Stage 1 is intent-agnostic by design
	return out
}

// identifierRE matches conventional code identifiers (3+ chars):
//
//   - CamelCase / PascalCase: ProcessRequest, HandleSignup
//   - snake_case: process_request
//   - SCREAMING_SNAKE: PUBLIC_KEY (will match parts; that's fine,
//     BM25 reranks)
//   - Dotted paths split into pieces (cleanup below)
//
// We deliberately accept some over-extraction; downstream BM25 zero-scores
// keywords that don't hit real code, so noise is filtered automatically.
var identifierRE = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]{2,}`)

// extractIdentifiers returns identifier-shaped tokens from text. The result
// preserves source order so the first identifier the user typed appears
// first in the candidate list (small but useful signal for tie-breaking
// after rerank).
func extractIdentifiers(text string) []string {
	matches := identifierRE.FindAllString(text, -1)
	if matches == nil {
		return nil
	}
	// Filter the most common English stopwords that pass the regex. Korean
	// stopwords don't match the regex (no Latin letters) so the list is
	// English-only by design.
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if isStopword(strings.ToLower(m)) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// stopwords contains common English filler words that pass identifierRE
// but rarely identify code. Kept short on purpose — anything else gets
// filtered by BM25 zero-score downstream.
var stopwords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "but": {}, "with": {},
	"this": {}, "that": {}, "from": {}, "into": {}, "have": {},
	"are": {}, "was": {}, "were": {}, "been": {}, "being": {},
	"what": {}, "when": {}, "where": {}, "which": {}, "who": {},
	"how": {}, "why": {}, "does": {}, "did": {}, "doing": {},
}

func isStopword(s string) bool {
	_, ok := stopwords[s]
	return ok
}
