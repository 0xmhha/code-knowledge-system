package eval

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReport_WriteJSON_Schema(t *testing.T) {
	t.Parallel()
	rep := Report{
		SchemaVersion: 1,
		CKSVersion:    "cks-eval/test",
		StartedAt:     time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		Results: []ScenarioResult{
			{
				Name:      "s1",
				Prompt:    "find x",
				Runs:      1,
				MatchMode: "overlap",
				Metrics: Metrics{
					FilePrecision: 1.0,
					FileRecall:    0.5,
					FileF1:        0.6666666666666666,
					CitationCount: 2,
					LatencyMS:     42,
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, rep); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	out := buf.String()

	// Sanity-check key fields are present with expected JSON names.
	for _, want := range []string{
		`"schema_version": 1`,
		`"cks_version": "cks-eval/test"`,
		`"started_at"`,
		`"results"`,
		`"file_precision": 1`,
		`"file_recall": 0.5`,
		`"file_f1"`,
		`"name": "s1"`,
		`"runs": 1`,
		`"match_mode": "overlap"`,
		`"latency_ms": 42`,
		`"citation_count": 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestReport_WriteJSON_TrailingNewline(t *testing.T) {
	t.Parallel()
	// CLI tools that pipe JSON to other tools (`jq`, etc.) need a
	// trailing newline. WriteJSON must always emit one.
	var buf bytes.Buffer
	if err := WriteJSON(&buf, Report{SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("output missing trailing newline")
	}
}

func TestReport_WriteJSON_IsValidJSON(t *testing.T) {
	t.Parallel()
	rep := Report{
		SchemaVersion: 1,
		Results: []ScenarioResult{
			{Name: "x", Runs: 1, MatchMode: "strict"},
		},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, rep); err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(buf.Bytes(), &roundtrip); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
}

func TestReport_WriteJSON_Deterministic(t *testing.T) {
	t.Parallel()
	rep := Report{
		SchemaVersion: 1,
		CKSVersion:    "v",
		StartedAt:     time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		Results: []ScenarioResult{
			{Name: "a", Runs: 1, MatchMode: "overlap"},
			{Name: "b", Runs: 1, MatchMode: "overlap"},
		},
	}
	var b1, b2 bytes.Buffer
	_ = WriteJSON(&b1, rep)
	_ = WriteJSON(&b2, rep)
	if b1.String() != b2.String() {
		t.Error("WriteJSON not deterministic for the same input")
	}
}
