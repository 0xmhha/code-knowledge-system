// Package envelope propagates request-scoped identifiers (trace_id, run_id,
// dry_run) through context.Context so that footprint logs, audit records, and
// MCP responses can be correlated across composer, agent, and eval components.
//
// trace_id: identifies one logical user request end-to-end (e.g. one vibe
// prompt resolved across multiple ckg/ckv calls).
// run_id:   identifies one agent run (e.g. one evaluation scenario execution
// or one coding-agent invocation; usually parent of multiple trace_ids).
// dry_run:  if true, side-effectful operations (file writes, git commits, MCP
// state mutations) must be skipped; reads/logs still proceed.
package envelope

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKey int

const (
	keyTraceID ctxKey = iota
	keyRunID
	keyDryRun
)

const idBytes = 16

// NewTraceID returns a new random hex-encoded trace identifier.
// Format: 32 lowercase hex characters (16 random bytes).
func NewTraceID() string {
	return randomHex(idBytes)
}

// NewRunID returns a new random hex-encoded run identifier.
func NewRunID() string {
	return randomHex(idBytes)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand on Linux/Darwin uses getrandom/arc4random; failure here
		// implies the OS RNG is broken, which is a fatal environment problem.
		panic("envelope: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// WithTraceID returns a derived context carrying traceID. Callers must pass a
// non-nil ctx (per Go context conventions); pass context.Background or
// context.TODO at API boundaries.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, keyTraceID, traceID)
}

// TraceID returns the trace identifier from ctx, or "" if absent.
func TraceID(ctx context.Context) string {
	v, _ := ctx.Value(keyTraceID).(string)
	return v
}

// WithRunID returns a derived context carrying runID.
func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, keyRunID, runID)
}

// RunID returns the run identifier from ctx, or "" if absent.
func RunID(ctx context.Context) string {
	v, _ := ctx.Value(keyRunID).(string)
	return v
}

// WithDryRun returns a derived context carrying the dry_run flag.
func WithDryRun(ctx context.Context, dryRun bool) context.Context {
	return context.WithValue(ctx, keyDryRun, dryRun)
}

// DryRun reports whether dry_run is set on ctx (default false).
func DryRun(ctx context.Context) bool {
	v, _ := ctx.Value(keyDryRun).(bool)
	return v
}

// EnsureTraceID returns ctx if it already has a trace_id, otherwise returns a
// derived context with a freshly generated trace_id. Useful at API entry
// points (HTTP handler, MCP tool dispatch) where upstream may or may not have
// set one.
func EnsureTraceID(ctx context.Context) context.Context {
	if TraceID(ctx) != "" {
		return ctx
	}
	return WithTraceID(ctx, NewTraceID())
}

// EnsureRunID returns ctx if it already has a run_id, otherwise returns a
// derived context with a freshly generated run_id.
func EnsureRunID(ctx context.Context) context.Context {
	if RunID(ctx) != "" {
		return ctx
	}
	return WithRunID(ctx, NewRunID())
}
