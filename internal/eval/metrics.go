// Package eval is the cks retrieval-quality evaluation harness.
//
// Phase E (Slim, Layer 1): measures the quality of the EvidencePack
// cks-mcp returns for a vibe prompt by comparing the citations
// against a YAML-declared ground-truth set. No LLM is invoked; the
// metrics here cover the cks side of the pipeline only.
//
// Files:
//   - metrics.go   pure citation-set comparison (precision/recall/f1,
//                  match modes, median over runs)
//   - scenario.go  YAML schema + loader
//   - runner.go    Runner: scenario → MCP get_for_task → metrics
//   - report.go    JSON report serialization
package eval

import (
	"sort"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// MatchMode controls how an expected citation is matched against an
// actual one. The default is MatchOverlap because "found related code"
// is more honest to a partial line-range overlap than to byte-perfect
// equality.
type MatchMode string

const (
	// MatchOverlap: same file AND the line ranges overlap by at
	// least one line. This is the recommended default.
	MatchOverlap MatchMode = "overlap"

	// MatchStrict: same file AND identical line ranges.
	MatchStrict MatchMode = "strict"
)

// IsValid reports whether m is a known match mode.
func (m MatchMode) IsValid() bool {
	switch m {
	case MatchOverlap, MatchStrict:
		return true
	}
	return false
}

// matchOverlap reports whether actual covers at least one line of
// expected within the same file.
//
// Boundary handling: ranges [a,b] and [c,d] overlap iff a <= d AND
// c <= b. Touching ranges (b == c) overlap by one shared line — this
// is deliberate, matching how editors and reviewers think of "lines
// covered by this hunk."
func matchOverlap(expected, actual contract.Citation) bool {
	if expected.File != actual.File {
		return false
	}
	return expected.StartLine <= actual.EndLine && actual.StartLine <= expected.EndLine
}

// matchStrict reports whether actual equals expected on file and
// exact line range.
func matchStrict(expected, actual contract.Citation) bool {
	return expected.File == actual.File &&
		expected.StartLine == actual.StartLine &&
		expected.EndLine == actual.EndLine
}

// matcher selects a match function for the given mode. Returns
// matchOverlap as the safe default for unknown modes (Scenario.Validate
// rejects unknown modes upstream; this is belt-and-suspenders).
func matcher(mode MatchMode) func(a, b contract.Citation) bool {
	if mode == MatchStrict {
		return matchStrict
	}
	return matchOverlap
}

// precisionRecall computes (precision, recall, f1) of actual against
// expected under the given match mode.
//
// Semantics for edge cases:
//   - Empty actual: precision = recall = f1 = 0 (no retrieval attempt).
//   - Empty expected: precision = recall = 1.0 — there is no
//     ground-truth to disprove. F1 follows. This is the "trivially
//     correct" interpretation; scenarios without expected citations
//     are not useful for retrieval scoring and the runner can flag
//     them separately.
//   - Duplicate actuals: collapsed by Citation.Key() so a backend that
//     returns the same code location twice cannot inflate metrics.
//
// Computation: for each expected citation, mark it "found" iff any
// (deduplicated) actual citation matches it under the mode; for each
// actual, mark it "correct" iff it matches at least one expected.
// Counts are then folded into the standard P/R/F1 formulas.
func precisionRecall(expected, actual []contract.Citation, mode MatchMode) (precision, recall, f1 float64) {
	// Dedup actual by Citation.Key(); preserve first-seen order so
	// downstream debug output stays predictable.
	seen := make(map[string]struct{}, len(actual))
	dedup := make([]contract.Citation, 0, len(actual))
	for _, c := range actual {
		k := c.Key()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		dedup = append(dedup, c)
	}

	if len(dedup) == 0 {
		// Empty retrieval: every expected is missed.
		return 0, 0, 0
	}
	if len(expected) == 0 {
		// No ground truth: nothing we returned can be disproved.
		// Recall is trivially 1.0 (zero misses); precision likewise.
		return 1.0, 1.0, 1.0
	}

	m := matcher(mode)

	hitExpected := 0
	for _, e := range expected {
		for _, a := range dedup {
			if m(e, a) {
				hitExpected++
				break
			}
		}
	}
	hitActual := 0
	for _, a := range dedup {
		for _, e := range expected {
			if m(e, a) {
				hitActual++
				break
			}
		}
	}

	precision = float64(hitActual) / float64(len(dedup))
	recall = float64(hitExpected) / float64(len(expected))
	if precision == 0 && recall == 0 {
		f1 = 0
	} else {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return precision, recall, f1
}

// median returns the median of xs without mutating the input.
// Returns 0 for an empty slice (callers treat 0 as "no data" and
// surface that distinctly).
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := make([]float64, len(xs))
	copy(sorted, xs)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// percentile returns the nearest-rank percentile of xs (0 < p <= 1)
// without mutating the input.
//
// Nearest-rank (NIST method): the value at sorted index ceil(p*N)
// (1-based), clamped to N. This is the simplest definition that
// agrees with intuition on small samples — p50 of a 10-element
// sorted slice is the 5th value, p95 is the 10th, p100 is the max.
// Interpolated percentiles (e.g. linear, R-style "type 7") give
// fractionally-different answers on small N and add complexity
// without changing the cks story; cks-eval typically runs N in
// the single digits.
//
// Edge handling:
//   - empty xs: returns 0 (same convention as median).
//   - p <= 0: returns the min.
//   - p >= 1: returns the max.
//
// Used by Runner to compute LatencyMSP50 / LatencyMSP95 / LatencyMSMax.
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := make([]float64, len(xs))
	copy(sorted, xs)
	sort.Float64s(sorted)
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	// Nearest-rank: idx = ceil(p * N), then clamp 1..N, then 0-based.
	idx := int(p*float64(len(sorted)) + 0.999999999) // ceil via float trick
	if idx < 1 {
		idx = 1
	}
	if idx > len(sorted) {
		idx = len(sorted)
	}
	return sorted[idx-1]
}
