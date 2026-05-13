// Package budget implements the composer pipeline's Stage 4: pick the
// subset of Stage 2's seeds and Stage 3's neighbors whose bodies fit
// within a token budget.
//
// Stage 4 is the final "size reduction" step before sanitize (B.7) and
// pack assembly (B.8). The output is a ranked, body-bearing list ready
// for EvidencePack construction.
//
// Body sourcing: Stage 2/3 carry citations (file path + line range)
// without inline text. Stage 4 takes a BodyFetcher and pulls text on
// demand. Real implementations come in Phase C (filesystem read scoped
// to the indexed snapshot, or ckg-backed retrieval); tests use a Fake.
//
// Selection: greedy by score. The candidate list is the union of seeds
// and neighbors, sorted descending by their score. Citations are taken
// in order; when one doesn't fit, the loop continues — a smaller
// candidate further down the list may still fit. This is non-optimal
// (greedy isn't knapsack-optimal), but the simplicity is worth it for
// Phase 0; Phase E will measure whether the suboptimality matters.
package budget

import (
	"context"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// BodyFetcher returns the source text for a Citation. Implementations:
//
//   - Fake (this package): canned responses for tests.
//   - Real (Phase C): file-system read scoped to the indexed snapshot,
//     or a ckg-backed retrieval API that returns the chunk text directly.
//
// Empty string + nil error means "no body available" — the file was
// deleted, the ckg index is missing data, or the citation points to a
// line range that's gone. Stage 4 treats this as "skip, but not an
// error" so a single missing body doesn't abort the whole allocation.
type BodyFetcher interface {
	Fetch(ctx context.Context, c contract.Citation) (string, error)
}

// FakeFetcher is an in-memory BodyFetcher for tests. Bodies maps
// Citation.Key() -> text. Missing keys return ("", nil) — the same
// signal a real fetcher uses for "deleted/unavailable", which lets
// tests exercise the skip-empty-body path naturally.
//
// Err, when non-nil, is returned in preference to a successful result.
// Calls records every invocation for assertions.
type FakeFetcher struct {
	Bodies map[string]string
	Err    error

	Calls []FetchCall
}

// FetchCall captures one Fetch invocation.
type FetchCall struct {
	Citation contract.Citation
}

// Fetch implements BodyFetcher.
func (f *FakeFetcher) Fetch(ctx context.Context, c contract.Citation) (string, error) {
	f.Calls = append(f.Calls, FetchCall{Citation: c})
	if f.Err != nil {
		return "", f.Err
	}
	return f.Bodies[c.Key()], nil
}

// ResetCalls clears the recorded call history.
func (f *FakeFetcher) ResetCalls() { f.Calls = nil }
