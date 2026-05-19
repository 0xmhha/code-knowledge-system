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

// --- SummarizeByIntent ---

func TestSummarizeByIntent_GroupsAndAverages(t *testing.T) {
	t.Parallel()
	results := []ScenarioResult{
		{Name: "a", Intent: "arch_explain", Metrics: Metrics{FilePrecision: 0.4, FileRecall: 0.8, FileF1: 0.5, LatencyMS: 100}},
		{Name: "b", Intent: "arch_explain", Metrics: Metrics{FilePrecision: 0.6, FileRecall: 0.6, FileF1: 0.6, LatencyMS: 200}},
		{Name: "c", Intent: "refactor", Metrics: Metrics{FilePrecision: 0.1, FileRecall: 1.0, FileF1: 0.18, LatencyMS: 50}},
	}
	out := SummarizeByIntent(results)
	if len(out) != 2 {
		t.Fatalf("got %d intent groups, want 2", len(out))
	}
	// Deterministic order: sorted by intent name.
	if out[0].Intent != "arch_explain" || out[1].Intent != "refactor" {
		t.Errorf("order = %s,%s want arch_explain,refactor", out[0].Intent, out[1].Intent)
	}
	arch := out[0]
	if arch.Count != 2 {
		t.Errorf("arch.Count = %d, want 2", arch.Count)
	}
	if !approxEqReport(arch.AvgPrecision, 0.5) || !approxEqReport(arch.AvgRecall, 0.7) || !approxEqReport(arch.AvgF1, 0.55) {
		t.Errorf("arch avg P/R/F = %.3f/%.3f/%.3f, want 0.5/0.7/0.55",
			arch.AvgPrecision, arch.AvgRecall, arch.AvgF1)
	}
	if arch.AvgLatencyMS != 150 {
		t.Errorf("arch.AvgLatencyMS = %d, want 150", arch.AvgLatencyMS)
	}
	if out[1].Count != 1 || out[1].AvgRecall != 1.0 {
		t.Errorf("refactor group wrong: %+v", out[1])
	}
}

func TestSummarizeByIntent_SkipsErroredResults(t *testing.T) {
	t.Parallel()
	// Errored runs have zero metrics. Including them would drag the
	// average toward 0 and mask the working scenarios. SummarizeByIntent
	// must skip results with non-empty Error.
	results := []ScenarioResult{
		{Name: "ok", Intent: "arch_explain", Metrics: Metrics{FilePrecision: 0.8, FileRecall: 0.8, FileF1: 0.8}},
		{Name: "broken", Intent: "arch_explain", Error: "tool error", Metrics: Metrics{}},
	}
	out := SummarizeByIntent(results)
	if len(out) != 1 || out[0].Count != 1 {
		t.Fatalf("errored result not skipped: %+v", out)
	}
	if !approxEqReport(out[0].AvgRecall, 0.8) {
		t.Errorf("recall pulled toward 0: %v", out[0].AvgRecall)
	}
}

func TestSummarizeByIntent_EmptyIntentUsesSentinel(t *testing.T) {
	t.Parallel()
	// Scenarios without an explicit intent group under "(unspecified)"
	// so the breakdown still shows them rather than silently dropping.
	results := []ScenarioResult{
		{Name: "x", Intent: "", Metrics: Metrics{FilePrecision: 0.5, FileRecall: 0.5, FileF1: 0.5}},
	}
	out := SummarizeByIntent(results)
	if len(out) != 1 || out[0].Intent != "(unspecified)" {
		t.Errorf("expected one (unspecified) group, got %+v", out)
	}
}

func TestSummarizeByIntent_EmptyResults(t *testing.T) {
	t.Parallel()
	if got := SummarizeByIntent(nil); got != nil {
		t.Errorf("SummarizeByIntent(nil) = %v, want nil", got)
	}
}

// --- Report JSON shape (latency percentiles) ---

func TestReport_LatencyPercentilesInJSON(t *testing.T) {
	t.Parallel()
	rep := Report{
		SchemaVersion: 1,
		Results: []ScenarioResult{
			{
				Name: "x", Runs: 1, MatchMode: "overlap", Intent: "arch_explain",
				Metrics: Metrics{
					LatencyMS:    42,
					LatencyMSP50: 42,
					LatencyMSP95: 42,
					LatencyMSMax: 42,
				},
			},
		},
		IntentSummary: []IntentSummary{
			{Intent: "arch_explain", Count: 1, AvgPrecision: 0.5, AvgRecall: 0.5, AvgF1: 0.5, AvgLatencyMS: 42},
		},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, rep); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`"intent": "arch_explain"`,
		`"latency_ms_p50": 42`,
		`"latency_ms_p95": 42`,
		`"latency_ms_max": 42`,
		`"intent_summary"`,
		`"avg_latency_ms": 42`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in JSON:\n%s", want, out)
		}
	}
}

func approxEqReport(a, b float64) bool {
	if a > b {
		return a-b < 1e-9
	}
	return b-a < 1e-9
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
