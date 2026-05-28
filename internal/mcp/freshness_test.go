package mcp

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
)

func TestHandleFreshness_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.FreshnessVal = ckvclient.FreshnessReport{
			Fresh:        false,
			IndexedHead:  "abc123",
			CurrentHead:  "def456",
			ChangedFiles: []string{"consensus/wbft/finalize.go", "core/genesis.go"},
		}
	})
	res, err := handleFreshness(context.Background(), f.deps, callToolReq(nil))
	if err != nil {
		t.Fatalf("handleFreshness: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out freshnessResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Fresh {
		t.Errorf("Fresh = true, want false")
	}
	if out.IndexedHead != "abc123" {
		t.Errorf("IndexedHead = %q", out.IndexedHead)
	}
	if out.CurrentHead != "def456" {
		t.Errorf("CurrentHead = %q", out.CurrentHead)
	}
	if len(out.ChangedFiles) != 2 {
		t.Errorf("ChangedFiles = %d, want 2", len(out.ChangedFiles))
	}
	if f.ckv.Calls.Freshness != 1 {
		t.Errorf("Freshness calls = %d, want 1", f.ckv.Calls.Freshness)
	}
}

func TestHandleFreshness_DummyEmitsInstruction(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.deps.CKV = ckvclient.NewDummy()

	res, err := handleFreshness(context.Background(), f.deps, callToolReq(nil))
	if err != nil {
		t.Fatalf("handleFreshness: %v", err)
	}
	var out freshnessResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Instructions) != 1 {
		t.Fatalf("Instructions = %d, want 1", len(out.Instructions))
	}
	if out.Instructions[0].Operation != "Freshness" || out.Instructions[0].Backend != "ckv" {
		t.Errorf("instruction = %+v", out.Instructions[0])
	}
}
