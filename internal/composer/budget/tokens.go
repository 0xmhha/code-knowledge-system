package budget

import "unicode/utf8"

// charsPerToken is the rough character-to-token ratio used to estimate
// token cost without a real tokenizer. 4 is a common heuristic for
// BPE-style tokenizers on Latin text; it slightly under-estimates for
// CJK (where modern tokenizers often use 1-2 chars per token) but
// over-estimates would be worse (we'd stuff the budget too cautiously
// and underuse capacity).
//
// Phase E can swap in a real tokenizer (tiktoken-style, model-specific)
// behind the same EstimateTokens signature.
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
