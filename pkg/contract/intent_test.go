package contract

import (
	"encoding/json"
	"testing"
)

func TestIntent_String(t *testing.T) {
	t.Parallel()
	cases := map[Intent]string{
		IntentUnknown:           "",
		IntentBugFix:            "bug_fix",
		IntentFeatureAdd:        "feature_add",
		IntentRefactor:          "refactor",
		IntentArchExplain:       "arch_explain",
		IntentTestAdd:           "test_add",
		IntentConcurrencySafety: "concurrency_safety",
		IntentSecurity:          "security",
		IntentDocsUpdate:        "docs_update",
	}
	for i, want := range cases {
		if got := i.String(); got != want {
			t.Errorf("Intent(%v).String() = %q, want %q", i, got, want)
		}
	}
}

func TestIntent_IsValid(t *testing.T) {
	t.Parallel()
	for _, i := range AllIntents() {
		if !i.IsValid() {
			t.Errorf("AllIntents() includes invalid %q", i)
		}
	}
	if Intent("not_a_real_intent").IsValid() {
		t.Error("unknown string passed IsValid")
	}
}

func TestParseIntent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in    string
		want  Intent
		known bool
	}{
		{"bug_fix", IntentBugFix, true},
		{"security", IntentSecurity, true},
		{"", IntentUnknown, true}, // empty string is the valid Unknown value
		{"Bug_Fix", IntentUnknown, false},
		{"not_real", IntentUnknown, false},
	}
	for _, tc := range cases {
		got, known := ParseIntent(tc.in)
		if got != tc.want || known != tc.known {
			t.Errorf("ParseIntent(%q) = (%v, %v), want (%v, %v)", tc.in, got, known, tc.want, tc.known)
		}
	}
}

func TestIntent_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	type wrapper struct {
		I Intent `json:"i"`
	}

	for _, i := range AllIntents() {
		w := wrapper{I: i}
		buf, err := json.Marshal(w)
		if err != nil {
			t.Fatalf("marshal %v: %v", i, err)
		}
		var got wrapper
		if err := json.Unmarshal(buf, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", buf, err)
		}
		if got.I != i {
			t.Errorf("round-trip %q: got %q", i, got.I)
		}
	}
}

func TestAllIntents_NoMutation(t *testing.T) {
	t.Parallel()
	a := AllIntents()
	b := AllIntents()
	if len(a) != len(b) {
		t.Fatalf("AllIntents lengths differ: %d vs %d", len(a), len(b))
	}
	a[0] = IntentSecurity // mutate caller's copy
	if b[0] != IntentUnknown {
		t.Fatal("AllIntents leaks internal slice; mutation should not affect new caller")
	}
}
