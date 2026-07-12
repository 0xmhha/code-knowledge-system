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
//
// All calls are recorded on Fake.Calls; tests can assert what was invoked
// (counts, arguments) without injecting a mocking framework.
type Fake struct {
	// SearchHits is returned by SemanticSearch on success.
	SearchHits []contract.Hit
	// SearchErr, when non-nil, is returned by SemanticSearch.
	SearchErr error

	// FreshnessVal is returned by Freshness on success.
	FreshnessVal FreshnessReport
	// FreshnessErr, when non-nil, is returned by Freshness.
	FreshnessErr error

	// HealthVal is returned by Health on success.
	HealthVal Health
	// HealthErr, when non-nil, is returned by Health.
	HealthErr error

	// CloseErr, when non-nil, is returned by Close.
	CloseErr error

	// Flow* back the FlowClient methods (Phase D). Each *Val is returned on
	// success; the matching *Err takes precedence when non-nil.
	FlowVal                    Flow
	GetFlowErr                 error
	ExpandVal                  FlowExpansion
	ExpandFlowErr              error
	BranchMatches              []BranchMatch
	FindBranchesErr            error
	EnforcementVal             InvariantEnforcement
	GetInvariantEnforcementErr error
	InvariantHits              []InvariantHit
	FindInvariantsErr          error
	Conventions                []ConventionHit
	GetConventionsErr          error

	// Calls records every method invocation for test assertions.
	Calls FakeCalls

	// closed flips true after Close is called; visible via Closed() for
	// post-condition assertions.
	closed bool
}

// FakeCalls records the methods invoked on a Fake and their arguments.
type FakeCalls struct {
	SemanticSearch          []SemanticSearchCall
	Freshness               int
	Health                  int
	Close                   int
	GetFlow                 []FlowQuery
	ExpandFlow              []ExpandFlowQuery
	FindBranches            []FindBranchesCall
	GetInvariantEnforcement []GetInvariantEnforcementCall
	FindInvariants          []FindInvariantsCall
	GetConventions          []GetConventionsCall
}

// FindInvariantsCall captures the arguments of one FindInvariants invocation.
type FindInvariantsCall struct {
	File     string
	Category string
	TierMin  int
}

// GetConventionsCall captures the arguments of one GetConventions invocation.
type GetConventionsCall struct {
	PackagePrefix string
}

// SemanticSearchCall captures the arguments of one SemanticSearch invocation.
type SemanticSearchCall struct {
	Query string
	Opts  SearchOpts
}

// FindBranchesCall captures the arguments of one FindBranches invocation.
type FindBranchesCall struct {
	Symptom string
	K       int
}

// GetInvariantEnforcementCall captures the arguments of one
// GetInvariantEnforcement invocation.
type GetInvariantEnforcementCall struct {
	InvID string
	Max   int
}

// Reset clears all recorded calls. Useful between test sub-cases.
func (c *FakeCalls) Reset() { *c = FakeCalls{} }

// Compile-time assertion that Fake satisfies Client.
var _ Client = (*Fake)(nil)

// SemanticSearch records the call, then returns f.SearchHits or f.SearchErr.
func (f *Fake) SemanticSearch(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	f.Calls.SemanticSearch = append(f.Calls.SemanticSearch, SemanticSearchCall{
		Query: query,
		Opts:  opts,
	})
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

// Freshness records the call, then returns f.FreshnessVal or f.FreshnessErr.
func (f *Fake) Freshness(ctx context.Context) (FreshnessReport, error) {
	f.Calls.Freshness++
	if f.FreshnessErr != nil {
		return FreshnessReport{}, f.FreshnessErr
	}
	return f.FreshnessVal, nil
}

// Health records the call, then returns f.HealthVal or f.HealthErr.
func (f *Fake) Health(ctx context.Context) (Health, error) {
	f.Calls.Health++
	if f.HealthErr != nil {
		return Health{}, f.HealthErr
	}
	return f.HealthVal, nil
}

// Close records the call, marks the fake closed, and returns f.CloseErr.
func (f *Fake) Close() error {
	f.Calls.Close++
	f.closed = true
	return f.CloseErr
}

// Closed reports whether Close has been called. For test assertions only.
func (f *Fake) Closed() bool { return f.closed }
