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

	// Neighbors traverses graph edges from src per opts. Used by the
	// Phase B.5 expander.
	Neighbors(ctx context.Context, src contract.Citation, opts NeighborsOpts) ([]contract.Neighbor, error)

	// Health reports backend reachability and version pins.
	//
	// Callers that need round-trip latency should measure time.Since
	// around the call themselves; Health does not include it because a
	// single in-band measurement carries no statistical meaning.
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
	// Kinds, when non-empty, restricts results to symbols of any of the
	// listed kinds ("function", "type", "method", "var", "const",
	// "interface"). Empty means any kind. Plural shape matches
	// ckvclient.SearchFilter.SymbolKinds — a single Go identifier like
	// "ProcessRequest" is commonly both a function and a method, so
	// callers want to retrieve them in one call.
	Kinds      []string
	PathGlob   string
	CommitHash string
}

// NeighborsOpts shapes a single Neighbors call.
type NeighborsOpts struct {
	// Relations restricts which edge types to traverse. Empty means all
	// known relations (RelationCalls, CalledBy, Implements, Imports,
	// References, TestedBy, Embeds, Defines).
	Relations []contract.Relation
	// Hops is the maximum traversal depth. Zero is treated as 1
	// (direct neighbors only). Negative values are rejected.
	Hops int
	// MaxTotal caps the total number of neighbors returned across all
	// relations. Zero means no cap (the backend may still apply its own).
	MaxTotal int
}

// Health is the result of a Client.Health() call. Reports backend state
// (reachability + version pins), not call-specific metrics.
type Health struct {
	Reachable bool
	// SchemaVersion is the ckg store schema version; the evaluation
	// harness compares this across runs.
	SchemaVersion string
	// IndexedHead is the git commit hash of the snapshot the backend was
	// built against. Empty when the backend did not report it.
	IndexedHead string
}
