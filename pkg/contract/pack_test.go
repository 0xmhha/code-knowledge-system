package contract

import (
	"encoding/json"
	"testing"
	"time"
)

func goodCitation(file string, start, end int) Citation {
	return Citation{File: file, StartLine: start, EndLine: end, CommitHash: "deadbeef"}
}

func TestEvidencePack_IsValid_OK(t *testing.T) {
	t.Parallel()
	c1 := goodCitation("a.go", 1, 10)
	c2 := goodCitation("b.go", 20, 30)
	p := EvidencePack{
		Intent:    IntentBugFix,
		Query:     "fix null pointer in handler",
		Citations: []Citation{c1, c2},
		Bodies: []Body{
			{Citation: c1, Text: "func handler() {...}", TokenEstimate: 25},
		},
		SanitizeReport: []Redaction{
			{RuleID: "SANITIZE_PII_EMAIL", Action: RedactionMask},
		},
		Metadata: PackMetadata{
			BudgetTokens: 8000,
			UsedTokens:   1234,
			BuiltAt:      time.Now().UTC(),
		},
	}
	if !p.IsValid() {
		t.Fatal("expected IsValid()=true for well-formed pack")
	}
}

func TestEvidencePack_IsValid_RejectsCases(t *testing.T) {
	t.Parallel()
	good := goodCitation("a.go", 1, 10)
	stray := goodCitation("not_in_citations.go", 1, 1)

	cases := map[string]EvidencePack{
		"empty query": {
			Citations: []Citation{good},
		},
		"unknown intent": {
			Query:     "q",
			Intent:    Intent("made_up"),
			Citations: []Citation{good},
		},
		"bad citation": {
			Query:     "q",
			Intent:    IntentUnknown,
			Citations: []Citation{{File: "", StartLine: 1, EndLine: 1}},
		},
		"body without matching citation": {
			Query:     "q",
			Intent:    IntentUnknown,
			Citations: []Citation{good},
			Bodies:    []Body{{Citation: stray, Text: "x"}},
		},
		"unknown sanitize action": {
			Query:          "q",
			Intent:         IntentUnknown,
			Citations:      []Citation{good},
			SanitizeReport: []Redaction{{RuleID: "X", Action: "wat"}},
		},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if p.IsValid() {
				t.Fatalf("expected IsValid()=false for %s, pack=%+v", name, p)
			}
		})
	}
}

func TestEvidencePack_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	c := goodCitation("a.go", 1, 10)
	in := EvidencePack{
		Intent:    IntentSecurity,
		Query:     "audit input boundary",
		Citations: []Citation{c},
		Bodies:    []Body{{Citation: c, Text: "// ...", TokenEstimate: 1}},
		SanitizeReport: []Redaction{
			{RuleID: "API_KEY", Action: RedactionDrop, Excerpt: "sk-..."},
		},
		Metadata: PackMetadata{
			BudgetTokens:     8000,
			UsedTokens:       42,
			UtilizationRatio: 42.0 / 8000.0,
			BuiltAt:          time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
			BuilderVersion:   "cks/0.0.1-dev",
			CKGSchemaVersion: "v1",
		},
	}
	buf, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out EvidencePack
	if err := json.Unmarshal(buf, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.IsValid() {
		t.Fatal("round-tripped pack failed IsValid()")
	}
	// time comparison via Equal because of monotonic clock stripping over JSON
	if !out.Metadata.BuiltAt.Equal(in.Metadata.BuiltAt) {
		t.Errorf("BuiltAt differs: got %v, want %v", out.Metadata.BuiltAt, in.Metadata.BuiltAt)
	}
	if out.Intent != in.Intent || out.Query != in.Query {
		t.Errorf("Intent/Query mismatch: %+v vs %+v", out, in)
	}
	if len(out.Citations) != 1 || out.Citations[0] != c {
		t.Errorf("Citations mismatch: %+v", out.Citations)
	}
	if len(out.SanitizeReport) != 1 || out.SanitizeReport[0].Action != RedactionDrop {
		t.Errorf("SanitizeReport mismatch: %+v", out.SanitizeReport)
	}
}
