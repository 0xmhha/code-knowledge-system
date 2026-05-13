package budget

import "unicode/utf8"

// charsPerToken is the rough character-to-token ratio used to estimate
// token cost without a real tokenizer. 4 is a common heuristic for
// BPE-style tokenizers on Latin text; it slightly under-estimates for
// CJK (where modern tokenizers often use 1-2 chars per token) but
// over-estimates would be worse (we'd stuff the budget too cautiously
// and underuse capacity).
//
// PHASE E REQUIREMENT (not optional): a real tokenizer must replace
// this heuristic. Search quality is highly sensitive to token-count
// accuracy — under-estimating leads to LLM-side truncation, over-
// estimating leaves budget on the table. The EstimateTokens signature
// is the integration point; the body of this function is the only
// thing that should change.
const charsPerToken = 4

// EstimateTokens returns an approximate token count for text using
// UTF-8 rune count (not byte count) divided by charsPerToken. Rune
// counting matters for multilingual content: a Korean prompt's byte
// count is ~3x its character count, and tokenizer cost tracks
// characters not bytes.
//
// Returns 0 for empty input.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return utf8.RuneCountInString(text) / charsPerToken
}
