package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// reportSchemaVersion is the cks-eval report schema version. Bumped on
// breaking changes only; additive fields with `omitempty` keep
// downstream consumers compatible without a bump.
const reportSchemaVersion = 1

// Report is the top-level cks-eval output. One Report covers an entire
// `cks-eval` invocation; ScenarioResult is the per-scenario row.
//
// IntentSummary is a derived per-intent rollup. The CLI populates it
// before serialization so downstream consumers don't have to re-group
// the per-scenario rows.
type Report struct {
	SchemaVersion int              `json:"schema_version"`
	CKSVersion    string           `json:"cks_version,omitempty"`
	StartedAt     time.Time        `json:"started_at"`
	FinishedAt    time.Time        `json:"finished_at,omitempty"`
	Results       []ScenarioResult `json:"results"`
	IntentSummary []IntentSummary  `json:"intent_summary,omitempty"`
}

// IntentSummary is the per-intent rollup across all scenarios that
// share that intent. Scenarios with an empty Intent group under the
// sentinel "(unspecified)" so they remain visible. Errored runs are
// skipped from the average so a single broken scenario doesn't drag
// the group's metrics toward zero.
type IntentSummary struct {
	Intent       string  `json:"intent"`
	Count        int     `json:"count"`
	AvgPrecision float64 `json:"avg_precision"`
	AvgRecall    float64 `json:"avg_recall"`
	AvgF1        float64 `json:"avg_f1"`
	AvgLatencyMS int64   `json:"avg_latency_ms"`
}

// SummarizeByIntent buckets results by Intent and computes the average
// metrics inside each bucket. Returns nil for empty input. Output is
// sorted by Intent name so consumers get stable ordering across runs.
func SummarizeByIntent(results []ScenarioResult) []IntentSummary {
	if len(results) == 0 {
		return nil
	}
	const unspecified = "(unspecified)"
	type acc struct {
		count                    int
		sumP, sumR, sumF, sumLat float64
	}
	buckets := make(map[string]*acc)
	for _, r := range results {
		if r.Error != "" {
			continue
		}
		key := r.Intent
		if key == "" {
			key = unspecified
		}
		a, ok := buckets[key]
		if !ok {
			a = &acc{}
			buckets[key] = a
		}
		a.count++
		a.sumP += r.Metrics.FilePrecision
		a.sumR += r.Metrics.FileRecall
		a.sumF += r.Metrics.FileF1
		a.sumLat += float64(r.Metrics.LatencyMS)
	}
	if len(buckets) == 0 {
		return nil
	}
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]IntentSummary, 0, len(keys))
	for _, k := range keys {
		a := buckets[k]
		n := float64(a.count)
		out = append(out, IntentSummary{
			Intent:       k,
			Count:        a.count,
			AvgPrecision: a.sumP / n,
			AvgRecall:    a.sumR / n,
			AvgF1:        a.sumF / n,
			AvgLatencyMS: int64(a.sumLat / n),
		})
	}
	return out
}

// NewReport returns a Report with the schema version pre-set.
func NewReport(cksVersion string) Report {
	return Report{
		SchemaVersion: reportSchemaVersion,
		CKSVersion:    cksVersion,
		StartedAt:     time.Now().UTC(),
	}
}

// WriteJSON serializes rep as indented JSON with a trailing newline,
// suitable for piping to `jq` or committing as a CI artifact.
//
// Indented output is the cks-eval default — a CI run produces one
// report per invocation, so the storage cost is negligible and human
// readability is high. Callers that need compact output can wrap
// json.Marshal directly.
func WriteJSON(w io.Writer, rep Report) error {
	buf, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("eval: marshal report: %w", err)
	}
	if _, err := w.Write(buf); err != nil {
		return fmt.Errorf("eval: write report: %w", err)
	}
	if _, err := w.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("eval: write report newline: %w", err)
	}
	return nil
}
