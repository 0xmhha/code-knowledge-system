package mcp

import (
	"context"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// healthResponse is the structured payload returned by cks.ops.health.
// Field names are JSON-tagged for stable wire shape.
//
// Beyond the rollup Status it carries identity metadata (builder version,
// source root, per-backend model + indexed commit) so a caller — possibly a
// remote Claude Code talking to one of several MCP instances — can tell
// WHICH instance it reached and WHAT code state / model that instance serves.
type healthResponse struct {
	Status string `json:"status"`
	// Serviceable is the single gate the query path honors. It is true only
	// when status == "ok": both ckg AND ckv (semantic retrieval, model up)
	// are usable. "degraded" and "down" are BOTH non-serviceable — per the
	// policy that a ckg-only pack lacks the upfront semantic understanding
	// needed to design/implement correctly, so it must not be served.
	Serviceable    bool                   `json:"serviceable"`
	BuilderVersion string                 `json:"builder_version,omitempty"`
	SourceRoot     string                 `json:"source_root,omitempty"`
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
	Reachable      bool       `json:"reachable"`
	ModelReachable bool       `json:"model_reachable,omitempty"` // ckv only
	SchemaVersion  string     `json:"schema_version,omitempty"`  // ckg only
	IndexedHead    string     `json:"indexed_head,omitempty"`    // indexed source commit
	StatsHash      string     `json:"stats_hash,omitempty"`      // ckv only
	LastIndexAt    *time.Time `json:"last_index_at,omitempty"`   // ckv only
	Provider       string     `json:"provider,omitempty"`        // ckv embedding provider
	Model          string     `json:"model,omitempty"`           // ckv embedding model
	Dim            int        `json:"dim,omitempty"`             // ckv embedding dimension
	Endpoint       string     `json:"endpoint,omitempty"`        // ckv model endpoint
	DataPath       string     `json:"data_path,omitempty"`       // backend DB path
	Reason         string     `json:"reason,omitempty"`          // why not serviceable
	Error          string     `json:"error,omitempty"`
}

// handleHealth is the standalone handler for cks.ops.health. It probes
// both backends in sequence (separate ctx-derived calls so a slow ckg
// does not delay ckv reporting on a future timeout-aware revision) and
// hands the booleans to aggregateHealthStatus for the rollup.
func handleHealth(ctx context.Context, d Deps, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	ckgH, ckgErr := d.CKG.Health(ctx)
	ckvH, ckvErr := d.CKV.Health(ctx)

	ckg := backendStat{Reachable: ckgErr == nil && ckgH.Reachable}
	ckg.DataPath = d.Index.CKGDataPath
	if ckgErr != nil {
		ckg.Error = ckgErr.Error()
	} else {
		ckg.SchemaVersion = ckgH.SchemaVersion
		ckg.IndexedHead = ckgH.IndexedHead
	}

	ckv := backendStat{Reachable: ckvErr == nil && ckvH.Reachable}
	// Identity/metadata is config-sourced (known regardless of reachability),
	// so a caller can still see which model+DB this instance is wired to even
	// when the backend is down.
	ckv.Provider = d.Embed.Provider
	ckv.Model = d.Embed.Model
	ckv.Dim = d.Embed.Dim
	ckv.Endpoint = d.Embed.Endpoint
	ckv.DataPath = d.Index.CKVDataPath
	if ckvErr != nil {
		ckv.Error = ckvErr.Error()
	} else {
		ckv.ModelReachable = ckvH.ModelReachable
		ckv.StatsHash = ckvH.StatsHash
		ckv.IndexedHead = ckvH.IndexedHead
		ckv.Reason = ckvH.Reason
		if !ckvH.LastIndexAt.IsZero() {
			t := ckvH.LastIndexAt
			ckv.LastIndexAt = &t
		}
	}

	// ckv is usable only when the index responds AND the model is reachable.
	ckvUsable := ckvErr == nil && ckvH.Reachable && ckvH.ModelReachable
	status := aggregateHealthStatus(ckg.Reachable, ckvUsable)

	out := healthResponse{
		Status:         status,
		Serviceable:    status == "ok",
		BuilderVersion: d.BuilderVersion,
		SourceRoot:     d.Index.SourceRoot,
		Backends: map[string]backendStat{
			"ckg": ckg,
			"ckv": ckv,
		},
	}
	return mcpgo.NewToolResultStructured(out, "health"), nil
}

// serviceable probes both backends and reports whether the composer can
// produce a trustworthy pack. ckv semantic understanding is REQUIRED: without
// it the pack lacks the upfront meaning the upper-layer LLM needs to design
// correctly, so a ckv-down ("degraded") state is non-serviceable — not a
// usable middle ground. Returns false plus an actionable reason otherwise.
func serviceable(ctx context.Context, d Deps) (bool, string) {
	ckgH, ckgErr := d.CKG.Health(ctx)
	ckvH, ckvErr := d.CKV.Health(ctx)
	ckgUp := ckgErr == nil && ckgH.Reachable
	ckvUp := ckvErr == nil && ckvH.Reachable && ckvH.ModelReachable
	switch {
	case !ckgUp:
		r := "ckg code graph unavailable"
		if ckgErr != nil {
			r += ": " + ckgErr.Error()
		}
		return false, r
	case !ckvUp:
		r := "ckv semantic retrieval not ready"
		switch {
		case ckvErr != nil:
			r += ": " + ckvErr.Error()
		case ckvH.Reason != "":
			r += ": " + ckvH.Reason
		}
		return false, r
	}
	return true, ""
}

// aggregateHealthStatus maps backend usability to an overall rollup.
//
// Policy:
//   - ckg unavailable → "down" (no citations, pack path broken).
//   - ckg up but ckv unusable (index down OR embedding model unreachable)
//     → "degraded". The label is retained for diagnosis (it tells an
//     operator the failure is on the ckv/model side), but "degraded" is
//     NOT a serviceable state: semantic understanding is required, so the
//     query path treats degraded the same as down (see serviceable).
//   - Both usable → "ok" (the only serviceable status).
//
// The second argument is "ckv usable" = index reachable AND model reachable,
// so a runtime model disconnect under a loaded index still rolls up to
// degraded rather than a false "ok".
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
