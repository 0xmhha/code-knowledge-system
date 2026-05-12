// Package ckgclient defines the cks-internal interface to a ckg (graph +
// BM25) backend and provides an in-memory fake for tests.
//
// The composer pipeline's Stage 2 (precise) calls BM25Search and
// FindSymbol with the keywords/symbols surfaced by Stage 1 (ckv). The
// graph neighbor expander (Phase B.5) calls Neighbors to surround the
// matched citations with their calls, callers, implementations, etc.
//
// Two implementations are planned:
//
//   - Fake (this package, B.1):   in-memory canned responses for unit tests.
//   - Real (Phase C.1):            adapter over
//     github.com/0xmhha/code-knowledge-graph
//     pkg/store.Reader.
//
// Stability: the interface here is the contract cks code depends on. Real
// adapters translate backend-native types into pkg/contract types before
// returning, so backend changes do not leak through.
package ckgclient

import (
	"context"
	"time"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Client is the cks-internal interface to a ckg backend.
type Client interface {
	// BM25Search returns hits matching query via the backend's BM25
	// keyword index. Hits carry Source = HitSourceCKG.
	BM25Search(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error)

	// FindSymbol resolves a symbol name (e.g. "ProcessRequest") to its
	// definition citations. Multiple results are possible for overloaded
	// or package-private symbols sharing a name.
	FindSymbol(ctx context.Context, name string, opts SymbolOpts) ([]contract.Citation, error)

	// Neighbors traverses graph edges from src for the given relation
	// set, returning up to `hops` levels of neighbors. Used by the Phase
	// B.5 expander.
	//
	// When relations is empty, all known relations are traversed
	// (RelationCalls, CalledBy, Implements, Imports, References,
	// TestedBy, Embeds, Defines).
	Neighbors(ctx context.Context, src contract.Citation, relations []contract.Relation, hops int) ([]contract.Neighbor, error)

	// Health reports backend reachability and version pins.
	Health(ctx context.Context) (Health, error)

	// Close releases any resources. Idempotent.
	Close() error
}

// SearchOpts shapes a single BM25Search call.
type SearchOpts struct {
	// K is the maximum number of hits to return. Zero means backend default.
	K int
	// Filter narrows the search domain.
	Filter SearchFilter
}

// SearchFilter restricts BM25Search to a subset of indexed content.
type SearchFilter struct {
	Language   string // e.g. "go"
	PathGlob   string // e.g. "internal/**"
	CommitHash string // snapshot pin; empty = latest indexed
}

// SymbolOpts shapes a single FindSymbol call.
type SymbolOpts struct {
	// Kind, when non-empty, restricts results to symbols of this kind
	// ("function", "type", "method", "var", "const", "interface").
	Kind string
	// PathGlob and CommitHash behave as in SearchFilter.
	PathGlob   string
	CommitHash string
}

// Health is the result of a Client.Health() call.
type Health struct {
	Reachable bool
	Latency   time.Duration
	// SchemaVersion is the ckg store schema version; the evaluation
	// harness compares this across runs.
	SchemaVersion string
	// IndexedHead is the git commit hash of the snapshot the backend was
	// built against. Empty when the backend did not report it.
	IndexedHead string
}
