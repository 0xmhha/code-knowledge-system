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

	promptIDs := extractIdentifiers(prompt)

	// Compound identifier candidates FIRST: Go symbols are overwhelmingly
	// multi-word camelCase/PascalCase (QuorumSize, broadcastRoundChange,
	// handlePrepareMsg), but a natural-language prompt spells those
	// concepts as separate words ("quorum size", "broadcast a round
	// change message"). Stage 2's BM25/FindSymbol can only match the
	// joined identifier, so we synthesize joined candidates from adjacent
	// prompt tokens. They are emitted ahead of the bare single words on
	// purpose: ckg normalizes every query's top BM25 hit to 1.0, so the
	// Stage-1 rerank score saturates and ties are broken by candidate
	// order. Putting the more-specific compound ("QuorumSize") ahead of
	// the generic single ("quorum", which also matches an unrelated local
	// var and a struct field) ensures the compound survives the
	// MaxKeywords cap. Over-generation is safe: joins that don't hit real
	// code are zero-scored and dropped by the rerank.
	for _, c := range compoundCandidates(promptIDs) {
		add(c)
	}

	for _, id := range promptIDs {
		add(id)
	}

	for _, h := range hits {
		// Prefer the chunk's actual symbol when ckv populated it
		// (e.g. "Finalize", "QuorumSize") — this is the only keyword
		// that can disambiguate same-named identifiers across packages
		// (eight different `Finalize` methods exist in go-stablenet).
		// Falls back to the file-basename heuristic for hits without
		// symbol metadata (older ckv builds, doc/header chunks, ckg-
		// sourced hits, etc.) so retrieval quality on those is unchanged.
		if h.Symbol != "" {
			add(h.Symbol)
			continue
		}
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

// maxCompoundTokens bounds how many leading prompt identifiers feed the
// adjacent-window join. A handful covers real prompts ("supermajority
// threshold quorum size") while keeping the extra BM25 rerank calls
// (one per generated candidate) bounded and latency predictable.
const maxCompoundTokens = 10

// compoundCandidates synthesizes joined identifier candidates from
// adjacent windows of the prompt's ordered identifier tokens.
//
// For the token run [..., "quorum", "size", ...] it emits the PascalCase
// "QuorumSize" and camelCase "quorumSize"; trigram windows recover
// three-part symbols like "broadcastRoundChange" from "broadcast round
// change". Both cases are emitted because Go distinguishes exported
// (PascalCase) from unexported (camelCase) symbols and the prompt gives
// no hint which the target is. Candidates are deduplicated and quality-
// filtered downstream by the BM25 rerank, so this only needs to be a
// superset of plausible joins.
func compoundCandidates(ids []string) []string {
	if len(ids) < 2 {
		return nil
	}
	if len(ids) > maxCompoundTokens {
		ids = ids[:maxCompoundTokens]
	}
	out := make([]string, 0, len(ids)*4)
	emit := func(parts []string) {
		out = append(out, joinCase(parts, true), joinCase(parts, false))
	}
	for i := 0; i < len(ids)-1; i++ {
		emit(ids[i : i+2]) // adjacent bigram
		if i < len(ids)-2 {
			emit(ids[i : i+3]) // adjacent trigram
		}
	}
	return out
}

// joinCase joins parts into a single identifier. With pascal=true the
// result is PascalCase (every part capitalized); otherwise camelCase
// (first part lowercased, rest capitalized).
func joinCase(parts []string, pascal bool) string {
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 && !pascal {
			b.WriteString(strings.ToLower(p[:1]) + p[1:])
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
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
