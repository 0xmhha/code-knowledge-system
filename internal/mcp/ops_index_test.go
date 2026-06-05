package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestHandleOpsIndex_DomainExportFailureAborts(t *testing.T) {
	// A non-existent DomainProjectDir causes inventory.LoadProject to fail.
	// The handler must abort (not continue to ckv/ckg) and surface the
	// failure in resp.CKV.Error, containing "domain export".
	withStubRunner(t, "") // no real subprocess should fire
	d := Deps{Index: IndexConfig{
		CKVBinary:        "echo",
		CKGBinary:        "",
		DomainProjectDir: "/nonexistent/cks-domain-project",
		DomainCorpusDir:  t.TempDir(),
	}}
	res, err := handleOpsIndex(context.Background(), d, callToolReq(map[string]any{"mode": "full"}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	var resp opsIndexResponse
	if decErr := decodeStructured(res, &resp); decErr != nil {
		t.Fatalf("decode structured response: %v", decErr)
	}
	if !strings.Contains(resp.CKV.Error, "domain export") {
		t.Errorf("CKV.Error = %q; want substring \"domain export\"", resp.CKV.Error)
	}
}

func TestCKVIndexArgs_FullIncludesDocs(t *testing.T) {
	ic := IndexConfig{
		CKVDataPath:     "./ckv-stablenet",
		SourceRoot:      "/src",
		EmbedModel:      "bge-m3",
		DomainCorpusDir: "generated/domain-corpus/go-stablenet",
	}
	args := ckvIndexArgs(ic, "full", "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--docs generated/domain-corpus/go-stablenet") {
		t.Errorf("full build args missing --docs: %v", args)
	}
}

func TestCKVIndexArgs_IncrementalOmitsDocs(t *testing.T) {
	ic := IndexConfig{CKVDataPath: "./ckv-stablenet", DomainCorpusDir: "generated/corpus"}
	args := ckvIndexArgs(ic, "incremental", "")
	if strings.Contains(strings.Join(args, " "), "--docs") {
		t.Errorf("incremental (reindex) must not pass --docs: %v", args)
	}
}

type capturedRun struct {
	name string
	args []string
	env  []string
}

// withStubRunner swaps indexRunner for the duration of a test, capturing each
// invocation and returning failFor's error for the named binary.
func withStubRunner(t *testing.T, failFor string) *[]capturedRun {
	t.Helper()
	var calls []capturedRun
	orig := indexRunner
	indexRunner = func(_ context.Context, name string, args, env []string) error {
		calls = append(calls, capturedRun{name: name, args: args, env: env})
		if failFor != "" && name == failFor {
			return errors.New("boom")
		}
		return nil
	}
	t.Cleanup(func() { indexRunner = orig })
	return &calls
}

func TestHandleOpsIndex_UnconfiguredErrors(t *testing.T) {
	res, err := handleOpsIndex(context.Background(), Deps{}, callToolReq(map[string]any{"mode": "incremental"}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError result when no index binaries configured, got %+v", res)
	}
}

func TestHandleOpsIndex_BadModeErrors(t *testing.T) {
	res, _ := handleOpsIndex(context.Background(), Deps{Index: IndexConfig{CKVBinary: "ckv"}},
		callToolReq(map[string]any{"mode": "sideways"}))
	if res == nil || !res.IsError {
		t.Fatal("expected IsError for invalid mode")
	}
}

func TestHandleOpsIndex_IncrementalRunsBothBackends(t *testing.T) {
	calls := withStubRunner(t, "")
	d := Deps{Index: IndexConfig{
		CKVBinary: "ckv-bin", CKGBinary: "ckg-bin",
		CKVDataPath: "/d/ckv", CKGDataPath: "/d/ckg",
		SourceRoot: "/src", EmbedModel: "bge-m3", OllamaURL: "http://h:1",
	}}
	res, err := handleOpsIndex(context.Background(), d,
		callToolReq(map[string]any{"mode": "incremental", "since_commit": "abc123"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Errorf("unexpected IsError on success: %+v", res)
	}
	if len(*calls) != 2 {
		t.Fatalf("want 2 backend runs, got %d: %+v", len(*calls), *calls)
	}
	ckv, ckg := (*calls)[0], (*calls)[1]
	if ckv.name != "ckv-bin" {
		t.Errorf("first run = %q, want ckv-bin", ckv.name)
	}
	joined := strings.Join(ckv.args, " ")
	if !strings.Contains(joined, "reindex") || !strings.Contains(joined, "--out /d/ckv") ||
		!strings.Contains(joined, "--since abc123") || !strings.Contains(joined, "--model-name=bge-m3") {
		t.Errorf("ckv reindex args wrong: %v", ckv.args)
	}
	if len(ckv.env) == 0 || !strings.Contains(ckv.env[0], "CKV_OLLAMA_ENDPOINT=http://h:1") {
		t.Errorf("ckv env missing ollama endpoint: %v", ckv.env)
	}
	if ckg.name != "ckg-bin" || !strings.Contains(strings.Join(ckg.args, " "), "build --src /src --out /d/ckg") {
		t.Errorf("ckg build args wrong: %v", ckg.args)
	}
}

func TestHandleOpsIndex_CKGPolicyFileForwarded(t *testing.T) {
	calls := withStubRunner(t, "")
	d := Deps{Index: IndexConfig{
		CKGBinary: "ckg-bin", CKGDataPath: "/d/ckg", SourceRoot: "/src",
		CKGPolicyFile: "/p/policy.yaml",
	}}
	if _, err := handleOpsIndex(context.Background(), d, callToolReq(map[string]any{"mode": "full"})); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join((*calls)[0].args, " ")
	if !strings.Contains(joined, "build --src /src --out /d/ckg") ||
		!strings.Contains(joined, "--policy-file /p/policy.yaml") {
		t.Errorf("ckg build should forward --policy-file: %v", (*calls)[0].args)
	}
}

func TestHandleOpsIndex_CKGNoPolicyFileOmitsFlag(t *testing.T) {
	calls := withStubRunner(t, "")
	d := Deps{Index: IndexConfig{CKGBinary: "ckg-bin", CKGDataPath: "/d/ckg", SourceRoot: "/src"}}
	if _, err := handleOpsIndex(context.Background(), d, callToolReq(map[string]any{"mode": "full"})); err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join((*calls)[0].args, " "); strings.Contains(joined, "--policy-file") {
		t.Errorf("ckg build must omit --policy-file when unset: %v", (*calls)[0].args)
	}
}

func TestHandleOpsIndex_FullUsesBuildWithSrc(t *testing.T) {
	calls := withStubRunner(t, "")
	d := Deps{Index: IndexConfig{CKVBinary: "ckv-bin", CKVDataPath: "/d/ckv", SourceRoot: "/src", EmbedModel: "bge-m3"}}
	if _, err := handleOpsIndex(context.Background(), d, callToolReq(map[string]any{"mode": "full"})); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join((*calls)[0].args, " ")
	if !strings.Contains(joined, "build --src /src --out /d/ckv") {
		t.Errorf("full mode should `build --src`: %v", (*calls)[0].args)
	}
}

func TestHandleOpsIndex_BackendFailureSurfacesNotError(t *testing.T) {
	// A backend exec failure is reported in the structured per-backend OK
	// field (not a transport error); the handler still returns a result.
	withStubRunner(t, "ckv-bin")
	d := Deps{Index: IndexConfig{CKVBinary: "ckv-bin", CKVDataPath: "/d/ckv", SourceRoot: "/src"}}
	res, err := handleOpsIndex(context.Background(), d, callToolReq(map[string]any{"mode": "incremental"}))
	if err != nil {
		t.Fatalf("should not return transport error on backend failure: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}
