package ckvclient

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/0xmhha/code-knowledge-vector/pkg/ckv"
)

// These tests cover the in-process adapter's construction guards and
// lifecycle. The SemanticSearch / Freshness / Health translation wraps a
// concrete *ckv.Engine (not an interface) over an on-disk index built by
// ckv's own pipeline — which lives in ckv's internal/ tree and cannot be
// imported here — so the translation is exercised end-to-end by the M2
// acceptance against a real go-stablenet index (operator-gated), not by a
// unit test. The field copies (ckv.Hit→contract.Hit, FreshnessReport,
// Manifest→Health) are mechanical and low-risk.

func TestNewReal_EmptyDataPathErrors(t *testing.T) {
	_, err := NewReal(context.Background(), RealOpts{Embedder: ckv.MockEmbedder()})
	if err == nil {
		t.Fatal("expected error for empty DataPath")
	}
}

func TestNewReal_NilEmbedderErrors(t *testing.T) {
	_, err := NewReal(context.Background(), RealOpts{DataPath: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for nil Embedder")
	}
}

func TestNewReal_MissingIndexErrors(t *testing.T) {
	// A real (mock) embedder + a directory with no manifest/vector.db: ckv.Open
	// must fail, and NewReal must surface it (fail fast, S5) rather than return
	// a half-open Real.
	_, err := NewReal(context.Background(), RealOpts{
		DataPath: filepath.Join(t.TempDir(), "nonexistent"),
		Embedder: ckv.MockEmbedder(),
	})
	if err == nil {
		t.Fatal("expected error opening a non-existent ckv index")
	}
}

func TestReal_Close_NilEngineIdempotent(t *testing.T) {
	// A zero-value Real (no engine) must Close cleanly — guards the degraded
	// path where construction failed but a closer is still deferred.
	r := &Real{}
	if err := r.Close(); err != nil {
		t.Fatalf("Close on nil-engine Real: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestReal_ImplementsClient(t *testing.T) {
	var _ Client = (*Real)(nil)
}
