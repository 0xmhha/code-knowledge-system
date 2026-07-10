package stage1

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func TestExtract_KnowledgePassIssuesKindScopedSearch(t *testing.T) {
	t.Parallel()
	// KnowledgeK>0 adds exactly one extra ckv search after the recall
	// rounds, scoped to knowledge chunk kinds. The pass must not affect
	// round accounting and its failure must not fail Extract.
	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{hit("handlers.go", 1, 0.9, contract.HitSourceCKV)},
	}
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{hit("handlers.go", 1, 10.0, contract.HitSourceCKG)},
	}
	e, _ := New(ckv, ckg, WithConfig(Config{
		MaxRounds:     1,
		InitialK:      DefaultInitialK,
		RerankPerKW:   DefaultRerankPerKW,
		MinConfidence: DefaultMinConfidence,
		MaxKeywords:   DefaultMaxKeywords,
		AugmentTopN:   DefaultAugmentTopN,
		KnowledgeK:    2,
	}))

	out, err := e.Extract(context.Background(), "fix the Login handler", contract.IntentBugFix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if out.Rounds != 1 {
		t.Errorf("Rounds = %d, want 1 (knowledge pass is not a round)", out.Rounds)
	}
	calls := ckv.Calls.SemanticSearch
	if len(calls) != 2 {
		t.Fatalf("SemanticSearch calls = %d, want 2 (1 round + 1 knowledge pass)", len(calls))
	}
	kp := calls[len(calls)-1]
	if kp.Opts.K != 2 {
		t.Errorf("knowledge pass K = %d, want 2", kp.Opts.K)
	}
	kinds := kp.Opts.Filter.ChunkKinds
	if len(kinds) != 2 || kinds[0] != "invariant" || kinds[1] != "convention" {
		t.Errorf("knowledge pass ChunkKinds = %v, want [invariant convention]", kinds)
	}
	// Fake returns the same citation for both calls; mergeHits dedupes.
	if len(out.Hits) != 1 {
		t.Errorf("Hits = %d, want 1 (deduped)", len(out.Hits))
	}
	if out.KnowledgeHits != 1 {
		t.Errorf("KnowledgeHits = %d, want 1", out.KnowledgeHits)
	}
}

func TestExtract_KnowledgePassDisabledByZeroK(t *testing.T) {
	t.Parallel()
	ckv := &ckvclient.Fake{
		SearchHits: []contract.Hit{hit("handlers.go", 1, 0.9, contract.HitSourceCKV)},
	}
	ckg := &ckgclient.Fake{
		BM25Hits: []contract.Hit{hit("handlers.go", 1, 10.0, contract.HitSourceCKG)},
	}
	e, _ := New(ckv, ckg, WithConfig(Config{
		MaxRounds:     1,
		InitialK:      DefaultInitialK,
		RerankPerKW:   DefaultRerankPerKW,
		MinConfidence: DefaultMinConfidence,
		MaxKeywords:   DefaultMaxKeywords,
		AugmentTopN:   DefaultAugmentTopN,
	}))
	if _, err := e.Extract(context.Background(), "fix the Login handler", contract.IntentBugFix); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if n := len(ckv.Calls.SemanticSearch); n != 1 {
		t.Errorf("SemanticSearch calls = %d, want 1 (KnowledgeK=0 disables the pass)", n)
	}
}

func TestDefaultConfig_KnowledgeK(t *testing.T) {
	t.Parallel()
	if DefaultConfig().KnowledgeK != 6 {
		t.Errorf("DefaultConfig.KnowledgeK = %d, want 6", DefaultConfig().KnowledgeK)
	}
}
