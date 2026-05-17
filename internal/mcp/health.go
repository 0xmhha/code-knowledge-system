package mcp

import (
	"context"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// healthResponse is the structured payload returned by cks.ops.health.
// Field names are JSON-tagged for stable wire shape.
type healthResponse struct {
	Status         string                 `json:"status"`
	BuilderVersion string                 `json:"builder_version,omitempty"`
	Backends       map[string]backendStat `json:"backends"`
}

// backendStat has a superset of ckg+ckv health fields; per-field omitempty
// keeps the wire payload to only what each backend actually reported.
//
// LastIndexAt is *time.Time, not time.Time: Go's encoding/json does NOT
// treat a zero time.Time as empty, so a non-pointer field would leak
// "0001-01-01T00:00:00Z" into the ckg payload even though ckg never
// reports a last-index time. The pointer form encodes nil as missing.
type backendStat struct {
	Reachable     bool       `json:"reachable"`
	SchemaVersion string     `json:"schema_version,omitempty"` // ckg only
	IndexedHead   string     `json:"indexed_head,omitempty"`   // ckg only
	StatsHash     string     `json:"stats_hash,omitempty"`     // ckv only
	LastIndexAt   *time.Time `json:"last_index_at,omitempty"`  // ckv only
	Error         string     `json:"error,omitempty"`
}

// handleHealth is the standalone handler for cks.ops.health. It probes
// both backends in sequence (separate ctx-derived calls so a slow ckg
// does not delay ckv reporting on a future timeout-aware revision) and
// hands the booleans to aggregateHealthStatus for the rollup.
func handleHealth(ctx context.Context, d Deps, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	ckgH, ckgErr := d.CKG.Health(ctx)
	ckvH, ckvErr := d.CKV.Health(ctx)

	ckg := backendStat{Reachable: ckgErr == nil && ckgH.Reachable}
	if ckgErr != nil {
		ckg.Error = ckgErr.Error()
	} else {
		ckg.SchemaVersion = ckgH.SchemaVersion
		ckg.IndexedHead = ckgH.IndexedHead
	}

	ckv := backendStat{Reachable: ckvErr == nil && ckvH.Reachable}
	if ckvErr != nil {
		ckv.Error = ckvErr.Error()
	} else {
		ckv.StatsHash = ckvH.StatsHash
		if !ckvH.LastIndexAt.IsZero() {
			t := ckvH.LastIndexAt
			ckv.LastIndexAt = &t
		}
	}

	out := healthResponse{
		Status:         aggregateHealthStatus(ckg.Reachable, ckv.Reachable),
		BuilderVersion: d.BuilderVersion,
		Backends: map[string]backendStat{
			"ckg": ckg,
			"ckv": ckv,
		},
	}
	return mcpgo.NewToolResultStructured(out, "health"), nil
}

// aggregateHealthStatus maps backend reachability to an overall rollup.
//
// Policy (HLD §10 Failure Modes & Graceful Degradation):
//   - ckg is REQUIRED. Without it the composer's stage2/stage3 cannot
//     produce citations and the pack path is broken: status = "down".
//   - ckv is OPTIONAL. With ckg up but ckv down the composer can still
//     produce a (less semantically-routed) pack: status = "degraded".
//   - Both up: status = "ok".
//
// Callers that need finer signal (per-stage liveness, latency budgets)
// should subscribe to the footprint event stream instead — health is a
// coarse, fast probe.
func aggregateHealthStatus(ckgUp, ckvUp bool) string {
	switch {
	case !ckgUp:
		return "down"
	case !ckvUp:
		return "degraded"
	default:
		return "ok"
	}
}
