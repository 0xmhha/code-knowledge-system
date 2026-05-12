package ckvclient

import (
	"context"
	"errors"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Fake is an in-memory Client that returns canned responses. Useful for
// unit-testing composer modules that depend on a Client without bringing
// up a real ckv backend.
//
// Configure Fake by populating its exported fields directly. The Err*
// fields, when non-nil, are returned in preference to the canned value;
// this lets tests assert error-path behavior cheaply.
type Fake struct {
	// SearchHits is returned by SemanticSearch on success.
	SearchHits []contract.Hit
	// SearchErr, when non-nil, is returned by SemanticSearch.
	SearchErr error

	// HealthVal is returned by Health on success.
	HealthVal Health
	// HealthErr, when non-nil, is returned by Health.
	HealthErr error

	// CloseErr, when non-nil, is returned by Close.
	CloseErr error

	// closed flips true after Close is called; visible to tests for
	// post-condition assertions.
	closed bool
}

// Compile-time assertion that Fake satisfies Client.
var _ Client = (*Fake)(nil)

// SemanticSearch returns f.SearchHits or f.SearchErr. Validates query.
func (f *Fake) SemanticSearch(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	if f.SearchErr != nil {
		return nil, f.SearchErr
	}
	if query == "" {
		return nil, errors.New("ckvclient: empty query")
	}
	if opts.K < 0 {
		return nil, errors.New("ckvclient: negative K")
	}
	out := f.SearchHits
	if opts.K > 0 && len(out) > opts.K {
		out = out[:opts.K]
	}
	return out, nil
}

// Health returns f.HealthVal or f.HealthErr.
func (f *Fake) Health(ctx context.Context) (Health, error) {
	if f.HealthErr != nil {
		return Health{}, f.HealthErr
	}
	return f.HealthVal, nil
}

// Close marks the fake closed and returns f.CloseErr.
func (f *Fake) Close() error {
	f.closed = true
	return f.CloseErr
}

// Closed reports whether Close has been called. For test assertions only.
func (f *Fake) Closed() bool { return f.closed }
