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
	Health(ctx context.Context) (Health, error)

	// Close releases any resources (connections, mmap'd files).
	// Idempotent.
	Close() error
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
}

// Health is the result of a Client.Health() call.
type Health struct {
	// Reachable is true when the backend responded within the health
	// check timeout.
	Reachable bool
	// Latency is the round-trip duration of the health check.
	Latency time.Duration
	// StatsHash is the ckv stats hash for cross-run reproducibility;
	// the evaluation harness compares this across runs.
	StatsHash string
	// LastIndexAt is when the backend last completed an index build.
	// Zero when the backend did not report it.
	LastIndexAt time.Time
}
