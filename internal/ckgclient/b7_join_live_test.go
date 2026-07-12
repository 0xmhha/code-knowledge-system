package ckgclient_test

// B7 canonical_id join fixture (live half). Env-gated integration check that
// runs the REAL join chain cks depends on — ckv chunk → canonical_id →
// ckg node — against a live dataset, asserting the cross-repo agreements
// (coordination 2026-06-29, D-2/D-3):
//
//  1. schema gate: the graph must be built at cache SchemaVersion >= 1.19,
//     the first version whose canonical_id VALUES are populated (a 1.16–1.18
//     graph has the column but NULLs — joining on it fails silently);
//  2. chunk inheritance: real ckv semantic hits must carry a ckg canonical id
//     (CanonicalID, inherited per CKV PR #9);
//  3. join resolution: those ids must resolve through cks's public FindSymbol
//     surface at >= 90% — the match-rate target agreed for the B7 measurement.
//
// Skipped unless CKS_B7_LIVE_CKG points at a graph.db. Run against pr-77-2:
//
//	CKS_B7_LIVE_CKG=/Users/.../knowledge-data/pr-77-2/graph.db \
//	CKS_B7_LIVE_CKV=/Users/.../knowledge-data/pr-77-2/ckv \
//	  go test ./internal/ckgclient/ -run TestLive_B7 -v
//
// Optional: CKS_B7_LIVE_OLLAMA (default http://localhost:11434),
// CKS_B7_LIVE_MODEL (default bge-m3).

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/embedder"
)

func b7env(key, dflt string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return dflt
}

// schemaAtLeast reports whether a "major.minor" schema string is >= min.
func schemaAtLeast(version string, minMajor, minMinor int) bool {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > minMajor || (major == minMajor && minor >= minMinor)
}

func TestLive_B7_CanonicalJoin(t *testing.T) {
	graphPath := os.Getenv("CKS_B7_LIVE_CKG")
	if graphPath == "" {
		t.Skip("set CKS_B7_LIVE_CKG=<graph.db> (and CKS_B7_LIVE_CKV=<ckv dir>) to run the live B7 join check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// --- 1. schema gate (>= 1.19; anything lower joins on NULLs silently) ---
	ckg, err := ckgclient.NewReal(graphPath)
	if err != nil {
		t.Fatalf("ckgclient.NewReal(%q): %v", graphPath, err)
	}
	defer ckg.Close()
	h, err := ckg.Health(ctx)
	if err != nil {
		t.Fatalf("ckg Health: %v", err)
	}
	t.Logf("ckg: schema=%s indexed_head=%s", h.SchemaVersion, h.IndexedHead)
	if !schemaAtLeast(h.SchemaVersion, 1, 19) {
		t.Fatalf("schema gate: graph schema %q < 1.19 — canonical_id values are not populated; rebuild the graph", h.SchemaVersion)
	}

	// --- 2. chunk inheritance: real ckv hits must carry canonical ids -------
	ckvPath := os.Getenv("CKS_B7_LIVE_CKV")
	if ckvPath == "" {
		t.Skip("CKS_B7_LIVE_CKV unset — schema gate verified, skipping the chunk-join half")
	}
	emb, cap, err := embedder.Open("ollama", b7env("CKS_B7_LIVE_MODEL", "bge-m3"), b7env("CKS_B7_LIVE_OLLAMA", "http://localhost:11434"))
	if err != nil {
		t.Fatalf("embedder.Open: %v", err)
	}
	ckv, err := ckvclient.NewReal(ctx, ckvclient.RealOpts{DataPath: ckvPath, Embedder: emb})
	if err != nil {
		t.Fatalf("ckvclient.NewReal(%q): %v", ckvPath, err)
	}
	defer ckv.Close()
	t.Logf("ckv: %s/%s dim=%d", cap.Provider, cap.Model, cap.Dim)

	queries := []string{
		"quorum size validation for consensus",
		"genesis block initialization",
		"transaction pool admission checks",
	}
	ids := map[string]struct{}{}
	total := 0
	for _, q := range queries {
		hits, err := ckv.SemanticSearch(ctx, q, ckvclient.SearchOpts{K: 10})
		if err != nil {
			t.Fatalf("SemanticSearch(%q): %v", q, err)
		}
		for _, hit := range hits {
			total++
			if hit.CanonicalID != "" {
				ids[hit.CanonicalID] = struct{}{}
			}
		}
	}
	if len(ids) == 0 {
		t.Fatalf("no ckv hit carried a CanonicalID — chunk inheritance (CKV PR #9) not in effect on this index, or the cks translation dropped it")
	}
	t.Logf("collected %d unique canonical ids from %d hits", len(ids), total)

	// --- 3. join resolution through the public FindSymbol surface -----------
	resolved, lineQualified := 0, 0
	var missed []string
	for id := range ids {
		cits, err := ckg.FindSymbol(ctx, id, ckgclient.SymbolOpts{})
		if err != nil {
			t.Fatalf("FindSymbol(%q): %v", id, err)
		}
		if len(cits) > 0 {
			resolved++
			if strings.Contains(id, "@") {
				lineQualified++
			}
		} else if len(missed) < 5 {
			missed = append(missed, id)
		}
	}
	rate := float64(resolved) / float64(len(ids))
	t.Logf("join: %d/%d resolved (%.1f%%), %d line-qualified (@<line>) among resolved", resolved, len(ids), rate*100, lineQualified)
	if len(missed) > 0 {
		t.Logf("sample misses: %v", missed)
	}
	if rate < 0.9 {
		t.Errorf("canonical join rate %.1f%% < 90%% target (D-1/§1-R agreement)", rate*100)
	}
}
