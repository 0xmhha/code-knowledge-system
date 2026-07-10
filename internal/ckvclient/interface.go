// Package ckvclient defines the cks-internal interface to a ckv (vector)
// backend and provides an in-memory fake for tests.
//
// The composer pipeline's Stage 1 (semantic) calls Client.SemanticSearch
// with a natural-language vibe prompt; the returned hits feed the keyword
// extractor (B.3) which surfaces concrete symbols/keywords for Stage 2
// (ckg search).
//
// Two implementations are planned:
//
//   - Fake (this package, B.1):   in-memory canned responses for unit
//     tests of composer modules.
//   - Real (Phase C.2):            adapter over
//     github.com/0xmhha/code-knowledge-vector
//     pkg/types.VectorStore.
//
// Stability: the interface here is the contract cks code depends on. Real
// adapters translate backend-native types into pkg/contract.Hit before
// returning, so backend changes do not leak through.
package ckvclient

import (
	"context"
	"time"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Client is the cks-internal interface to a ckv backend.
type Client interface {
	// SemanticSearch returns hits semantically similar to query, ranked
	// by the backend's vector distance. Hits carry Source = HitSourceCKV.
	//
	// Returns an error when query is empty, opts are invalid, or the
	// backend is unreachable. Callers should treat errors as "backend
	// degraded" and fall back to ckg BM25 alone if possible.
	SemanticSearch(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error)

	// Health reports backend reachability and version pins. Used by the
	// cks.ops.health MCP tool and by the evaluation harness to assert
	// the same index snapshot was used across runs.
	//
	// Callers that need round-trip latency should measure time.Since
	// around the call themselves; Health does not include it because a
	// single in-band measurement carries no statistical meaning.
	Health(ctx context.Context) (Health, error)

	// Freshness reports whether the index is up-to-date with the source
	// repository. Returns changed files since the last index build.
	Freshness(ctx context.Context) (FreshnessReport, error)

	// Close releases any resources (connections, mmap'd files).
	// Idempotent.
	Close() error
}

// FreshnessReport is the result of a Client.Freshness() call.
type FreshnessReport struct {
	// Fresh is true when the index matches the current source HEAD.
	Fresh bool
	// IndexedHead is the git commit the index was built against.
	IndexedHead string
	// CurrentHead is the current git HEAD of the source.
	CurrentHead string
	// ChangedFiles lists files modified since the indexed commit.
	ChangedFiles []string
}

// SearchOpts shapes a single SemanticSearch call.
type SearchOpts struct {
	// K is the maximum number of hits to return. Zero means use the
	// backend's default (typically 10).
	K int
	// Filter narrows the search domain.
	Filter SearchFilter
}

// SearchFilter restricts SemanticSearch to a subset of indexed content.
type SearchFilter struct {
	// Language, when non-empty, restricts to chunks of this language
	// (e.g. "go", "typescript").
	Language string
	// PathGlob, when non-empty, restricts to file paths matching the glob.
	PathGlob string
	// SymbolKinds restricts to specific symbol kinds (e.g. "function",
	// "method"). Empty means any kind.
	SymbolKinds []string
	// CommitHash pins the search to a specific snapshot. Empty means the
	// backend's latest indexed snapshot.
	CommitHash string
	// ChunkKinds restricts to ckv chunking strategies (e.g. "invariant",
	// "convention", "doc"). Empty means any kind. Powers the composer's
	// knowledge pass — invariants never outrank 14k code chunks in a
	// generic query, so they need a kind-scoped retrieval.
	ChunkKinds []string
}

// Health is the result of a Client.Health() call. Reports backend state
// (reachability + version pins), not call-specific metrics.
type Health struct {
	// Reachable is true when the ckv index itself is loaded and responding.
	// This is the "index identity" signal — it says nothing about whether
	// the embedding model behind the index is alive (see ModelReachable).
	Reachable bool
	// ModelReachable is true when the embedding model endpoint (e.g. the
	// Ollama bge-m3 daemon) answered the most recent liveness probe or
	// query. It is deliberately separate from Reachable: an index can be
	// loaded (Reachable=true) while the model serving its vectors has died
	// at runtime (ModelReachable=false) — a disconnect the old single
	// "Reachable" flag could not express. Dummy/unconfigured backends
	// report false: semantic retrieval is not actually available.
	ModelReachable bool
	// StatsHash is the ckv stats hash for cross-run reproducibility;
	// the evaluation harness compares this across runs.
	StatsHash string
	// IndexedHead is the git commit of the source snapshot the index was
	// built against — i.e. the project (go-stablenet) commit the DB
	// "learned". Surfaced in cks.ops.health so a caller can tell which
	// code state a given MCP instance was trained on.
	IndexedHead string
	// LastIndexAt is when the backend last completed an index build.
	// Zero when the backend did not report it.
	LastIndexAt time.Time
	// Reason explains why the backend is not fully serviceable (model
	// down, not configured, index mismatch). Empty when healthy.
	Reason string
}
