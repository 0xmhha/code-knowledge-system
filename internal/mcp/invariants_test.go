package mcp

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
)

func TestHandleFindInvariants_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.InvariantHits = []ckvclient.InvariantHit{
			{ChunkID: "c1", File: "core/state.go", Tier: 1, Text: "no drop of valid next-seq", Category: "consensus"},
		}
	})
	res, err := handleFindInvariants(context.Background(), f.deps,
		callToolReq(map[string]any{"file": "core/state.go", "tier_min": 1}))
	if err != nil {
		t.Fatalf("handleFindInvariants: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out []ckvclient.InvariantHit
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].ChunkID != "c1" {
		t.Errorf("unexpected invariants: %+v", out)
	}
}

func TestHandleGetConventions_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.Conventions = []ckvclient.ConventionHit{
			{ChunkID: "c2", Package: "core/vm", Summary: "early-return idiom"},
		}
	})
	res, err := handleGetConventions(context.Background(), f.deps,
		callToolReq(map[string]any{"package_prefix": "core/"}))
	if err != nil {
		t.Fatalf("handleGetConventions: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out []ckvclient.ConventionHit
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].Package != "core/vm" {
		t.Errorf("unexpected conventions: %+v", out)
	}
}
