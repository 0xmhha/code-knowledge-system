package ckgclient

// B7 canonical_id join fixture (unit half). The cross-repo agreement
// (coordination 2026-06-29, D-2/D-3) makes ckg's canonical_id THE join key
// between ckv chunks and ckg nodes, and cks resolves it at the client surface.
// These cases pin the two agreed caveats at that surface:
//
//  1. a full canonical id (ADR-0001, e.g. "<importpath>.(*Recv).Method") must
//     resolve through the public FindSymbol path — the same path the
//     find_symbol / find_callers / find_callees tools use — not only through
//     the internal resolveQname helpers;
//  2. same-file duplicates carry a positional "@<line>" suffix (ckg
//     refinement B3) and must resolve as distinct nodes.
//
// The ≥1.19 schema-population gate is a live-dataset property and is asserted
// by the env-gated live half (b7_join_live_test.go).

import (
	"context"
	"testing"

	"github.com/0xmhha/code-knowledge-graph/pkg/types"
)

func canonNode(canonID, qname, file string, start, end int) types.Node {
	return types.Node{
		ID:            "sha-" + qname,
		Type:          types.NodeFunction,
		Name:          qname,
		QualifiedName: qname,
		FilePath:      file,
		StartLine:     start,
		EndLine:       end,
	}
}

func TestFindSymbol_ResolvesCanonicalID(t *testing.T) {
	t.Parallel()
	const canonID = "github.com/org/stablenet/consensus/wbft.(*Backend).Finalize"
	m := &mockStoreReader{
		canonicalByID: map[string]types.Node{
			canonID: canonNode(canonID, "wbft.(*Backend).Finalize", "consensus/wbft/backend.go", 100, 140),
		},
		// FindSymbol-by-name knows nothing about the canonical id: resolution
		// must come from the FindByCanonicalID path, not qname suffix matching.
		symbolByName: map[string][]types.Node{},
	}
	r := newRealWithStore(m)

	cits, err := r.FindSymbol(context.Background(), canonID, SymbolOpts{})
	if err != nil {
		t.Fatalf("FindSymbol(canonical id): %v", err)
	}
	if len(cits) != 1 {
		t.Fatalf("FindSymbol(canonical id) = %d citations, want 1 (canonical-first resolution)", len(cits))
	}
	if cits[0].File != "consensus/wbft/backend.go" || cits[0].StartLine != 100 {
		t.Errorf("unexpected citation: %+v", cits[0])
	}
}

func TestFindSymbol_ResolvesLineQualifiedDuplicate(t *testing.T) {
	t.Parallel()
	// Same-file duplicate canonical ids carry an "@<line>" suffix (B3); each
	// must resolve to its own node.
	const base = "contracts/gov.sol:Vote(uint256)"
	m := &mockStoreReader{
		canonicalByID: map[string]types.Node{
			base + "@42": canonNode(base+"@42", "Vote", "contracts/gov.sol", 42, 60),
			base + "@88": canonNode(base+"@88", "Vote", "contracts/gov.sol", 88, 105),
		},
		symbolByName: map[string][]types.Node{},
	}
	r := newRealWithStore(m)

	for _, tc := range []struct {
		id        string
		wantStart int
	}{
		{base + "@42", 42},
		{base + "@88", 88},
	} {
		cits, err := r.FindSymbol(context.Background(), tc.id, SymbolOpts{})
		if err != nil {
			t.Fatalf("FindSymbol(%q): %v", tc.id, err)
		}
		if len(cits) != 1 || cits[0].StartLine != tc.wantStart {
			t.Errorf("FindSymbol(%q) = %+v, want single citation at line %d", tc.id, cits, tc.wantStart)
		}
	}
}

// A canonical-id miss must fall back to the existing qname suffix matching —
// canonical-first must not break plain-name lookups.
func TestFindSymbol_CanonicalMissFallsBackToQname(t *testing.T) {
	t.Parallel()
	n := canonNode("", "QuorumSize", "consensus/wbft/validator.go", 10, 20)
	m := &mockStoreReader{
		canonicalByID: map[string]types.Node{}, // no canonical entries
		symbolByName:  map[string][]types.Node{"QuorumSize": {n}},
	}
	r := newRealWithStore(m)

	cits, err := r.FindSymbol(context.Background(), "QuorumSize", SymbolOpts{})
	if err != nil {
		t.Fatalf("FindSymbol: %v", err)
	}
	if len(cits) != 1 || cits[0].File != "consensus/wbft/validator.go" {
		t.Errorf("qname fallback broken: %+v", cits)
	}
}
