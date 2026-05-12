package contract

import (
	"encoding/json"
	"testing"
)

func TestCitation_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		c    Citation
		want string
	}{
		{Citation{File: "a.go", StartLine: 10, EndLine: 10}, "a.go:10"},
		{Citation{File: "a.go", StartLine: 10, EndLine: 20}, "a.go:10-20"},
		{Citation{File: "a.go", StartLine: 10, EndLine: 10, CommitHash: "abcd"}, "a.go:10@abcd"},
		{Citation{File: "a.go", StartLine: 10, EndLine: 20, CommitHash: "abcd"}, "a.go:10-20@abcd"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("String(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}

func TestCitation_IsValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		c    Citation
		want bool
	}{
		{Citation{File: "a.go", StartLine: 1, EndLine: 1}, true},
		{Citation{File: "a.go", StartLine: 1, EndLine: 100}, true},
		{Citation{File: "", StartLine: 1, EndLine: 1}, false},
		{Citation{File: "a.go", StartLine: 0, EndLine: 1}, false},
		{Citation{File: "a.go", StartLine: 1, EndLine: 0}, false},
		{Citation{File: "a.go", StartLine: 10, EndLine: 5}, false}, // inverted
		{Citation{File: "a.go", StartLine: -1, EndLine: 1}, false},
	}
	for _, tc := range cases {
		if got := tc.c.IsValid(); got != tc.want {
			t.Errorf("IsValid(%+v) = %v, want %v", tc.c, got, tc.want)
		}
	}
}

func TestCitation_Key_DedupsAcrossCommitHash(t *testing.T) {
	t.Parallel()
	c1 := Citation{File: "a.go", StartLine: 10, EndLine: 20, CommitHash: "aaa"}
	c2 := Citation{File: "a.go", StartLine: 10, EndLine: 20, CommitHash: "bbb"}
	if c1.Key() != c2.Key() {
		t.Errorf("expected identical Key for same span, got %q vs %q", c1.Key(), c2.Key())
	}
}

func TestCitation_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	in := Citation{File: "internal/composer.go", StartLine: 42, EndLine: 58, CommitHash: "deadbeef"}
	buf, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"file":"internal/composer.go","start_line":42,"end_line":58,"commit_hash":"deadbeef"}`
	if string(buf) != want {
		t.Fatalf("marshal output mismatch:\n got: %s\nwant: %s", buf, want)
	}
	var out Citation
	if err := json.Unmarshal(buf, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round-trip: got %+v, want %+v", out, in)
	}
}

func TestCitation_JSONIncludesEmptyCommitHash(t *testing.T) {
	t.Parallel()
	in := Citation{File: "a.go", StartLine: 1, EndLine: 1}
	buf, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// commit_hash is always present in JSON output (no omitempty) — explicit
	// over implicit, so consumers never see schema variants based on
	// optional fields.
	want := `{"file":"a.go","start_line":1,"end_line":1,"commit_hash":""}`
	if string(buf) != want {
		t.Fatalf("marshal output mismatch:\n got: %s\nwant: %s", buf, want)
	}
}
