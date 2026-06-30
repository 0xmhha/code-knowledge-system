package mcp

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func TestHandleGetFlow_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.FlowVal = ckvclient.Flow{
			FlowID:     "F1",
			EntryPoint: "Handle",
			Steps: []ckvclient.FlowStep{
				{StepID: "s1", Symbol: "A", Citation: cit("a.go", 1, 5)},
			},
		}
	})
	res, err := handleGetFlow(context.Background(), f.deps, callToolReq(map[string]any{"flow_id": "F1", "max_steps": 5}))
	if err != nil {
		t.Fatalf("handleGetFlow: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out ckvclient.Flow
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.FlowID != "F1" || len(out.Steps) != 1 || out.Steps[0].Symbol != "A" {
		t.Errorf("unexpected flow: %+v", out)
	}
	if len(f.ckv.Calls.GetFlow) != 1 || f.ckv.Calls.GetFlow[0].MaxSteps != 5 {
		t.Errorf("GetFlow call not recorded with max_steps: %+v", f.ckv.Calls.GetFlow)
	}
}

func TestHandleGetFlow_NoSelector_IsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	res, err := handleGetFlow(context.Background(), f.deps, callToolReq(nil))
	if err != nil {
		t.Fatalf("handleGetFlow: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError when no selector; got %+v", res)
	}
}

func TestHandleExpandFlow_BadDirection_IsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	res, err := handleExpandFlow(context.Background(), f.deps, callToolReq(map[string]any{"step_id": "s1", "direction": "sideways"}))
	if err != nil {
		t.Fatalf("handleExpandFlow: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for bad direction")
	}
}

func TestHandleExpandFlow_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.ExpandVal = ckvclient.FlowExpansion{
			Origin:    "s1",
			Neighbors: []ckvclient.FlowNeighbor{{StepID: "s2", Symbol: "B", Relation: "calls", Citation: cit("b.go", 3, 9)}},
		}
	})
	res, err := handleExpandFlow(context.Background(), f.deps, callToolReq(map[string]any{"step_id": "s1", "direction": "down", "hops": 2}))
	if err != nil {
		t.Fatalf("handleExpandFlow: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out ckvclient.FlowExpansion
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Origin != "s1" || len(out.Neighbors) != 1 {
		t.Errorf("unexpected expansion: %+v", out)
	}
	if got := f.ckv.Calls.ExpandFlow; len(got) != 1 || got[0].Hops != 2 || got[0].Direction != "down" {
		t.Errorf("ExpandFlow call not recorded: %+v", got)
	}
}

func TestHandleFindBranches_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.BranchMatches = []ckvclient.BranchMatch{{When: "seq>head", Then: "drop", At: "future.go:12", Score: 0.8}}
	})
	res, err := handleFindBranches(context.Background(), f.deps, callToolReq(map[string]any{"symptom_text": "valid message dropped", "k": 5}))
	if err != nil {
		t.Fatalf("handleFindBranches: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out findBranchesResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Matches) != 1 || out.Matches[0].When != "seq>head" {
		t.Errorf("unexpected matches: %+v", out.Matches)
	}
	if got := f.ckv.Calls.FindBranches; len(got) != 1 || got[0].K != 5 {
		t.Errorf("FindBranches call not recorded: %+v", got)
	}
}

func TestHandleFindBranches_MissingSymptom_IsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	res, err := handleFindBranches(context.Background(), f.deps, callToolReq(nil))
	if err != nil {
		t.Fatalf("handleFindBranches: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for missing symptom_text")
	}
}

func TestHandleGetInvariantEnforcement_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.EnforcementVal = ckvclient.InvariantEnforcement{
			InvID:      "INV1",
			Statement:  "no drop of valid next-seq",
			EnforcedAt: []ckvclient.EnforcementSite{{Flow: "F1", Step: "s1"}},
		}
	})
	res, err := handleGetInvariantEnforcement(context.Background(), f.deps, callToolReq(map[string]any{"inv_id": "INV1"}))
	if err != nil {
		t.Fatalf("handleGetInvariantEnforcement: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out ckvclient.InvariantEnforcement
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.InvID != "INV1" || len(out.EnforcedAt) != 1 {
		t.Errorf("unexpected enforcement: %+v", out)
	}
}

// ErrFlowUnsupported from the backend maps to a clear tool error (the
// scaffolding state: tools registered, CKV pkg/ckv flow methods not yet shipped).
func TestHandleGetFlow_FlowUnsupported_MapsToToolError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.GetFlowErr = ckvclient.ErrFlowUnsupported
	})
	res, err := handleGetFlow(context.Background(), f.deps, callToolReq(map[string]any{"flow_id": "F1"}))
	if err != nil {
		t.Fatalf("handleGetFlow: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for ErrFlowUnsupported")
	}
}

// A backend that implements Client but NOT FlowClient yields the
// "flow surface not available" signal via the type assertion in flowClient.
func TestHandleGetFlow_BackendWithoutFlowClient(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	d := f.deps
	d.CKV = nonFlowCKV{}
	res, err := handleGetFlow(context.Background(), d, callToolReq(map[string]any{"flow_id": "F1"}))
	if err != nil {
		t.Fatalf("handleGetFlow: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError when backend does not implement FlowClient")
	}
}

// nonFlowCKV implements ckvclient.Client but deliberately not FlowClient.
type nonFlowCKV struct{}

func (nonFlowCKV) SemanticSearch(context.Context, string, ckvclient.SearchOpts) ([]contract.Hit, error) {
	return nil, nil
}
func (nonFlowCKV) Health(context.Context) (ckvclient.Health, error) { return ckvclient.Health{}, nil }
func (nonFlowCKV) Freshness(context.Context) (ckvclient.FreshnessReport, error) {
	return ckvclient.FreshnessReport{}, nil
}
func (nonFlowCKV) Close() error { return nil }
