// Package intent classifies a vibe prompt to one of the cks contract
// Intents using embedding-based similarity to per-Intent anchor examples.
//
// Why embedding (not keyword): cks's pipeline philosophy is "meaning via
// embedding, exactness via keyword/graph". Intent classification is a
// meaning task, so it uses the same family of tooling as ckv chunks. The
// upshot is that the same anchor lookups handle Korean and English
// uniformly (a multilingual embedder maps both to a shared vector space)
// and survive paraphrase ("X is broken" ~= "X 가 깨졌어요") without
// per-language rule maintenance.
//
// The classifier never calls an LLM — embeddings are produced by a local
// ONNX/transformer model (real impl lands with ckv's adapter in Phase C.2).
package intent

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

// Embedder turns text into a fixed-length float vector. cks's real
// implementation in Phase C.2 wraps the same model ckv uses for chunk
// embeddings, so anchor and chunk vectors live in the same space and
// downstream pipeline stages can reuse one model.
type Embedder interface {
	// Embed returns the embedding vector for text. Same input always
	// produces the same output. Returns an error on empty input or
	// backend failure.
	Embed(ctx context.Context, text string) ([]float32, error)
}

// FakeEmbedder is a deterministic in-memory Embedder for tests.
//
// Vectors maps known text -> canned vector. Texts not in the map fall
// back to a deterministic hash-derived vector (same text -> same vector,
// different texts -> uncorrelated vectors). The map lets a test compose
// "prompt P matches anchor A" relationships by giving both the same
// vector.
//
// Err, when non-nil, is returned in preference to a successful result.
// Calls records every invocation for assertions.
type FakeEmbedder struct {
	Vectors map[string][]float32
	Err     error
	// Dim is the dimension of hash-derived fallback vectors. Zero is
	// treated as 8 (small enough for tests, large enough to avoid
	// collisions).
	Dim int

	Calls []EmbedCall
}

// EmbedCall captures one Embed invocation.
type EmbedCall struct {
	Text string
}

// Embed implements Embedder.
func (f *FakeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	f.Calls = append(f.Calls, EmbedCall{Text: text})
	if f.Err != nil {
		return nil, f.Err
	}
	if v, ok := f.Vectors[text]; ok {
		return v, nil
	}
	return hashVector(text, f.dim()), nil
}

// ResetCalls clears the recorded call history.
func (f *FakeEmbedder) ResetCalls() { f.Calls = nil }

func (f *FakeEmbedder) dim() int {
	if f.Dim == 0 {
		return 8
	}
	return f.Dim
}

// hashVector turns text into a deterministic float vector by seeding a
// PRNG with SHA-256(text) and drawing dim NormFloat64 samples. Same text
// always yields the same vector; different texts yield uncorrelated
// vectors (cosine similarity near zero in expectation).
func hashVector(text string, dim int) []float32 {
	h := sha256.Sum256([]byte(text))
	seed1 := binary.LittleEndian.Uint64(h[0:8])
	seed2 := binary.LittleEndian.Uint64(h[8:16])
	rng := rand.New(rand.NewPCG(seed1, seed2))
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(rng.NormFloat64())
	}
	return v
}
