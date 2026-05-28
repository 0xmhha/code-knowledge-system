package vocab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_RejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()
	_, err := New(Glossary{Version: 99})
	if err == nil {
		t.Fatal("New: want version error, got nil")
	}
}

func TestNew_RejectsEntryWithoutAliases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		g    Glossary
	}{
		{
			name: "nil aliases",
			g: Glossary{Version: 1, Entries: []Entry{{
				Canonical:    "genesis block",
				CodeKeywords: []string{"GenesisBlock"},
			}}},
		},
		{
			name: "all blank aliases",
			g: Glossary{Version: 1, Entries: []Entry{{
				Aliases:      []string{"", "  "},
				Canonical:    "genesis block",
				CodeKeywords: []string{"GenesisBlock"},
			}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(tc.g); err == nil {
				t.Fatalf("New: want error for %s, got nil", tc.name)
			}
		})
	}
}

func TestNew_VersionZeroAccepted(t *testing.T) {
	t.Parallel()
	// Version=0 is treated as unspecified (helpful for tests that
	// construct a Glossary literal without specifying the schema).
	r, err := New(Glossary{Entries: []Entry{{
		Aliases:      []string{"foo"},
		Canonical:    "foo",
		CodeKeywords: []string{"Foo"},
	}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if r.EntryCount() != 1 {
		t.Errorf("EntryCount = %d, want 1", r.EntryCount())
	}
}

func TestResolve_NilReceiverIsPassThrough(t *testing.T) {
	t.Parallel()
	var r *Resolver
	res := r.Resolve("anything")
	if res.Original != "anything" || res.Expanded != "anything" {
		t.Errorf("nil resolver did not pass through: %+v", res)
	}
	if len(res.MatchedKeywords) != 0 {
		t.Errorf("nil resolver matched: %v", res.MatchedKeywords)
	}
}

func TestResolve_EmptyGlossaryIsPassThrough(t *testing.T) {
	t.Parallel()
	r, _ := New(Glossary{Version: 1})
	res := r.Resolve("find genesis logic")
	if res.Expanded != "find genesis logic" {
		t.Errorf("Expanded = %q, want unchanged", res.Expanded)
	}
	if len(res.MatchedKeywords) != 0 {
		t.Errorf("MatchedKeywords = %v, want empty", res.MatchedKeywords)
	}
}

func TestResolve_MatchesSingleAlias(t *testing.T) {
	t.Parallel()
	r := mustResolver(t, Glossary{Version: 1, Entries: []Entry{{
		Aliases:      []string{"0번 블록", "genesis"},
		Canonical:    "genesis block",
		CodeKeywords: []string{"GenesisBlock", "InitGenesis"},
	}}})

	res := r.Resolve("0번 블록 어디서 만들어져?")
	if res.Original != "0번 블록 어디서 만들어져?" {
		t.Errorf("Original = %q", res.Original)
	}
	if !strings.Contains(res.Expanded, "GenesisBlock") {
		t.Errorf("Expanded = %q, want to contain GenesisBlock", res.Expanded)
	}
	if !strings.Contains(res.Expanded, "InitGenesis") {
		t.Errorf("Expanded = %q, want to contain InitGenesis", res.Expanded)
	}
	if len(res.MatchedEntries) != 1 {
		t.Errorf("MatchedEntries = %d, want 1", len(res.MatchedEntries))
	}
	if len(res.MatchedKeywords) != 2 {
		t.Errorf("MatchedKeywords = %v, want 2", res.MatchedKeywords)
	}
}

func TestResolve_CaseInsensitiveAlias(t *testing.T) {
	t.Parallel()
	r := mustResolver(t, Glossary{Version: 1, Entries: []Entry{{
		Aliases:      []string{"Finalize"},
		Canonical:    "finalize",
		CodeKeywords: []string{"WBFT.Finalize"},
	}}})

	res := r.Resolve("why does FINALIZE fail when quorum drops?")
	if len(res.MatchedKeywords) != 1 || res.MatchedKeywords[0] != "WBFT.Finalize" {
		t.Errorf("MatchedKeywords = %v, want [WBFT.Finalize]", res.MatchedKeywords)
	}
}

func TestResolve_MultipleEntriesAccumulate(t *testing.T) {
	t.Parallel()
	r := mustResolver(t, Glossary{Version: 1, Entries: []Entry{
		{
			Aliases:      []string{"genesis"},
			Canonical:    "genesis block",
			CodeKeywords: []string{"GenesisBlock"},
		},
		{
			Aliases:      []string{"consensus", "합의"},
			Canonical:    "consensus",
			CodeKeywords: []string{"WBFT", "Finalize"},
		},
	}})

	res := r.Resolve("genesis 블록 합의 처리")
	if len(res.MatchedEntries) != 2 {
		t.Errorf("MatchedEntries = %d, want 2", len(res.MatchedEntries))
	}
	want := []string{"GenesisBlock", "WBFT", "Finalize"}
	if !equalSlices(res.MatchedKeywords, want) {
		t.Errorf("MatchedKeywords = %v, want %v", res.MatchedKeywords, want)
	}
}

func TestResolve_DedupesKeywordsAcrossEntries(t *testing.T) {
	t.Parallel()
	// Two entries both surface "Finalize" — output keeps it exactly once.
	r := mustResolver(t, Glossary{Version: 1, Entries: []Entry{
		{Aliases: []string{"consensus"}, Canonical: "consensus", CodeKeywords: []string{"Finalize", "WBFT"}},
		{Aliases: []string{"quorum"}, Canonical: "quorum check", CodeKeywords: []string{"verifyVotes", "Finalize"}},
	}})

	res := r.Resolve("consensus quorum failure")
	got := res.MatchedKeywords
	want := []string{"Finalize", "WBFT", "verifyVotes"}
	if !equalSlices(got, want) {
		t.Errorf("MatchedKeywords = %v, want %v (dedup)", got, want)
	}
}

func TestResolve_NoMatchReturnsOriginal(t *testing.T) {
	t.Parallel()
	r := mustResolver(t, Glossary{Version: 1, Entries: []Entry{{
		Aliases:      []string{"genesis"},
		Canonical:    "genesis",
		CodeKeywords: []string{"GenesisBlock"},
	}}})

	res := r.Resolve("totally unrelated text")
	if res.Expanded != res.Original {
		t.Errorf("Expanded = %q, want unchanged", res.Expanded)
	}
	if len(res.MatchedKeywords) != 0 {
		t.Errorf("MatchedKeywords = %v, want empty", res.MatchedKeywords)
	}
}

func TestLoad_ReadsYAMLFile(t *testing.T) {
	t.Parallel()
	yamlContent := `version: 1
entries:
  - aliases: ["genesis", "0번 블록"]
    canonical: "genesis block"
    code_keywords: ["GenesisBlock", "InitGenesis"]
  - aliases: ["consensus", "합의"]
    canonical: "consensus"
    code_keywords: ["WBFT", "Finalize"]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "glossary.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if r.EntryCount() != 2 {
		t.Errorf("EntryCount = %d, want 2", r.EntryCount())
	}
	res := r.Resolve("genesis 블록 합의 처리")
	if len(res.MatchedKeywords) != 4 {
		t.Errorf("MatchedKeywords = %v, want 4", res.MatchedKeywords)
	}
}

func TestLoad_RejectsMissingFile(t *testing.T) {
	t.Parallel()
	_, err := Load("/no/such/glossary.yaml")
	if err == nil {
		t.Fatal("Load: want error for missing file, got nil")
	}
}

func TestLoad_RejectsEmptyPath(t *testing.T) {
	t.Parallel()
	_, err := Load("")
	if err == nil {
		t.Fatal("Load: want error for empty path, got nil")
	}
}

// --- helpers ---

func mustResolver(t *testing.T, g Glossary) *Resolver {
	t.Helper()
	r, err := New(g)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
