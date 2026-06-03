package ckvclient

import (
	"context"
	"fmt"
	"time"

	"github.com/0xmhha/code-knowledge-vector/pkg/ckv"
	ckvtypes "github.com/0xmhha/code-knowledge-vector/pkg/types"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// DefaultK is the SemanticSearch limit when SearchOpts.K is zero.
const DefaultK = 10

// RealOpts configures the in-process ckv adapter (G1). The old subprocess
// fields (BinaryPath / Env / CallTimeout) are gone — cks imports pkg/ckv
// directly, so there is no child process to spawn or restart.
type RealOpts struct {
	// DataPath is the ckv-data directory (vector.db + manifest.json)
	// produced by `ckv build`. Passed to ckv.Open.
	DataPath string

	// Embedder must match the embedder the index was built with (same
	// name + dimension) or ckv.Open returns ErrIndexUnavailable. The
	// caller constructs it once (typically the Ollama bge-m3 adapter)
	// and shares the same instance with the intent classifier so anchor,
	// query, and chunk vectors live in one model space.
	Embedder ckvtypes.Embedder
}

// Real is the in-process ckv adapter (G1). It holds an open *ckv.Engine and
// translates ckv.Hit / Manifest / FreshnessReport into cks contract types.
//
// No subprocess, no MCP transport: the 543-LOC proxy this replaced spawned
// the ckv binary and proxied every query over stdio, which hung 2/9 dogfood
// runs. ckv.Engine is concurrent-safe for reads, so Real needs no locking.
type Real struct {
	eng    *ckv.Engine
	closed bool
}

// Compile-time guarantee Real satisfies Client.
var _ Client = (*Real)(nil)

// NewReal opens the ckv index at opts.DataPath with opts.Embedder, in
// process. It fails fast (S5) when DataPath is empty, the Embedder is nil,
// or the embedder can't serve the index (identity mismatch) — the caller
// (cmd/cks-mcp) catches the error and falls back to the Smart Dummy plus a
// degraded cks.ops.health status rather than crashing.
func NewReal(ctx context.Context, opts RealOpts) (*Real, error) {
	if opts.DataPath == "" {
		return nil, fmt.Errorf("ckvclient: empty DataPath")
	}
	if opts.Embedder == nil {
		return nil, fmt.Errorf("ckvclient: nil Embedder")
	}
	eng, err := ckv.Open(opts.DataPath, ckv.OpenOptions{Embedder: opts.Embedder})
	if err != nil {
		return nil, fmt.Errorf("ckvclient: ckv.Open %q: %w", opts.DataPath, err)
	}
	return &Real{eng: eng}, nil
}

// SemanticSearch runs an in-process vector query and translates each
// ckv.Hit into a contract.Hit stamped HitSourceCKV. The score is ckv's
// normalized distance (Score.Normalized, in [0,1]).
//
// Filter push-down (Language/PathGlob/SymbolKinds) is not yet mapped onto
// ckv.SearchOptions — the composer's Stage 1 uses K-bounded semantic recall
// and applies its own narrowing downstream. Follow-up: thread the filter
// through once ckv exposes the corresponding query.Options fields.
func (r *Real) SemanticSearch(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	if query == "" {
		return nil, fmt.Errorf("ckvclient: empty query")
	}
	k := opts.K
	if k <= 0 {
		k = DefaultK
	}
	resp, err := r.eng.SemanticSearch(ctx, query, ckv.SearchOptions{K: k})
	if err != nil {
		return nil, fmt.Errorf("ckvclient: SemanticSearch: %w", err)
	}
	if resp == nil {
		return nil, nil
	}
	out := make([]contract.Hit, 0, len(resp.Hits))
	for i, h := range resp.Hits {
		out = append(out, contract.Hit{
			Citation: contract.Citation{
				File:       h.Citation.File,
				StartLine:  h.Citation.StartLine,
				EndLine:    h.Citation.EndLine,
				CommitHash: h.Citation.CommitHash,
			},
			Rank:   i + 1,
			Score:  h.Score.Normalized,
			Source: contract.HitSourceCKV,
		})
	}
	return out, nil
}

// Freshness reports index-vs-HEAD staleness via ckv's structured
// Engine.Freshness (02 G6). Git unavailability is reported by ckv as
// Fresh=false with warnings, not as an error here.
func (r *Real) Freshness(ctx context.Context) (FreshnessReport, error) {
	rep, err := r.eng.Freshness()
	if err != nil {
		return FreshnessReport{}, fmt.Errorf("ckvclient: freshness: %w", err)
	}
	return FreshnessReport{
		Fresh:        rep.Fresh,
		IndexedHead:  rep.IndexedHead,
		CurrentHead:  rep.CurrentHead,
		ChangedFiles: rep.ChangedFiles,
	}, nil
}

// Health reports the in-process engine as reachable plus the index identity.
// StatsHash is the indexed git head (stable per snapshot, what the eval
// harness compares across runs); LastIndexAt parses the manifest's RFC3339
// built-at timestamp when present.
func (r *Real) Health(ctx context.Context) (Health, error) {
	man := r.eng.Manifest()
	h := Health{
		Reachable: true,
		StatsHash: man.IndexedHead,
	}
	if man.BuiltAt != "" {
		if t, perr := time.Parse(time.RFC3339, man.BuiltAt); perr == nil {
			h.LastIndexAt = t
		}
	}
	return h, nil
}

// Close releases the underlying ckv engine. Idempotent.
func (r *Real) Close() error {
	if r.closed || r.eng == nil {
		return nil
	}
	r.closed = true
	return r.eng.Close()
}
