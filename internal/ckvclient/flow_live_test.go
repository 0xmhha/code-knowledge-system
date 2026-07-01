package ckvclient_test

// Env-gated live integration check for the Phase D flow surface (T6). It opens
// the REAL ckv index in-process (needs a live Ollama bge-m3) and exercises the
// four flow methods end to end, so it is skipped unless CKS_FLOW_LIVE_CKV points
// at a ckv-data directory. Run it against the pr-77-2 flow index:
//
//	CKS_FLOW_LIVE_CKV=/Users/.../knowledge-data/pr-77-2/ckv \
//	  go test ./internal/ckvclient/ -run TestLive_Flow -v
//
// Optional overrides: CKS_FLOW_LIVE_OLLAMA (default http://localhost:11434),
// CKS_FLOW_LIVE_MODEL (default bge-m3), CKS_FLOW_LIVE_FLOW (default ep-cli-init),
// CKS_FLOW_LIVE_INV (default INV-CONSENSUS-01), CKS_FLOW_LIVE_SYMPTOM
// (default "정족수 부족").

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/embedder"
)

func envOr(key, dflt string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return dflt
}

func TestLive_Flow(t *testing.T) {
	dataPath := os.Getenv("CKS_FLOW_LIVE_CKV")
	if dataPath == "" {
		t.Skip("set CKS_FLOW_LIVE_CKV=<ckv-data dir> to run the live flow check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	emb, cap, err := embedder.Open("ollama", envOr("CKS_FLOW_LIVE_MODEL", "bge-m3"), envOr("CKS_FLOW_LIVE_OLLAMA", "http://localhost:11434"))
	if err != nil {
		t.Fatalf("embedder.Open: %v", err)
	}
	t.Logf("embedder: %s/%s dim=%d", cap.Provider, cap.Model, cap.Dim)

	real, err := ckvclient.NewReal(ctx, ckvclient.RealOpts{DataPath: dataPath, Embedder: emb})
	if err != nil {
		t.Fatalf("NewReal(%q): %v", dataPath, err)
	}
	defer real.Close()

	// 1. GetFlow — expect a non-empty step sequence for the known flow.
	flowID := envOr("CKS_FLOW_LIVE_FLOW", "ep-cli-init")
	flow, err := real.GetFlow(ctx, ckvclient.FlowQuery{FlowID: flowID})
	if err != nil {
		t.Fatalf("GetFlow(%q): %v", flowID, err)
	}
	if len(flow.Steps) == 0 {
		t.Fatalf("GetFlow(%q): no steps", flowID)
	}
	t.Logf("GetFlow %q: %d steps, root=%s entry=%s", flow.FlowID, len(flow.Steps), flow.RootSymbol, flow.EntryPoint)
	for i, s := range flow.Steps {
		t.Logf("  step[%d] %s %s @ %s:%d-%d", i, s.StepID, s.Symbol, s.Citation.File, s.Citation.StartLine, s.Citation.EndLine)
	}

	// MaxSteps cap is applied cks-side.
	if capped, err := real.GetFlow(ctx, ckvclient.FlowQuery{FlowID: flowID, MaxSteps: 1}); err != nil {
		t.Errorf("GetFlow MaxSteps: %v", err)
	} else if len(capped.Steps) != 1 {
		t.Errorf("MaxSteps=1 not applied: got %d steps", len(capped.Steps))
	}

	// 2. ExpandFlow — walk down from the first step.
	origin := flow.Steps[0].StepID
	exp, err := real.ExpandFlow(ctx, ckvclient.ExpandFlowQuery{StepID: origin, Direction: "down", Hops: 1})
	if err != nil {
		t.Fatalf("ExpandFlow(%q): %v", origin, err)
	}
	t.Logf("ExpandFlow %q down: %d neighbors, %d origin-branches", exp.Origin, len(exp.Neighbors), len(exp.OriginBranches))
	for _, n := range exp.Neighbors {
		t.Logf("  neighbor %s %s (%s)", n.StepID, n.Symbol, n.Relation)
	}

	// 3. GetInvariantEnforcement — expect enforcement sites for the known invariant.
	invID := envOr("CKS_FLOW_LIVE_INV", "INV-CONSENSUS-01")
	inv, err := real.GetInvariantEnforcement(ctx, invID, 0)
	if err != nil {
		t.Fatalf("GetInvariantEnforcement(%q): %v", invID, err)
	}
	t.Logf("GetInvariantEnforcement %q: %q, %d sites", inv.InvID, inv.Statement, len(inv.EnforcedAt))
	for _, e := range inv.EnforcedAt {
		t.Logf("  enforced @ flow=%s step=%s loc=%s", e.Flow, e.Step, e.Loc)
	}
	if len(inv.EnforcedAt) == 0 {
		t.Errorf("GetInvariantEnforcement(%q): no enforcement sites", invID)
	}

	// 4. FindBranches — symptom→cause over the flow corpus (needs the embedder).
	symptom := envOr("CKS_FLOW_LIVE_SYMPTOM", "정족수 부족")
	matches, err := real.FindBranches(ctx, symptom, 5)
	if err != nil {
		t.Fatalf("FindBranches(%q): %v", symptom, err)
	}
	t.Logf("FindBranches %q: %d matches", symptom, len(matches))
	for _, m := range matches {
		t.Logf("  when=%q then=%q at=%s (score=%.3f) %s", m.When, m.Then, m.At, m.Score, m.Symbol)
	}
	if len(matches) == 0 {
		t.Errorf("FindBranches(%q): no matches", symptom)
	}
}
