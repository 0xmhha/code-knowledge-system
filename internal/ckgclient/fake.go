package ckgclient

import (
	"context"
	"errors"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Fake is an in-memory Client returning canned responses. See ckvclient.Fake
// for the same pattern; the two are deliberately symmetric so composer
// tests can wire both with identical idioms.
type Fake struct {
	// BM25Hits is returned by BM25Search.
	BM25Hits []contract.Hit
	BM25Err  error

	// SymbolCitations is returned by FindSymbol.
	SymbolCitations []contract.Citation
	SymbolErr       error

	// NeighborEdges is returned by Neighbors.
	NeighborEdges []contract.Neighbor
	NeighborErr   error

	HealthVal Health
	HealthErr error

	CloseErr error

	closed bool
}

// Compile-time assertion that Fake satisfies Client.
var _ Client = (*Fake)(nil)

// BM25Search returns f.BM25Hits or f.BM25Err.
func (f *Fake) BM25Search(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	if f.BM25Err != nil {
		return nil, f.BM25Err
	}
	if query == "" {
		return nil, errors.New("ckgclient: empty query")
	}
	if opts.K < 0 {
		return nil, errors.New("ckgclient: negative K")
	}
	out := f.BM25Hits
	if opts.K > 0 && len(out) > opts.K {
		out = out[:opts.K]
	}
	return out, nil
}

// FindSymbol returns f.SymbolCitations or f.SymbolErr.
func (f *Fake) FindSymbol(ctx context.Context, name string, opts SymbolOpts) ([]contract.Citation, error) {
	if f.SymbolErr != nil {
		return nil, f.SymbolErr
	}
	if name == "" {
		return nil, errors.New("ckgclient: empty symbol name")
	}
	return f.SymbolCitations, nil
}

// Neighbors returns f.NeighborEdges or f.NeighborErr.
func (f *Fake) Neighbors(ctx context.Context, src contract.Citation, relations []contract.Relation, hops int) ([]contract.Neighbor, error) {
	if f.NeighborErr != nil {
		return nil, f.NeighborErr
	}
	if !src.IsValid() {
		return nil, errors.New("ckgclient: invalid src citation")
	}
	if hops < 1 {
		return nil, errors.New("ckgclient: hops must be >= 1")
	}
	return f.NeighborEdges, nil
}

// Health returns f.HealthVal or f.HealthErr.
func (f *Fake) Health(ctx context.Context) (Health, error) {
	if f.HealthErr != nil {
		return Health{}, f.HealthErr
	}
	return f.HealthVal, nil
}

// Close marks the fake closed.
func (f *Fake) Close() error {
	f.closed = true
	return f.CloseErr
}

// Closed reports whether Close was called.
func (f *Fake) Closed() bool { return f.closed }
