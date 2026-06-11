package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRetrievalStepKind_IsValid(t *testing.T) {
	t.Parallel()
	valid := []RetrievalStepKind{
		StepCKVRecall, StepCKGBM25, StepCKGFindSymbol, StepCKGSubgraph, StepCKGImpact,
	}
	for _, k := range valid {
		if !k.IsValid() {
			t.Errorf("known kind %q failed IsValid", k)
		}
	}
	if RetrievalStepKind("ckg.find_callers").IsValid() {
		t.Error("not-yet-added kind passed IsValid")
	}
	if RetrievalStepKind("").IsValid() {
		t.Error("empty kind passed IsValid")
	}
}

func validTrace() RetrievalTrace {
	return RetrievalTrace{
		Producer:      "composer",
		Intent:        IntentBugFix,
		Prompt:        "수수료 위임 검증은 어디서?",
		VocabExpanded: true,
		VocabKeywords: []string{"FeePayer"},
		Steps: []RetrievalStep{
			{
				N:          1,
				Kind:       StepCKVRecall,
				Query:      "수수료 위임 검증 FeePayer",
				Source:     HitSourceCKV,
				Keywords:   []string{"FeePayer", "ValidateTransactionWithState"},
				Confidence: 0.71,
				Decision:   "accept",
			},
			{
				N:      2,
				Kind:   StepCKGSubgraph,
				Query:  "core/txpool.ValidateTransactionWithState",
				Source: HitSourceCKG,
			},
		},
		FinalSeeds: []Citation{
			{File: "core/txpool/validation.go", StartLine: 236, EndLine: 236, CommitHash: ""},
		},
		Rounds:   1,
		CKVCalls: 1,
		CKGCalls: 1,
	}
}

func TestRetrievalTrace_IsValid(t *testing.T) {
	t.Parallel()

	if !validTrace().IsValid() {
		t.Fatal("a well-formed trace failed IsValid")
	}

	cases := map[string]func(*RetrievalTrace){
		"empty prompt":   func(tr *RetrievalTrace) { tr.Prompt = "" },
		"empty producer": func(tr *RetrievalTrace) { tr.Producer = "" },
		"no steps":       func(tr *RetrievalTrace) { tr.Steps = nil },
		"bad step kind":  func(tr *RetrievalTrace) { tr.Steps[0].Kind = "ckg.bogus" },
	}
	for name, mutate := range cases {
		tr := validTrace()
		mutate(&tr)
		if tr.IsValid() {
			t.Errorf("%s: expected IsValid()=false", name)
		}
	}
}

func TestRetrievalTrace_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	in := validTrace()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// snake_case keys are the stored contract — guard against accidental renames.
	for _, key := range []string{
		`"producer"`, `"intent"`, `"prompt"`, `"steps"`, `"final_seeds"`,
		`"ckv_calls"`, `"ckg_calls"`, `"vocab_expanded"`,
	} {
		if !strings.Contains(string(b), key) {
			t.Errorf("marshaled trace missing key %s\n%s", key, b)
		}
	}
	// omitempty: a zero TokensIn / FailedKeywords must not appear.
	if strings.Contains(string(b), `"tokens_in"`) {
		t.Error("zero tokens_in should be omitted")
	}
	if strings.Contains(string(b), `"failed_keywords"`) {
		t.Error("empty failed_keywords should be omitted")
	}

	var out RetrievalTrace
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.IsValid() {
		t.Error("round-tripped trace failed IsValid")
	}
	if out.Producer != in.Producer || len(out.Steps) != len(in.Steps) ||
		out.Steps[0].Kind != StepCKVRecall || out.FinalSeeds[0].StartLine != 236 {
		t.Errorf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}
