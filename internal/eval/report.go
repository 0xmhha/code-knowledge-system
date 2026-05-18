package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// reportSchemaVersion is the cks-eval report schema version. Bumped on
// breaking changes only; additive fields with `omitempty` keep
// downstream consumers compatible without a bump.
const reportSchemaVersion = 1

// Report is the top-level cks-eval output. One Report covers an entire
// `cks-eval` invocation; ScenarioResult is the per-scenario row.
type Report struct {
	SchemaVersion int               `json:"schema_version"`
	CKSVersion    string            `json:"cks_version,omitempty"`
	StartedAt     time.Time         `json:"started_at"`
	FinishedAt    time.Time         `json:"finished_at,omitempty"`
	Results       []ScenarioResult  `json:"results"`
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
