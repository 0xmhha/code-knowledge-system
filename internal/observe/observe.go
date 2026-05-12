// Package observe glues footprint and auditlog together for events that
// matter both as performance signals (latency, counts, decisions in flight)
// and as tamper-evident decision records (capability gates, sanitize hits).
//
// Use Audited where you would otherwise call footprint.Event followed by
// auditlog.Record on the next line; the helper keeps the two in lock-step
// and propagates Actor/Resource/Decision into the footprint fields so a
// single log query can pivot between the two streams.
package observe

import (
	"context"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/auditlog"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
)

// Audited emits a footprint event named r.Event and appends r to the audit
// log, in that order. Footprint receives Actor/Resource/Decision as
// additional fields when set on r, plus any extra zap fields supplied by
// the caller.
//
// Ordering rationale: footprint records the observation regardless of audit
// success so that a failed audit (e.g., disk full) is still visible in the
// perf trace. The audit log carries the tamper-evident decision; the
// returned error is from auditlog.Record and is the value the caller should
// surface or branch on.
//
// Nil fp or al is tolerated: the corresponding stream is skipped. This
// allows callers to wire up only one of the two streams during early
// development without conditional branches at every call site.
func Audited(
	ctx context.Context,
	fp *footprint.Logger,
	al *auditlog.Logger,
	r auditlog.Record,
	fields ...zap.Field,
) error {
	if r.Event == "" {
		return auditlog.ErrNoEvent
	}

	if fp != nil {
		fpFields := make([]zap.Field, 0, len(fields)+3)
		if r.Actor != "" {
			fpFields = append(fpFields, zap.String("actor", r.Actor))
		}
		if r.Resource != "" {
			fpFields = append(fpFields, zap.String("resource", r.Resource))
		}
		if r.Decision != "" {
			fpFields = append(fpFields, zap.String("decision", string(r.Decision)))
		}
		fpFields = append(fpFields, fields...)
		fp.Event(ctx, r.Event, fpFields...)
	}

	if al == nil {
		return nil
	}
	return al.Record(ctx, r)
}
