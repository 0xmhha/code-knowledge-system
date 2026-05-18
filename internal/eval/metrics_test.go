package eval

import (
	"math"
	"testing"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func cit(file string, start, end int) contract.Citation {
	return contract.Citation{File: file, StartLine: start, EndLine: end}
}

// --- matchOverlap ---

func TestMatchOverlap_SameFileLineOverlap(t *testing.T) {
	t.Parallel()
	// expected 10-30, actual 15-25 → overlap on lines 15-25.
	expected := cit("a.go", 10, 30)
	actual := cit("a.go", 15, 25)
	if !matchOverlap(expected, actual) {
		t.Error("ranges overlap; should match")
	}
}

func TestMatchOverlap_ExactRange(t *testing.T) {
	t.Parallel()
	c := cit("a.go", 10, 30)
	if !matchOverlap(c, c) {
		t.Error("identical citations must match")
	}
}

func TestMatchOverlap_TouchingRange(t *testing.T) {
	t.Parallel()
	// 10-30 and 30-40 share line 30 → overlap (1 line is enough).
	if !matchOverlap(cit("a.go", 10, 30), cit("a.go", 30, 40)) {
		t.Error("touching ranges should overlap")
	}
}

func TestMatchOverlap_NoLineOverlap(t *testing.T) {
	t.Parallel()
	if matchOverlap(cit("a.go", 10, 30), cit("a.go", 40, 50)) {
		t.Error("disjoint ranges must not match")
	}
}

func TestMatchOverlap_DifferentFiles(t *testing.T) {
	t.Parallel()
	if matchOverlap(cit("a.go", 10, 30), cit("b.go", 10, 30)) {
		t.Error("different files must not match even with same range")
	}
}

// --- matchStrict ---

func TestMatchStrict_ExactOnly(t *testing.T) {
	t.Parallel()
	c := cit("a.go", 10, 30)
	if !matchStrict(c, c) {
		t.Error("exact match should pass strict")
	}
	if matchStrict(c, cit("a.go", 11, 30)) {
		t.Error("off-by-one start fails strict")
	}
	if matchStrict(c, cit("a.go", 10, 29)) {
		t.Error("off-by-one end fails strict")
	}
}

// --- precisionRecall ---

func TestPrecisionRecall_AllExpectedReturned(t *testing.T) {
	t.Parallel()
	expected := []contract.Citation{
		cit("a.go", 1, 10),
		cit("b.go", 1, 10),
	}
	actual := []contract.Citation{
		cit("a.go", 1, 10),
		cit("b.go", 1, 10),
	}
	p, r, f := precisionRecall(expected, actual, MatchOverlap)
	if p != 1.0 || r != 1.0 || f != 1.0 {
		t.Errorf("P/R/F = %.2f/%.2f/%.2f, want 1.0/1.0/1.0", p, r, f)
	}
}

func TestPrecisionRecall_HalfRecall(t *testing.T) {
	t.Parallel()
	expected := []contract.Citation{
		cit("a.go", 1, 10),
		cit("b.go", 1, 10),
	}
	actual := []contract.Citation{
		cit("a.go", 1, 10),
		cit("c.go", 1, 10), // not in expected
	}
	p, r, f := precisionRecall(expected, actual, MatchOverlap)
	if !approxEq(p, 0.5) {
		t.Errorf("precision = %.4f, want 0.5", p)
	}
	if !approxEq(r, 0.5) {
		t.Errorf("recall = %.4f, want 0.5", r)
	}
	if !approxEq(f, 0.5) {
		t.Errorf("f1 = %.4f, want 0.5", f)
	}
}

func TestPrecisionRecall_EmptyActualZerosBoth(t *testing.T) {
	t.Parallel()
	expected := []contract.Citation{cit("a.go", 1, 10)}
	p, r, f := precisionRecall(expected, nil, MatchOverlap)
	if p != 0 || r != 0 || f != 0 {
		t.Errorf("P/R/F = %.2f/%.2f/%.2f, want 0/0/0", p, r, f)
	}
}

func TestPrecisionRecall_EmptyExpectedYieldsPrecisionRecallSemantic(t *testing.T) {
	t.Parallel()
	// No ground truth: precision is undefined (we picked 1.0 as
	// "trivially correct" since no false-positive can be proven),
	// recall is 1.0 (no missed citations), f1 follows.
	actual := []contract.Citation{cit("a.go", 1, 10)}
	p, r, _ := precisionRecall(nil, actual, MatchOverlap)
	if !approxEq(p, 1.0) || !approxEq(r, 1.0) {
		t.Errorf("empty expected: P=%.2f R=%.2f, want 1/1", p, r)
	}
}

func TestPrecisionRecall_DedupesActual(t *testing.T) {
	t.Parallel()
	// Two actual citations with the same key must count once.
	expected := []contract.Citation{cit("a.go", 1, 10)}
	actual := []contract.Citation{
		cit("a.go", 1, 10),
		cit("a.go", 1, 10),
	}
	p, r, _ := precisionRecall(expected, actual, MatchOverlap)
	if !approxEq(p, 1.0) || !approxEq(r, 1.0) {
		t.Errorf("dedupe failed: P=%.4f R=%.4f", p, r)
	}
}

func TestPrecisionRecall_OverlapVsStrict(t *testing.T) {
	t.Parallel()
	expected := []contract.Citation{cit("a.go", 10, 30)}
	actual := []contract.Citation{cit("a.go", 15, 25)}

	pO, rO, _ := precisionRecall(expected, actual, MatchOverlap)
	if !approxEq(pO, 1.0) || !approxEq(rO, 1.0) {
		t.Errorf("overlap mode should accept partial range: P=%.2f R=%.2f", pO, rO)
	}

	pS, rS, _ := precisionRecall(expected, actual, MatchStrict)
	if pS != 0 || rS != 0 {
		t.Errorf("strict mode should reject partial range: P=%.2f R=%.2f", pS, rS)
	}
}

// --- median ---

func TestMedian_OddCount(t *testing.T) {
	t.Parallel()
	got := median([]float64{0.2, 0.5, 0.9})
	if !approxEq(got, 0.5) {
		t.Errorf("median = %.4f, want 0.5", got)
	}
}

func TestMedian_EvenCount(t *testing.T) {
	t.Parallel()
	got := median([]float64{0.2, 0.4, 0.6, 0.8})
	if !approxEq(got, 0.5) {
		t.Errorf("median = %.4f, want 0.5 (avg of middle two)", got)
	}
}

func TestMedian_EmptyReturnsZero(t *testing.T) {
	t.Parallel()
	if got := median(nil); got != 0 {
		t.Errorf("median(nil) = %.2f, want 0", got)
	}
}

func TestMedian_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	// median uses sort under the hood; callers may pass a slice they
	// care about. The helper must not reorder caller's data.
	in := []float64{0.9, 0.2, 0.5}
	_ = median(in)
	if in[0] != 0.9 || in[1] != 0.2 || in[2] != 0.5 {
		t.Errorf("input mutated: %v", in)
	}
}

func approxEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
