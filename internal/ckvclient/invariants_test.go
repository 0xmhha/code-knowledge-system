package ckvclient

import (
	"context"
	"errors"
	"testing"
)

func TestFake_FindInvariants_CannedAndRecord(t *testing.T) {
	t.Parallel()
	f := &Fake{InvariantHits: []InvariantHit{
		{ChunkID: "c1", File: "core/state.go", Tier: 1, Text: "no drop of valid next-seq", Category: "consensus"},
	}}
	got, err := f.FindInvariants(context.Background(), "core/state.go", "consensus", 1)
	if err != nil {
		t.Fatalf("FindInvariants: %v", err)
	}
	if len(got) != 1 || got[0].ChunkID != "c1" {
		t.Fatalf("unexpected hits: %+v", got)
	}
	if len(f.Calls.FindInvariants) != 1 ||
		f.Calls.FindInvariants[0].File != "core/state.go" ||
		f.Calls.FindInvariants[0].Category != "consensus" ||
		f.Calls.FindInvariants[0].TierMin != 1 {
		t.Errorf("call not recorded correctly: %+v", f.Calls.FindInvariants)
	}
}

func TestFake_GetConventions_CannedAndRecord(t *testing.T) {
	t.Parallel()
	f := &Fake{Conventions: []ConventionHit{
		{ChunkID: "c2", Package: "core/vm", Summary: "early-return idiom"},
	}}
	got, err := f.GetConventions(context.Background(), "core/")
	if err != nil {
		t.Fatalf("GetConventions: %v", err)
	}
	if len(got) != 1 || got[0].Package != "core/vm" {
		t.Fatalf("unexpected hits: %+v", got)
	}
	if len(f.Calls.GetConventions) != 1 || f.Calls.GetConventions[0].PackagePrefix != "core/" {
		t.Errorf("call not recorded correctly: %+v", f.Calls.GetConventions)
	}
}

func TestFake_FindInvariants_ErrPrecedence(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	f := &Fake{FindInvariantsErr: sentinel, InvariantHits: []InvariantHit{{ChunkID: "c1"}}}
	if _, err := f.FindInvariants(context.Background(), "", "", 0); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel", err)
	}
}

// The Smart Dummy degrades the knowledge lookups to empty (not
// ErrFlowUnsupported) so the coding-agent diagnose path proceeds without a
// hard error when no ckv backend is configured.
func TestDummy_FindInvariants_GetConventions_EmptyNoError(t *testing.T) {
	t.Parallel()
	var fc FlowClient = &Dummy{}
	inv, err := fc.FindInvariants(context.Background(), "", "", 0)
	if err != nil {
		t.Errorf("FindInvariants err = %v, want nil", err)
	}
	if len(inv) != 0 {
		t.Errorf("FindInvariants = %+v, want empty", inv)
	}
	conv, err := fc.GetConventions(context.Background(), "")
	if err != nil {
		t.Errorf("GetConventions err = %v, want nil", err)
	}
	if len(conv) != 0 {
		t.Errorf("GetConventions = %+v, want empty", conv)
	}
}
