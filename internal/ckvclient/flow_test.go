package ckvclient

import (
	"context"
	"errors"
	"testing"
)

func TestFake_GetFlow_CannedAndMaxStepsCap(t *testing.T) {
	t.Parallel()
	f := &Fake{FlowVal: Flow{
		FlowID:     "F1",
		EntryPoint: "Handle",
		Steps: []FlowStep{
			{StepID: "s1", Symbol: "A"},
			{StepID: "s2", Symbol: "B"},
			{StepID: "s3", Symbol: "C"},
		},
	}}
	got, err := f.GetFlow(context.Background(), FlowQuery{FlowID: "F1", MaxSteps: 2})
	if err != nil {
		t.Fatalf("GetFlow: %v", err)
	}
	if got.FlowID != "F1" {
		t.Errorf("FlowID = %q, want F1", got.FlowID)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("MaxSteps cap not applied: got %d steps, want 2", len(got.Steps))
	}
	if got.Steps[0].Symbol != "A" {
		t.Errorf("Symbol not carried: %q", got.Steps[0].Symbol)
	}
	if len(f.Calls.GetFlow) != 1 || f.Calls.GetFlow[0].MaxSteps != 2 {
		t.Errorf("call not recorded: %+v", f.Calls.GetFlow)
	}
}

func TestFake_ExpandFlow_LimitCap(t *testing.T) {
	t.Parallel()
	f := &Fake{ExpandVal: FlowExpansion{
		Origin: "s1",
		Neighbors: []FlowNeighbor{
			{StepID: "s2", Relation: "calls"},
			{StepID: "s3", Relation: "calls"},
		},
	}}
	got, err := f.ExpandFlow(context.Background(), ExpandFlowQuery{StepID: "s1", Direction: "down", Limit: 1})
	if err != nil {
		t.Fatalf("ExpandFlow: %v", err)
	}
	if len(got.Neighbors) != 1 {
		t.Fatalf("Limit cap not applied: got %d, want 1", len(got.Neighbors))
	}
}

func TestFake_FindBranches_KCapAndRecord(t *testing.T) {
	t.Parallel()
	f := &Fake{BranchMatches: []BranchMatch{
		{StepID: "s1", Score: 0.9},
		{StepID: "s2", Score: 0.5},
	}}
	got, err := f.FindBranches(context.Background(), "drops valid message", 1)
	if err != nil {
		t.Fatalf("FindBranches: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("k cap not applied: got %d, want 1", len(got))
	}
	if len(f.Calls.FindBranches) != 1 || f.Calls.FindBranches[0].Symptom != "drops valid message" {
		t.Errorf("call not recorded: %+v", f.Calls.FindBranches)
	}
}

func TestFake_GetInvariantEnforcement_MaxCap(t *testing.T) {
	t.Parallel()
	f := &Fake{EnforcementVal: InvariantEnforcement{
		InvID: "INV1",
		EnforcedAt: []EnforcementSite{
			{Flow: "F1", Step: "s1", Loc: "wbft.go:10"},
			{Flow: "F1", Step: "s2"},
		},
	}}
	got, err := f.GetInvariantEnforcement(context.Background(), "INV1", 1)
	if err != nil {
		t.Fatalf("GetInvariantEnforcement: %v", err)
	}
	if len(got.EnforcedAt) != 1 {
		t.Fatalf("max cap not applied: got %d, want 1", len(got.EnforcedAt))
	}
}

func TestFake_FlowErrPrecedence(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	f := &Fake{GetFlowErr: sentinel, FlowVal: Flow{FlowID: "F1"}}
	if _, err := f.GetFlow(context.Background(), FlowQuery{FlowID: "F1"}); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

// The Smart Dummy has no real index, so every flow call returns
// ErrFlowUnsupported. (Real is exercised against a live engine in integration,
// not here: its methods delegate to pkg/ckv.Engine which needs an open index.)
func TestDummy_ReturnsErrFlowUnsupported(t *testing.T) {
	t.Parallel()
	var fc FlowClient = &Dummy{}
	if _, err := fc.GetFlow(context.Background(), FlowQuery{FlowID: "x"}); !errors.Is(err, ErrFlowUnsupported) {
		t.Errorf("GetFlow err = %v, want ErrFlowUnsupported", err)
	}
	if _, err := fc.ExpandFlow(context.Background(), ExpandFlowQuery{StepID: "x"}); !errors.Is(err, ErrFlowUnsupported) {
		t.Errorf("ExpandFlow err = %v, want ErrFlowUnsupported", err)
	}
	if _, err := fc.FindBranches(context.Background(), "x", 1); !errors.Is(err, ErrFlowUnsupported) {
		t.Errorf("FindBranches err = %v, want ErrFlowUnsupported", err)
	}
	if _, err := fc.GetInvariantEnforcement(context.Background(), "x", 0); !errors.Is(err, ErrFlowUnsupported) {
		t.Errorf("GetInvariantEnforcement err = %v, want ErrFlowUnsupported", err)
	}
}
