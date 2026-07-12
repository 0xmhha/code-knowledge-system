package mcp

import (
	"context"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func manifestJSON(srcCommit, srcRoot, ledgerCommit, ledgerDigest string) []byte {
	j := `{"src_commit":"` + srcCommit + `","src_root":"` + srcRoot + `"`
	if ledgerCommit != "" || ledgerDigest != "" {
		j += `,"sources":{"ckg":{"src_commit":"` + ledgerCommit + `","graph_digest":"` + ledgerDigest + `"}}`
	}
	j += `}`
	return []byte(j)
}

func TestComputeAlignment_OK(t *testing.T) {
	t.Parallel()
	rep := ComputeAlignment(AlignmentInputs{
		CKGSrcCommit:     "abc123",
		CKGSchema:        "1.23",
		CKVManifest:      manifestJSON("abc123", "/src", "", ""),
		ConfigSourceRoot: "/src",
		SourceHead:       "abc123",
	})
	if !rep.OK {
		t.Fatalf("want OK, got reason=%q", rep.Reason)
	}
	if !rep.SourceRootOK {
		t.Errorf("SourceRootOK = false, want true")
	}
	// Pre-P1 index → ledger-absent warning is expected, nothing else.
	for _, w := range rep.Warnings {
		if !strings.Contains(w, "ledger absent") {
			t.Errorf("unexpected warning: %s", w)
		}
	}
}

func TestComputeAlignment_CommitMismatch_IsError(t *testing.T) {
	t.Parallel()
	rep := ComputeAlignment(AlignmentInputs{
		CKGSrcCommit: "abc123",
		CKGSchema:    "1.23",
		CKVManifest:  manifestJSON("def456", "/src", "", ""),
	})
	if rep.OK {
		t.Fatal("commit mismatch must be error-tier (OK=false)")
	}
	if !strings.Contains(rep.Reason, "different commits") {
		t.Errorf("reason = %q", rep.Reason)
	}
}

func TestComputeAlignment_OldSchema_IsError(t *testing.T) {
	t.Parallel()
	rep := ComputeAlignment(AlignmentInputs{
		CKGSrcCommit: "abc123",
		CKGSchema:    "1.16",
		CKVManifest:  manifestJSON("abc123", "/src", "", ""),
	})
	if rep.OK {
		t.Fatal("schema < 1.19 must be error-tier")
	}
	if !strings.Contains(rep.Reason, "1.19") {
		t.Errorf("reason = %q", rep.Reason)
	}
}

func TestComputeAlignment_DigestMismatch_IsError(t *testing.T) {
	t.Parallel()
	rep := ComputeAlignment(AlignmentInputs{
		CKGSrcCommit: "abc123",
		CKGSchema:    "1.23",
		CKGDigest:    "digest-NEW",
		CKVManifest:  manifestJSON("", "/src", "abc123", "digest-OLD"),
	})
	if rep.OK {
		t.Fatal("digest mismatch must be error-tier")
	}
	if !strings.Contains(rep.Reason, "digest mismatch") {
		t.Errorf("reason = %q", rep.Reason)
	}
}

func TestComputeAlignment_SourceRootDiffers_IsWarningOnly(t *testing.T) {
	t.Parallel()
	rep := ComputeAlignment(AlignmentInputs{
		CKGSrcCommit:     "abc123",
		CKGSchema:        "1.23",
		CKVManifest:      manifestJSON("abc123", "/indexed/checkout", "", ""),
		ConfigSourceRoot: "/other/checkout",
		SourceHead:       "abc123", // same commit → warning tier per agreement
	})
	if !rep.OK {
		t.Fatalf("same-commit path difference must stay warning-tier, got reason=%q", rep.Reason)
	}
	if rep.SourceRootOK {
		t.Error("SourceRootOK should be false")
	}
	joined := strings.Join(rep.Warnings, " | ")
	if !strings.Contains(joined, "source_root") {
		t.Errorf("expected source_root warning, got %v", rep.Warnings)
	}
}

func TestComputeAlignment_StaleSourceHead_IsWarningOnly(t *testing.T) {
	t.Parallel()
	rep := ComputeAlignment(AlignmentInputs{
		CKGSrcCommit:     "abc123",
		CKGSchema:        "1.23",
		CKVManifest:      manifestJSON("abc123", "/src", "", ""),
		ConfigSourceRoot: "/src",
		SourceHead:       "fff999", // tree moved on — freshness territory
	})
	if !rep.OK {
		t.Fatalf("stale source head must stay warning-tier, got reason=%q", rep.Reason)
	}
	joined := strings.Join(rep.Warnings, " | ")
	if !strings.Contains(joined, "stale tree") {
		t.Errorf("expected stale-tree warning, got %v", rep.Warnings)
	}
}

func TestComputeAlignment_LedgerPreferredOverTopLevel(t *testing.T) {
	t.Parallel()
	// sources.ckg.src_commit wins over the top-level src_commit.
	rep := ComputeAlignment(AlignmentInputs{
		CKGSrcCommit: "ledger-commit",
		CKGSchema:    "1.23",
		CKVManifest:  []byte(`{"src_commit":"stale-top","sources":{"ckg":{"src_commit":"ledger-commit"}}}`),
	})
	if !rep.OK {
		t.Fatalf("ledger commit matches — want OK, got %q", rep.Reason)
	}
	if rep.SrcCommitCKV != "ledger-commit" {
		t.Errorf("SrcCommitCKV = %q, want ledger-commit", rep.SrcCommitCKV)
	}
}

// Health must fold an alignment failure into serviceable=false while both
// backends stay individually reachable.
func TestHandleHealth_AlignmentFailure_NotServiceable(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.deps.Alignment = &AlignmentReport{OK: false, Reason: "ckg/ckv built from different commits"}

	res, err := handleHealth(context.Background(), f.deps, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleHealth: %v", err)
	}
	var out struct {
		Status      string `json:"status"`
		Serviceable bool   `json:"serviceable"`
		Alignment   *AlignmentReport
	}
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Status != "ok" {
		t.Errorf("backend status should stay %q-independent of alignment; got %q", "ok", out.Status)
	}
	if out.Serviceable {
		t.Error("serviceable must be false on alignment failure")
	}
	if out.Alignment == nil || out.Alignment.OK {
		t.Errorf("alignment block missing or OK: %+v", out.Alignment)
	}
	// And the query-path gate agrees.
	ok, reason := serviceable(context.Background(), f.deps)
	if ok || !strings.Contains(reason, "alignment") {
		t.Errorf("serviceable() = %v %q, want false with alignment reason", ok, reason)
	}
}
