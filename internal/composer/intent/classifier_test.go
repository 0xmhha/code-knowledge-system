package intent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- helpers ---

// alignedVector returns a vector pointing along axis i out of dim.
// alignedVector(0, 3) -> [1, 0, 0]. Used to make orthogonal anchor groups.
func alignedVector(axis, dim int) []float32 {
	v := make([]float32, dim)
	v[axis] = 1
	return v
}

// twoIntentEmbedder returns a FakeEmbedder configured so that:
//   - Each anchor and the matching prompt share a per-Intent axis vector.
//   - Vectors for different Intents are orthogonal.
//
// Lets us assert "this prompt classifies as Intent X" deterministically.
func twoIntentEmbedder(_ *testing.T) *FakeEmbedder {
	const dim = 16
	vBugFix := alignedVector(0, dim)
	vSecurity := alignedVector(1, dim)
	return &FakeEmbedder{
		Dim: dim,
		Vectors: map[string][]float32{
			// One anchor per Intent so the classifier has something to
			// compare against; tests below pass a custom anchor map so
			// only these two Intents are evaluated.
			"bugfix-anchor":   vBugFix,
			"security-anchor": vSecurity,
			// Test prompts pre-mapped to identical vectors -> cosine 1.0
			"prompt: function crashes":             vBugFix,
			"prompt: audit auth for vulnerability": vSecurity,
			// Off-topic prompt: hash-derived (uncorrelated) -> low sim
		},
	}
}

func twoIntentAnchors() map[contract.Intent][]string {
	return map[contract.Intent][]string{
		contract.IntentBugFix:   {"bugfix-anchor"},
		contract.IntentSecurity: {"security-anchor"},
	}
}

// --- New / construction ---

func TestNew_NilEmbedderErrors(t *testing.T) {
	t.Parallel()
	_, err := New(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil embedder")
	}
}

func TestNew_PreEmbedsAllAnchors(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	cls, err := New(context.Background(), embedder, WithAnchors(twoIntentAnchors()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(cls.embedded) != 2 {
		t.Fatalf("embedded intents = %d, want 2", len(cls.embedded))
	}
	if len(cls.embedded[contract.IntentBugFix]) != 1 {
		t.Errorf("BugFix anchor count = %d", len(cls.embedded[contract.IntentBugFix]))
	}
	// Should have called Embed exactly once per anchor.
	if len(embedder.Calls) != 2 {
		t.Errorf("Embed call count = %d, want 2", len(embedder.Calls))
	}
}

func TestNew_PreEmbedErrorPropagates(t *testing.T) {
	t.Parallel()
	embedder := &FakeEmbedder{Err: errors.New("embed down")}
	_, err := New(context.Background(), embedder, WithAnchors(twoIntentAnchors()))
	if err == nil {
		t.Fatal("expected pre-embed error")
	}
	if !strings.Contains(err.Error(), "pre-embed") {
		t.Errorf("error = %v, want 'pre-embed' context", err)
	}
}

func TestNew_RejectsInvalidIntent(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	_, err := New(context.Background(), embedder, WithAnchors(map[contract.Intent][]string{
		contract.Intent("made_up_intent"): {"foo"},
	}))
	if err == nil {
		t.Fatal("expected error for unknown Intent in anchors")
	}
	if !strings.Contains(err.Error(), "unknown Intent") {
		t.Errorf("error = %v, want 'unknown Intent' context", err)
	}
}

func TestNew_RejectsEmptyAnchorList(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	_, err := New(context.Background(), embedder, WithAnchors(map[contract.Intent][]string{
		contract.IntentBugFix: {},
	}))
	if err == nil {
		t.Fatal("expected error for empty anchor list")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %v, want 'empty' context", err)
	}
}

func TestNew_RejectsEmptyAnchorText(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	cases := map[string]string{
		"empty":           "",
		"whitespace only": "   \t\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := New(context.Background(), embedder, WithAnchors(map[contract.Intent][]string{
				contract.IntentBugFix: {text},
			}))
			if err == nil {
				t.Fatalf("expected error for anchor text = %q", text)
			}
		})
	}
}

func TestDefaultUnknownThreshold_Value(t *testing.T) {
	t.Parallel()
	// Locked-in policy value (cks "when in doubt, IntentUnknown"). Any
	// future change to this number should go through review because it
	// affects classifier behavior across every Intent.
	if DefaultUnknownThreshold != 0.55 {
		t.Errorf("DefaultUnknownThreshold = %v, want 0.55 (2026-07-09 bge-m3 calibration)", DefaultUnknownThreshold)
	}
}

func TestNew_DefaultAnchorsLoad(t *testing.T) {
	t.Parallel()
	// Use FakeEmbedder with hash fallback (no Vectors map). The
	// default anchor set should embed cleanly.
	embedder := &FakeEmbedder{Dim: 16}
	cls, err := New(context.Background(), embedder)
	if err != nil {
		t.Fatalf("New with default anchors: %v", err)
	}
	// Every Intent in the default set must have at least one embedded
	// anchor.
	for intentVal := range defaultAnchors {
		if len(cls.embedded[intentVal]) == 0 {
			t.Errorf("default anchor for %s did not embed", intentVal)
		}
	}
}

// --- Classify ---

func TestClassify_MatchesAlignedPrompt(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	cls, err := New(context.Background(), embedder, WithAnchors(twoIntentAnchors()))
	if err != nil {
		t.Fatal(err)
	}
	got, err := cls.Classify(context.Background(), "prompt: function crashes")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Intent != contract.IntentBugFix {
		t.Errorf("Intent = %v, want BugFix", got.Intent)
	}
	if math.Abs(got.Confidence-1.0) > 1e-6 {
		t.Errorf("Confidence = %f, want ~1.0", got.Confidence)
	}
	if got.BestAnchor != "bugfix-anchor" {
		t.Errorf("BestAnchor = %q", got.BestAnchor)
	}
}

func TestClassify_DistinguishesBetweenIntents(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	cls, err := New(context.Background(), embedder, WithAnchors(twoIntentAnchors()))
	if err != nil {
		t.Fatal(err)
	}
	got, err := cls.Classify(context.Background(), "prompt: audit auth for vulnerability")
	if err != nil {
		t.Fatal(err)
	}
	if got.Intent != contract.IntentSecurity {
		t.Errorf("Intent = %v, want Security", got.Intent)
	}
}

func TestClassify_BelowThresholdReturnsUnknown(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	cls, err := New(context.Background(), embedder,
		WithAnchors(twoIntentAnchors()),
		WithUnknownThreshold(0.9),
	)
	if err != nil {
		t.Fatal(err)
	}
	// Off-topic prompt has no entry in Vectors, so FakeEmbedder returns
	// a hash-derived vector uncorrelated with either anchor -> low sim.
	got, err := cls.Classify(context.Background(), "totally unrelated text about gardening")
	if err != nil {
		t.Fatal(err)
	}
	if got.Intent != contract.IntentUnknown {
		t.Errorf("Intent = %v, want Unknown (low confidence)", got.Intent)
	}
	if got.BestAnchor != "" {
		t.Errorf("BestAnchor = %q, want empty for Unknown", got.BestAnchor)
	}
}

func TestClassify_EmptyPromptErrors(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	cls, err := New(context.Background(), embedder, WithAnchors(twoIntentAnchors()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = cls.Classify(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestClassify_EmbedErrorPropagates(t *testing.T) {
	t.Parallel()
	embedder := twoIntentEmbedder(t)
	cls, err := New(context.Background(), embedder, WithAnchors(twoIntentAnchors()))
	if err != nil {
		t.Fatal(err)
	}
	// Flip embedder to fail on the *classify* call (after anchors are
	// already embedded successfully).
	embedder.Err = errors.New("embed broken")
	got, err := cls.Classify(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected error from embedder")
	}
	if got.Intent != contract.IntentUnknown {
		t.Errorf("on error, Intent should fall back to Unknown, got %v", got.Intent)
	}
}

// --- Footprint emission ---

func TestClassify_EmitsFootprintEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	embedder := twoIntentEmbedder(t)
	cls, err := New(context.Background(), embedder,
		WithAnchors(twoIntentAnchors()),
		WithFootprint(fp),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = cls.Classify(context.Background(), "prompt: function crashes")
	_ = fp.Sync()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if rec["event"] != "composer.intent_classified" {
		t.Errorf("event = %v", rec["event"])
	}
	if rec["intent"] != "bug_fix" {
		t.Errorf("intent = %v, want bug_fix", rec["intent"])
	}
	if conf, ok := rec["confidence"].(float64); !ok || conf < 0.99 {
		t.Errorf("confidence = %v, want ~1.0", rec["confidence"])
	}
}

func TestClassify_NoFootprintNoEmit(t *testing.T) {
	t.Parallel()
	// Without WithFootprint, Classify must not panic and must not need
	// a footprint logger.
	embedder := twoIntentEmbedder(t)
	cls, err := New(context.Background(), embedder, WithAnchors(twoIntentAnchors()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cls.Classify(context.Background(), "prompt: function crashes"); err != nil {
		t.Fatalf("Classify without footprint: %v", err)
	}
}

// --- cosine ---

func TestCosine_IdenticalVectors(t *testing.T) {
	t.Parallel()
	a := []float32{1, 2, 3, 4}
	if got := cosine(a, a); math.Abs(got-1.0) > 1e-6 {
		t.Errorf("cosine(a,a) = %f, want 1.0", got)
	}
}

func TestCosine_OrthogonalVectors(t *testing.T) {
	t.Parallel()
	a := []float32{1, 0, 0, 0}
	b := []float32{0, 1, 0, 0}
	if got := cosine(a, b); math.Abs(got) > 1e-6 {
		t.Errorf("cosine(orthogonal) = %f, want 0", got)
	}
}

func TestCosine_OppositeVectors(t *testing.T) {
	t.Parallel()
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	if got := cosine(a, b); math.Abs(got+1.0) > 1e-6 {
		t.Errorf("cosine(opposite) = %f, want -1.0", got)
	}
}

func TestCosine_EdgeCases(t *testing.T) {
	t.Parallel()
	if cosine(nil, nil) != 0 {
		t.Error("cosine(nil, nil) should be 0")
	}
	if cosine([]float32{1}, []float32{1, 2}) != 0 {
		t.Error("cosine(length mismatch) should be 0")
	}
	if cosine([]float32{0, 0}, []float32{1, 1}) != 0 {
		t.Error("cosine(zero vector) should be 0")
	}
}

// --- FakeEmbedder ---

func TestFakeEmbedder_HashFallbackDeterministic(t *testing.T) {
	t.Parallel()
	e := &FakeEmbedder{Dim: 8}
	v1, err := e.Embed(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := e.Embed(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("hash vectors diverged at index %d: %v vs %v", i, v1[i], v2[i])
		}
	}
}

func TestFakeEmbedder_HashFallbackUncorrelated(t *testing.T) {
	t.Parallel()
	e := &FakeEmbedder{Dim: 64}
	va, _ := e.Embed(context.Background(), "alpha")
	vb, _ := e.Embed(context.Background(), "beta")
	// Hash-derived vectors should be near-orthogonal — well below the
	// 0.5 default unknown threshold.
	sim := cosine(va, vb)
	if math.Abs(sim) > 0.5 {
		t.Errorf("hash-derived cosine = %f, want |sim| < 0.5", sim)
	}
}

func TestFakeEmbedder_RecordsCalls(t *testing.T) {
	t.Parallel()
	e := &FakeEmbedder{Dim: 4}
	_, _ = e.Embed(context.Background(), "first")
	_, _ = e.Embed(context.Background(), "second")
	if len(e.Calls) != 2 || e.Calls[0].Text != "first" || e.Calls[1].Text != "second" {
		t.Fatalf("Calls = %+v", e.Calls)
	}
	e.ResetCalls()
	if len(e.Calls) != 0 {
		t.Fatalf("ResetCalls did not clear: %+v", e.Calls)
	}
}

func TestFakeEmbedder_ErrTakesPrecedence(t *testing.T) {
	t.Parallel()
	want := errors.New("backend down")
	e := &FakeEmbedder{Err: want, Vectors: map[string][]float32{"x": {1, 0}}}
	_, err := e.Embed(context.Background(), "x")
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
