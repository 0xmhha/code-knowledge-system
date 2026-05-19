package eval

import (
	"strings"
	"testing"
)

func TestParseScenario_HappyPath(t *testing.T) {
	t.Parallel()
	yaml := `
version: 1
name: stablenet-bft-quorum
description: BFT consensus quorum lookup
prompt: find the BFT consensus quorum logic
expected_citations:
  - file: consensus/bft.go
    start_line: 100
    end_line: 200
  - file: consensus/quorum.go
    start_line: 1
    end_line: 50
match_mode: overlap
runs: 3
`
	s, err := ParseScenario([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseScenario: %v", err)
	}
	if s.Name != "stablenet-bft-quorum" {
		t.Errorf("Name = %q", s.Name)
	}
	if s.Prompt != "find the BFT consensus quorum logic" {
		t.Errorf("Prompt = %q", s.Prompt)
	}
	if len(s.ExpectedCitations) != 2 {
		t.Fatalf("ExpectedCitations = %d, want 2", len(s.ExpectedCitations))
	}
	if s.ExpectedCitations[0].File != "consensus/bft.go" ||
		s.ExpectedCitations[0].StartLine != 100 ||
		s.ExpectedCitations[0].EndLine != 200 {
		t.Errorf("first citation wrong: %+v", s.ExpectedCitations[0])
	}
	if s.MatchMode != MatchOverlap {
		t.Errorf("MatchMode = %q", s.MatchMode)
	}
	if s.Runs != 3 {
		t.Errorf("Runs = %d", s.Runs)
	}
}

func TestParseScenario_DefaultsApplied(t *testing.T) {
	t.Parallel()
	// match_mode omitted → defaults to "overlap"; runs omitted → 1.
	yaml := `
version: 1
name: minimal
prompt: anything
`
	s, err := ParseScenario([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseScenario: %v", err)
	}
	if s.MatchMode != MatchOverlap {
		t.Errorf("MatchMode default = %q, want %q", s.MatchMode, MatchOverlap)
	}
	if s.Runs != 1 {
		t.Errorf("Runs default = %d, want 1", s.Runs)
	}
}

func TestParseScenario_RejectsWrongVersion(t *testing.T) {
	t.Parallel()
	yaml := `version: 2
name: x
prompt: y
`
	_, err := ParseScenario([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("err = %v, want version mismatch", err)
	}
}

func TestParseScenario_RejectsEmptyPrompt(t *testing.T) {
	t.Parallel()
	yaml := `version: 1
name: x
prompt: ""
`
	_, err := ParseScenario([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("err = %v, want prompt error", err)
	}
}

func TestParseScenario_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	yaml := `version: 1
prompt: x
`
	_, err := ParseScenario([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err = %v, want name error", err)
	}
}

func TestParseScenario_RejectsUnknownMatchMode(t *testing.T) {
	t.Parallel()
	yaml := `version: 1
name: x
prompt: y
match_mode: fuzzy
`
	_, err := ParseScenario([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "match_mode") {
		t.Fatalf("err = %v, want match_mode error", err)
	}
}

func TestParseScenario_RejectsNegativeRuns(t *testing.T) {
	t.Parallel()
	yaml := `version: 1
name: x
prompt: y
runs: -1
`
	_, err := ParseScenario([]byte(yaml))
	if err == nil || !strings.Contains(err.Error(), "runs") {
		t.Fatalf("err = %v, want runs error", err)
	}
}

func TestParseScenario_AcceptsKnownIntent(t *testing.T) {
	t.Parallel()
	yaml := `
version: 1
name: x
prompt: y
intent: arch_explain
`
	s, err := ParseScenario([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if s.Intent != "arch_explain" {
		t.Errorf("Intent = %q", s.Intent)
	}
}

func TestParseScenario_AcceptsEmptyIntent(t *testing.T) {
	t.Parallel()
	// Intent is optional. Scenarios without one group under
	// "(unspecified)" in the report (validated separately).
	yaml := `
version: 1
name: x
prompt: y
`
	s, err := ParseScenario([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if s.Intent != "" {
		t.Errorf("Intent = %q, want empty", s.Intent)
	}
}

func TestParseScenario_RejectsUnknownIntent(t *testing.T) {
	t.Parallel()
	yaml := `
version: 1
name: x
prompt: y
intent: scribble_thoughts
`
	_, err := ParseScenario([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unknown intent")
	}
}

func TestParseScenario_RejectsInvalidCitation(t *testing.T) {
	t.Parallel()
	yaml := `version: 1
name: x
prompt: y
expected_citations:
  - file: ""
    start_line: 1
    end_line: 5
`
	_, err := ParseScenario([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty citation file")
	}
}

func TestParseScenario_RejectsUnknownFields(t *testing.T) {
	t.Parallel()
	// KnownFields strict — typos like 'expectec_citations' must fail
	// rather than silently load with the field absent.
	yaml := `version: 1
name: x
prompt: y
expectec_citations: []
`
	_, err := ParseScenario([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for typo'd field")
	}
}
