package mcp

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- find_symbol ---

func TestHandleFindSymbol_MissingName_IsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	res, err := handleFindSymbol(context.Background(), f.deps, callToolReq(nil))
	if err != nil {
		t.Fatalf("handleFindSymbol: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError for missing name; got %+v", res)
	}
}

func TestHandleFindSymbol_HappyPath(t *testing.T) {
	t.Parallel()
	wantCit := cit("login.go", 10, 30)
	f := newFixture(t, func(f *fixture) {
		f.ckg.SymbolCitations = []contract.Citation{wantCit}
	})

	req := callToolReq(map[string]any{
		"name":  "Login",
		"kinds": "function,method",
	})
	res, err := handleFindSymbol(context.Background(), f.deps, req)
	if err != nil {
		t.Fatalf("handleFindSymbol: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}

	var out findSymbolResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Symbol != "Login" {
		t.Errorf("Symbol = %q, want Login", out.Symbol)
	}
	if len(out.Citations) != 1 || out.Citations[0].Key() != wantCit.Key() {
		t.Errorf("Citations = %+v", out.Citations)
	}
	// Verify CKG fake recorded the kinds filter.
	if len(f.ckg.Calls.FindSymbol) != 1 {
		t.Fatalf("expected 1 FindSymbol call, got %d", len(f.ckg.Calls.FindSymbol))
	}
	gotKinds := f.ckg.Calls.FindSymbol[0].Opts.Kinds
	if len(gotKinds) != 2 || gotKinds[0] != "function" || gotKinds[1] != "method" {
		t.Errorf("Kinds = %v", gotKinds)
	}
}

func TestHandleFindSymbol_DummyEmitsInstructions(t *testing.T) {
	t.Parallel()
	// Wire a dummy ckg directly into Deps so handleFindSymbol invokes it.
	f := newFixture(t, nil)
	dummy := ckgclient.NewDummy()
	f.deps.CKG = dummy

	req := callToolReq(map[string]any{"name": "Finalize"})
	res, err := handleFindSymbol(context.Background(), f.deps, req)
	if err != nil {
		t.Fatalf("handleFindSymbol: %v", err)
	}
	var out findSymbolResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Instructions) != 1 {
		t.Fatalf("Instructions: got %d, want 1", len(out.Instructions))
	}
	if out.Instructions[0].Operation != "FindSymbol" || out.Instructions[0].Backend != "ckg" {
		t.Errorf("instruction = %+v", out.Instructions[0])
	}
}

// --- find_callers / find_callees ---

func TestHandleFindCallers_HappyPath(t *testing.T) {
	t.Parallel()
	seed := cit("consensus/wbft/finalize.go", 100, 150)
	f := newFixture(t, func(f *fixture) {
		f.ckg.SymbolCitations = []contract.Citation{seed}
		f.ckg.NeighborEdges = []contract.Neighbor{{
			Source:   cit("eth/handler.go", 50, 60),
			Target:   seed,
			Relation: contract.RelationCalledBy,
			Distance: 1,
		}}
	})
	req := callToolReq(map[string]any{
		"symbol": "consensus.wbft.Finalize",
		"depth":  float64(2),
	})
	res, err := handleFindRelatives(context.Background(), f.deps, req, ToolNameFindCallers, "callers", []contract.Relation{contract.RelationCalledBy})
	if err != nil {
		t.Fatalf("handleFindRelatives: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out graphNeighborsResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Direction != "callers" {
		t.Errorf("Direction = %q", out.Direction)
	}
	if len(out.Neighbors) != 1 || out.Neighbors[0].Relation != contract.RelationCalledBy {
		t.Errorf("Neighbors = %+v", out.Neighbors)
	}
	// Verify the depth=2 propagated through to Neighbors.
	if len(f.ckg.Calls.Neighbors) != 1 || f.ckg.Calls.Neighbors[0].Opts.Hops != 2 {
		t.Errorf("Neighbors call Hops = %v", f.ckg.Calls.Neighbors)
	}
}

func TestHandleFindCallers_SymbolNotFound_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckg.SymbolCitations = nil // no resolution
	})
	req := callToolReq(map[string]any{"symbol": "missing.Symbol"})
	res, err := handleFindRelatives(context.Background(), f.deps, req, ToolNameFindCallers, "callers", []contract.Relation{contract.RelationCalledBy})
	if err != nil {
		t.Fatalf("handleFindRelatives: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out graphNeighborsResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Neighbors) != 0 {
		t.Errorf("Neighbors should be empty, got %+v", out.Neighbors)
	}
}

// --- get_subgraph ---

func TestHandleGetSubgraph_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckg.SubgraphCitations = []contract.Citation{cit("a.go", 1, 10), cit("b.go", 1, 10)}
		f.ckg.SubgraphNeighbors = []contract.Neighbor{{
			Source: cit("a.go", 1, 10), Target: cit("b.go", 1, 10),
			Relation: contract.RelationImports, Distance: 1,
		}}
	})

	req := callToolReq(map[string]any{
		"symbol": "pkg.Foo",
		"depth":  float64(2),
	})
	res, err := handleGetSubgraph(context.Background(), f.deps, req)
	if err != nil {
		t.Fatalf("handleGetSubgraph: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", resultText(res))
	}
	var out subgraphResponse
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Seed != "pkg.Foo" {
		t.Errorf("Seed = %q", out.Seed)
	}
	if len(out.Nodes) != 2 {
		t.Errorf("Nodes = %d, want 2", len(out.Nodes))
	}
	if len(out.Edges) != 1 || out.Edges[0].Relation != contract.RelationImports {
		t.Errorf("Edges = %+v", out.Edges)
	}
}

func TestHandleGetSubgraph_MissingSymbol_IsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	res, err := handleGetSubgraph(context.Background(), f.deps, callToolReq(nil))
	if err != nil {
		t.Fatalf("handleGetSubgraph: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError for missing symbol; got %+v", res)
	}
}
