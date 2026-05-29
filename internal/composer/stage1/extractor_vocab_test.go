package stage1

import (
	"context"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/vocab"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// TestExtract_NoVocab_VerbatimQuery confirms that without a resolver the
// query the ckv backend sees equals the original prompt — no surprise
// rewriting when vocab is opt-out.
func TestExtract_NoVocab_VerbatimQuery(t *testing.T) {
	t.Parallel()
	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{hit("a.go", 1, 0.9, contract.HitSourceCKV)},
	}
	e, err := New(ckv, &ckgclient.Fake{})
	if err != nil {
		t.Fatal(err)
	}

	out, err := e.Extract(context.Background(), "find the validator quorum check", contract.IntentBugFix)
	if err != nil {
		t.Fatal(err)
	}
	if out.VocabExpanded {
		t.Errorf("VocabExpanded = true with no resolver, want false")
	}
	if len(out.VocabKeywords) != 0 {
		t.Errorf("VocabKeywords = %v, want empty", out.VocabKeywords)
	}
	if len(ckv.Calls.SemanticSearch) == 0 {
		t.Fatal("expected at least one SemanticSearch call")
	}
	got := ckv.Calls.SemanticSearch[0].Query
	if got != "find the validator quorum check" {
		t.Errorf("ckv query = %q, want verbatim prompt", got)
	}
}

// TestExtract_VocabExpands_QueryContainsKeywords confirms that a matched
// glossary entry appends its code keywords to the query the ckv backend
// sees, and records the expansion on Stage1Output.
func TestExtract_VocabExpands_QueryContainsKeywords(t *testing.T) {
	t.Parallel()
	r, err := vocab.New(vocab.Glossary{
		Version: 1,
		Entries: []vocab.Entry{{
			Aliases:      []string{"쿼럼", "consensus threshold"},
			Canonical:    "WBFT quorum",
			CodeKeywords: []string{"QuorumSize", "WBFTPrepares"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{hit("a.go", 1, 0.9, contract.HitSourceCKV)},
	}
	e, err := New(ckv, &ckgclient.Fake{}, WithVocab(r))
	if err != nil {
		t.Fatal(err)
	}

	out, err := e.Extract(context.Background(), "쿼럼 미달이면 어떻게 처리되나", contract.IntentBugFix)
	if err != nil {
		t.Fatal(err)
	}
	if !out.VocabExpanded {
		t.Errorf("VocabExpanded = false, want true")
	}
	if len(out.VocabKeywords) != 2 || out.VocabKeywords[0] != "QuorumSize" || out.VocabKeywords[1] != "WBFTPrepares" {
		t.Errorf("VocabKeywords = %v, want [QuorumSize WBFTPrepares]", out.VocabKeywords)
	}
	if len(ckv.Calls.SemanticSearch) == 0 {
		t.Fatal("expected at least one SemanticSearch call")
	}
	q := ckv.Calls.SemanticSearch[0].Query
	if !strings.Contains(q, "QuorumSize") || !strings.Contains(q, "WBFTPrepares") {
		t.Errorf("ckv query missing expanded keywords: %q", q)
	}
	if !strings.HasPrefix(q, "쿼럼 미달이면 어떻게 처리되나") {
		t.Errorf("ckv query lost original prompt prefix: %q", q)
	}
}

// TestExtract_VocabNoMatch_VerbatimQuery confirms that a wired resolver
// with no matching aliases is a no-op — query equals the verbatim
// prompt, no flag flip, no keywords leaked.
func TestExtract_VocabNoMatch_VerbatimQuery(t *testing.T) {
	t.Parallel()
	r, err := vocab.New(vocab.Glossary{
		Version: 1,
		Entries: []vocab.Entry{{
			Aliases:      []string{"a-thing-no-prompt-mentions"},
			Canonical:    "thing",
			CodeKeywords: []string{"ThingKeyword"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{hit("a.go", 1, 0.9, contract.HitSourceCKV)},
	}
	e, err := New(ckv, &ckgclient.Fake{}, WithVocab(r))
	if err != nil {
		t.Fatal(err)
	}

	out, err := e.Extract(context.Background(), "totally unrelated prompt", contract.IntentBugFix)
	if err != nil {
		t.Fatal(err)
	}
	if out.VocabExpanded {
		t.Errorf("VocabExpanded = true with no alias match, want false")
	}
	if len(out.VocabKeywords) != 0 {
		t.Errorf("VocabKeywords = %v, want empty", out.VocabKeywords)
	}
	q := ckv.Calls.SemanticSearch[0].Query
	if q != "totally unrelated prompt" {
		t.Errorf("ckv query = %q, want verbatim prompt", q)
	}
}
