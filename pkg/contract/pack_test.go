package contract

import (
	"encoding/json"
	"strings"
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
		GraphNeighbors: []Neighbor{
			{Source: c1, Target: c2, Relation: RelationCalls, Distance: 1},
		},
		SanitizeReport: []Redaction{
			{RuleID: "SANITIZE_PII_EMAIL", Action: RedactionDrop},
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
		"neighbor source not in citations": {
			Query:     "q",
			Intent:    IntentUnknown,
			Citations: []Citation{good},
			GraphNeighbors: []Neighbor{
				{Source: stray, Target: good, Relation: RelationCalls, Distance: 1},
			},
		},
		"neighbor target not in citations": {
			Query:     "q",
			Intent:    IntentUnknown,
			Citations: []Citation{good},
			GraphNeighbors: []Neighbor{
				{Source: good, Target: stray, Relation: RelationCalls, Distance: 1},
			},
		},
		"neighbor unknown relation": {
			Query:     "q",
			Intent:    IntentUnknown,
			Citations: []Citation{good},
			GraphNeighbors: []Neighbor{
				{Source: good, Target: good, Relation: Relation("invented"), Distance: 1},
			},
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
			{RuleID: "API_KEY", Action: RedactionDrop, Excerpt: "sk-* form, line 42"},
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
	if !out.Metadata.BuiltAt.Equal(in.Metadata.BuiltAt) {
		t.Errorf("BuiltAt differs: got %v, want %v", out.Metadata.BuiltAt, in.Metadata.BuiltAt)
	}
}

func TestNeighbor_IsValid(t *testing.T) {
	t.Parallel()
	good := goodCitation("a.go", 1, 1)
	bad := Citation{}

	cases := []struct {
		n    Neighbor
		want bool
	}{
		{Neighbor{Source: good, Target: good, Relation: RelationCalls, Distance: 1}, true},
		{Neighbor{Source: good, Target: good, Relation: RelationEmbeds, Distance: 2}, true},
		{Neighbor{Source: bad, Target: good, Relation: RelationCalls, Distance: 1}, false},
		{Neighbor{Source: good, Target: bad, Relation: RelationCalls, Distance: 1}, false},
		{Neighbor{Source: good, Target: good, Relation: RelationCalls, Distance: 0}, false},
		{Neighbor{Source: good, Target: good, Relation: Relation("invented"), Distance: 1}, false},
	}
	for _, tc := range cases {
		if got := tc.n.IsValid(); got != tc.want {
			t.Errorf("IsValid(%+v) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

// --- Integrity hash ---

func basePack(t *testing.T) EvidencePack {
	t.Helper()
	c := goodCitation("a.go", 1, 10)
	return EvidencePack{
		Intent:    IntentBugFix,
		Query:     "fix X",
		Citations: []Citation{c},
		Bodies:    []Body{{Citation: c, Text: "func foo() {}", TokenEstimate: 5}},
		Metadata: PackMetadata{
			BudgetTokens: 8000,
			UsedTokens:   42,
			BuiltAt:      time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
		},
	}
}

func TestStampIntegrity_PopulatesFields(t *testing.T) {
	t.Parallel()
	p := basePack(t)
	if err := StampIntegrity(&p); err != nil {
		t.Fatalf("StampIntegrity: %v", err)
	}
	if p.Metadata.IntegrityHash == "" {
		t.Error("IntegrityHash empty after stamp")
	}
	if p.Metadata.IntegrityHashAlgo != IntegrityHashAlgoSHA256 {
		t.Errorf("IntegrityHashAlgo = %q, want sha256", p.Metadata.IntegrityHashAlgo)
	}
	if len(p.Metadata.IntegrityHash) != 64 {
		t.Errorf("hex SHA-256 length = %d, want 64", len(p.Metadata.IntegrityHash))
	}
}

func TestVerifyIntegrity_CleanPasses(t *testing.T) {
	t.Parallel()
	p := basePack(t)
	if err := StampIntegrity(&p); err != nil {
		t.Fatalf("StampIntegrity: %v", err)
	}
	ok, err := VerifyIntegrity(p)
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	if !ok {
		t.Fatal("clean pack failed VerifyIntegrity")
	}
}

func TestVerifyIntegrity_DetectsTamperOnBody(t *testing.T) {
	t.Parallel()
	p := basePack(t)
	if err := StampIntegrity(&p); err != nil {
		t.Fatalf("StampIntegrity: %v", err)
	}
	// Tamper: silently change body text without re-stamping.
	p.Bodies[0].Text = "func foo() { evil() }"
	ok, err := VerifyIntegrity(p)
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	if ok {
		t.Fatal("tampered pack passed VerifyIntegrity; want false")
	}
}

func TestVerifyIntegrity_DetectsTamperOnMetadata(t *testing.T) {
	t.Parallel()
	p := basePack(t)
	if err := StampIntegrity(&p); err != nil {
		t.Fatalf("StampIntegrity: %v", err)
	}
	// Tamper: bump UsedTokens to hide budget overrun.
	p.Metadata.UsedTokens = 1
	ok, err := VerifyIntegrity(p)
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	if ok {
		t.Fatal("tampered metadata passed VerifyIntegrity")
	}
}

func TestVerifyIntegrity_NoHashErr(t *testing.T) {
	t.Parallel()
	p := basePack(t) // not stamped
	_, err := VerifyIntegrity(p)
	if err == nil || !strings.Contains(err.Error(), "no integrity_hash") {
		t.Fatalf("VerifyIntegrity unstamped = %v, want 'no integrity_hash' error", err)
	}
}

func TestVerifyIntegrity_UnknownAlgoErr(t *testing.T) {
	t.Parallel()
	p := basePack(t)
	if err := StampIntegrity(&p); err != nil {
		t.Fatalf("StampIntegrity: %v", err)
	}
	p.Metadata.IntegrityHashAlgo = "md5_lol"
	_, err := VerifyIntegrity(p)
	if err == nil || !strings.Contains(err.Error(), "unsupported integrity_hash_algo") {
		t.Fatalf("VerifyIntegrity bad algo = %v, want unsupported error", err)
	}
}

func TestComputeIntegrityHash_Deterministic(t *testing.T) {
	t.Parallel()
	p1 := basePack(t)
	p2 := basePack(t)
	h1, err := ComputeIntegrityHash(p1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ComputeIntegrityHash(p2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("identical packs hash differently: %q vs %q", h1, h2)
	}
}

func TestComputeIntegrityHash_IgnoresExistingHashField(t *testing.T) {
	t.Parallel()
	p1 := basePack(t)
	p2 := basePack(t)
	// Pre-populate p2 with junk hash fields. ComputeIntegrityHash must
	// zero these before computing, so p1 and p2 still hash the same.
	p2.Metadata.IntegrityHash = "junkhash"
	p2.Metadata.IntegrityHashAlgo = "junkalgo"
	h1, err := ComputeIntegrityHash(p1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ComputeIntegrityHash(p2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("pre-existing hash field affected ComputeIntegrityHash: %q vs %q", h1, h2)
	}
}

func TestStampIntegrity_NilPack(t *testing.T) {
	t.Parallel()
	if err := StampIntegrity(nil); err == nil {
		t.Fatal("StampIntegrity(nil) returned nil; want error")
	}
}
