package ckvclient

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xmhha/code-knowledge-vector/pkg/ckv"
	ckvtypes "github.com/0xmhha/code-knowledge-vector/pkg/types"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// DefaultK is the SemanticSearch limit when SearchOpts.K is zero.
const DefaultK = 10

// modelProbeTTL bounds how often Health issues a fresh embedding-model
// liveness probe. Health is a fast, frequently-polled signal, so we cache
// the probe result for this window instead of embedding on every call; a
// failed real query trips the modelDown flag for immediate detection in
// between probes.
const modelProbeTTL = 5 * time.Second

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
	emb    ckvtypes.Embedder
	closed bool

	// modelDown is tripped when a real SemanticSearch fails (the engine's
	// embed call could not reach the model) and cleared on success, so the
	// next Health reflects a runtime disconnect without waiting for a probe.
	modelDown atomic.Bool

	// probe* cache the most recent embedding-model liveness probe under a
	// modelProbeTTL window (see probeModel).
	probeMu         sync.Mutex
	lastProbe       time.Time
	lastProbeOK     bool
	lastProbeReason string
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
	// The caller (cmd/cks-mcp) only constructs Real after buildEmbedder
	// probed the model successfully, so the model starts known-up.
	return &Real{eng: eng, emb: opts.Embedder, lastProbeOK: true}, nil
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
		// A query failure means the engine could not embed the query —
		// almost always the model endpoint is unreachable. Trip the flag so
		// the next Health reports the model down without waiting for a probe.
		r.modelDown.Store(true)
		return nil, fmt.Errorf("ckvclient: SemanticSearch: %w", err)
	}
	r.modelDown.Store(false)
	if resp == nil {
		return nil, nil
	}
	out := make([]contract.Hit, 0, len(resp.Hits))
	for i, h := range resp.Hits {
		// Symbol and CKGNodeID are the composer's bridge to ckg: Stage 1
		// extracts hit.Symbol as a candidate keyword (instead of the
		// file basename — the basename fallback survives in
		// extractKeywords for hits with empty Symbol), and Stage 2 can
		// short-circuit FindSymbol when CKGNodeID is populated. Empty
		// values are fine (omitempty on the wire); they only mean the
		// ckv chunk lacked the metadata (e.g. doc/header chunks).
		out = append(out, contract.Hit{
			Citation: contract.Citation{
				File:       h.Citation.File,
				StartLine:  h.Citation.StartLine,
				EndLine:    h.Citation.EndLine,
				CommitHash: h.Citation.CommitHash,
			},
			Rank:      i + 1,
			Score:     h.Score.Normalized,
			Source:    contract.HitSourceCKV,
			Symbol:    h.Symbol,
			CKGNodeID: h.CKGNodeID,
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

// Health reports two distinct signals: index identity (the engine is open,
// Reachable=true, with StatsHash/IndexedHead = the indexed git head and
// LastIndexAt from the manifest) and embedding-model liveness
// (ModelReachable, via probeModel). Separating them lets cks.ops.health tell
// "index loaded but model down" apart from "fully healthy" — the runtime
// disconnect the old manifest-only Health silently reported as reachable.
// DocsRoots returns the `ckv build --docs` corpus roots recorded in the
// index manifest. The composer's body fetcher resolves doc/markdown
// citations against these — they live outside the code source_root, so
// without them domain-corpus chunks have no body and get dropped.
func (r *Real) DocsRoots() []string {
	return r.eng.Manifest().DocsRoots
}

func (r *Real) Health(ctx context.Context) (Health, error) {
	man := r.eng.Manifest()
	modelUp, reason := r.probeModel(ctx)
	h := Health{
		Reachable:      true,
		ModelReachable: modelUp,
		StatsHash:      man.IndexedHead,
		IndexedHead:    man.IndexedHead,
		Reason:         reason,
	}
	if man.BuiltAt != "" {
		if t, perr := time.Parse(time.RFC3339, man.BuiltAt); perr == nil {
			h.LastIndexAt = t
		}
	}
	return h, nil
}

// probeModel reports embedding-model liveness. It short-circuits to "down"
// when a real query has tripped modelDown; otherwise it reuses a cached
// probe within modelProbeTTL and only issues a fresh single-token embed when
// the cache is stale. A probe failure also trips modelDown so a concurrent
// query path observes the outage immediately.
func (r *Real) probeModel(ctx context.Context) (bool, string) {
	if r.modelDown.Load() {
		return false, "embedding model unreachable (last query failed)"
	}
	r.probeMu.Lock()
	defer r.probeMu.Unlock()
	if !r.lastProbe.IsZero() && time.Since(r.lastProbe) < modelProbeTTL {
		return r.lastProbeOK, r.lastProbeReason
	}
	_, err := r.emb.Embed(ctx, []string{"liveness probe"})
	r.lastProbe = time.Now()
	if err != nil {
		r.lastProbeOK = false
		r.lastProbeReason = "embedding model probe failed: " + err.Error()
		r.modelDown.Store(true)
	} else {
		r.lastProbeOK = true
		r.lastProbeReason = ""
		r.modelDown.Store(false)
	}
	return r.lastProbeOK, r.lastProbeReason
}

// Close releases the underlying ckv engine. Idempotent.
func (r *Real) Close() error {
	if r.closed || r.eng == nil {
		return nil
	}
	r.closed = true
	return r.eng.Close()
}
