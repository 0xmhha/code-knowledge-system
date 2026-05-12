package ckgclient

import (
	"context"
	"errors"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Fake is an in-memory Client returning canned responses. Symmetric to
// ckvclient.Fake; composer tests can wire both with identical idioms.
//
// All calls are recorded on Fake.Calls; tests can assert what was invoked
// (counts, arguments) without injecting a mocking framework.
type Fake struct {
	// BM25Hits is returned by BM25Search on success.
	BM25Hits []contract.Hit
	BM25Err  error

	// SymbolCitations is returned by FindSymbol on success.
	SymbolCitations []contract.Citation
	SymbolErr       error

	// NeighborEdges is returned by Neighbors on success.
	NeighborEdges []contract.Neighbor
	NeighborErr   error

	HealthVal Health
	HealthErr error

	CloseErr error

	// Calls records every method invocation.
	Calls FakeCalls

	closed bool
}

// FakeCalls records the methods invoked on a Fake and their arguments.
type FakeCalls struct {
	BM25Search []BM25SearchCall
	FindSymbol []FindSymbolCall
	Neighbors  []NeighborsCall
	Health     int
	Close      int
}

// BM25SearchCall captures the arguments of one BM25Search invocation.
type BM25SearchCall struct {
	Query string
	Opts  SearchOpts
}

// FindSymbolCall captures the arguments of one FindSymbol invocation.
type FindSymbolCall struct {
	Name string
	Opts SymbolOpts
}

// NeighborsCall captures the arguments of one Neighbors invocation.
type NeighborsCall struct {
	Src  contract.Citation
	Opts NeighborsOpts
}

// Reset clears all recorded calls.
func (c *FakeCalls) Reset() { *c = FakeCalls{} }

// Compile-time assertion that Fake satisfies Client.
var _ Client = (*Fake)(nil)

// BM25Search records the call, then returns f.BM25Hits or f.BM25Err.
func (f *Fake) BM25Search(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	f.Calls.BM25Search = append(f.Calls.BM25Search, BM25SearchCall{Query: query, Opts: opts})
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

// FindSymbol records the call, then returns f.SymbolCitations or f.SymbolErr.
func (f *Fake) FindSymbol(ctx context.Context, name string, opts SymbolOpts) ([]contract.Citation, error) {
	f.Calls.FindSymbol = append(f.Calls.FindSymbol, FindSymbolCall{Name: name, Opts: opts})
	if f.SymbolErr != nil {
		return nil, f.SymbolErr
	}
	if name == "" {
		return nil, errors.New("ckgclient: empty symbol name")
	}
	return f.SymbolCitations, nil
}

// Neighbors records the call, then returns f.NeighborEdges or f.NeighborErr.
func (f *Fake) Neighbors(ctx context.Context, src contract.Citation, opts NeighborsOpts) ([]contract.Neighbor, error) {
	f.Calls.Neighbors = append(f.Calls.Neighbors, NeighborsCall{Src: src, Opts: opts})
	if f.NeighborErr != nil {
		return nil, f.NeighborErr
	}
	if !src.IsValid() {
		return nil, errors.New("ckgclient: invalid src citation")
	}
	if opts.Hops < 0 {
		return nil, errors.New("ckgclient: negative hops")
	}
	// Hops == 0 is treated as 1 per doc; fake does not enforce traversal
	// depth (canned data is whatever the test sets).
	if opts.MaxTotal > 0 && len(f.NeighborEdges) > opts.MaxTotal {
		return f.NeighborEdges[:opts.MaxTotal], nil
	}
	return f.NeighborEdges, nil
}

// Health returns f.HealthVal or f.HealthErr.
func (f *Fake) Health(ctx context.Context) (Health, error) {
	f.Calls.Health++
	if f.HealthErr != nil {
		return Health{}, f.HealthErr
	}
	return f.HealthVal, nil
}

// Close marks the fake closed.
func (f *Fake) Close() error {
	f.Calls.Close++
	f.closed = true
	return f.CloseErr
}

// Closed reports whether Close was called.
func (f *Fake) Closed() bool { return f.closed }
