package composer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/composer/budget"
	"github.com/0xmhha/code-knowledge-system/internal/composer/intent"
	"github.com/0xmhha/code-knowledge-system/internal/composer/sanitize"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage1"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/internal/config"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// TestCompose_DummyBackendsPopulateInstructions wires the Composer with
// ckvclient.Dummy + ckgclient.Dummy and verifies that EvidencePack.Instructions
// captures every would-have-been backend call.
func TestCompose_DummyBackendsPopulateInstructions(t *testing.T) {
	ckv := ckvclient.NewDummy()
	ckg := ckgclient.NewDummy()
	embedder := &intent.FakeEmbedder{Dim: 16}
	fetcher := &budget.FakeFetcher{Bodies: map[string]string{}}
	ruleset := &config.SanitizeRuleset{
		Version: 1,
		Rules: []config.SanitizeRule{
			{ID: "NOOP", Pattern: `__no_match__`, Action: "drop", Severity: "low"},
		},
	}
	if err := ruleset.Validate(); err != nil {
		t.Fatalf("ruleset.Validate: %v", err)
	}

	ic, err := intent.New(context.Background(), embedder)
	if err != nil {
		t.Fatalf("intent.New: %v", err)
	}
	s1, err := stage1.New(ckv, ckg)
	if err != nil {
		t.Fatalf("stage1.New: %v", err)
	}
	s2, err := stage2.New(ckg)
	if err != nil {
		t.Fatalf("stage2.New: %v", err)
	}
	s3, err := stage3.New(ckg)
	if err != nil {
		t.Fatalf("stage3.New: %v", err)
	}
	b, err := budget.New(fetcher)
	if err != nil {
		t.Fatalf("budget.New: %v", err)
	}
	san, err := sanitize.New(ruleset)
	if err != nil {
		t.Fatalf("sanitize.New: %v", err)
	}
	c, err := New(ic, s1, s2, s3, b, san)
	if err != nil {
		t.Fatalf("composer.New: %v", err)
	}

	pack, err := c.Compose(context.Background(), "consensus failure handling in WBFT finalize path")
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	if len(pack.Instructions) == 0 {
		t.Fatalf("Instructions: got 0, want at least one (ckv SemanticSearch via stage1)")
	}

	// With unconfigured dummies the skill/source paths default to the current
	// working directory (see ckvclient/ckgclient Dummy.source/skill).
	wantSource, _ := os.Getwd()
	wantSkill := filepath.Join(wantSource, ".claude")

	// Every instruction must carry the derived skill + source paths and a
	// non-empty directive.
	seenBackends := map[string]bool{}
	for i, inst := range pack.Instructions {
		seenBackends[inst.Backend] = true
		if inst.SkillPath != wantSkill {
			t.Errorf("inst[%d].SkillPath: got %q, want %q", i, inst.SkillPath, wantSkill)
		}
		if inst.SourcePath != wantSource {
			t.Errorf("inst[%d].SourcePath: got %q, want %q", i, inst.SourcePath, wantSource)
		}
		if inst.Directive == "" {
			t.Errorf("inst[%d].Directive empty", i)
		}
		if !strings.Contains(inst.Directive, wantSource) {
			t.Errorf("inst[%d].Directive missing source path", i)
		}
	}

	// stage1 always calls ckv.SemanticSearch; with a dummy that's the
	// minimum we expect on this pipeline.
	if !seenBackends["ckv"] {
		t.Errorf("no ckv backend recorded; backends seen: %v", seenBackends)
	}

	// EvidencePack integrity hash should still verify even with
	// dummy-derived placeholder data.
	ok, err := contract.VerifyIntegrity(pack)
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	if !ok {
		t.Errorf("VerifyIntegrity: returned false")
	}
}
