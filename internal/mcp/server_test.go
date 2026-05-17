package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/composer"
	"github.com/0xmhha/code-knowledge-system/internal/composer/budget"
	"github.com/0xmhha/code-knowledge-system/internal/composer/intent"
	"github.com/0xmhha/code-knowledge-system/internal/composer/sanitize"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage1"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/internal/config"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// fixture wires a composer with shared ckg/ckv fakes so tests can both
// drive the pipeline (via the composer) and inspect/perturb the same
// backends the MCP health tool will report on.
type fixture struct {
	ckv      *ckvclient.Fake
	ckg      *ckgclient.Fake
	embedder *intent.FakeEmbedder
	fetcher  *budget.FakeFetcher
	ruleset  *config.SanitizeRuleset // overridable in setup
	deps     Deps                    // produced after setup
}

func newFixture(t *testing.T, setup func(f *fixture)) *fixture {
	t.Helper()
	f := &fixture{
		ckv:      &ckvclient.Fake{},
		ckg:      &ckgclient.Fake{},
		embedder: &intent.FakeEmbedder{Dim: 16},
		fetcher:  &budget.FakeFetcher{Bodies: map[string]string{}},
		ruleset: &config.SanitizeRuleset{
			Version: 1,
			Rules: []config.SanitizeRule{
				{ID: "NOOP", Pattern: `__no_match__`, Action: contract.RedactionDrop, Severity: config.SeverityLow},
			},
		},
	}
	if setup != nil {
		setup(f)
	}
	if err := f.ruleset.Validate(); err != nil {
		t.Fatalf("ruleset.Validate: %v", err)
	}

	ic, err := intent.New(context.Background(), f.embedder)
	if err != nil {
		t.Fatalf("intent.New: %v", err)
	}
	s1, err := stage1.New(f.ckv, f.ckg)
	if err != nil {
		t.Fatalf("stage1.New: %v", err)
	}
	s2, err := stage2.New(f.ckg)
	if err != nil {
		t.Fatalf("stage2.New: %v", err)
	}
	s3, err := stage3.New(f.ckg)
	if err != nil {
		t.Fatalf("stage3.New: %v", err)
	}
	b, err := budget.New(f.fetcher)
	if err != nil {
		t.Fatalf("budget.New: %v", err)
	}
	san, err := sanitize.New(f.ruleset)
	if err != nil {
		t.Fatalf("sanitize.New: %v", err)
	}
	c, err := composer.New(ic, s1, s2, s3, b, san)
	if err != nil {
		t.Fatalf("composer.New: %v", err)
	}

	f.deps = Deps{
		Composer:       c,
		CKG:            f.ckg,
		CKV:            f.ckv,
		BuilderVersion: "cks-test/0.0.1",
	}
	return f
}

func cit(file string, start, end int) contract.Citation {
	return contract.Citation{File: file, StartLine: start, EndLine: end, CommitHash: "abc"}
}
func hit(file string, start, end int, score float64, src contract.HitSource) contract.Hit {
	return contract.Hit{Citation: cit(file, start, end), Rank: 1, Score: score, Source: src}
}

// --- Register: tool registration + nil-dep rejection ---

func TestRegister_RegistersBothTools(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	srv := mcpserver.NewMCPServer("cks-test", "0.0.1")
	if err := Register(srv, f.deps); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Reachability smoke: the standalone handler should be callable
	// after registration is accepted.
	if _, err := handleHealth(context.Background(), f.deps, mcpgo.CallToolRequest{}); err != nil {
		t.Fatalf("handleHealth callable: %v", err)
	}
}

func TestRegister_NilDepRejected(t *testing.T) {
	t.Parallel()
	srv := mcpserver.NewMCPServer("cks-test", "0.0.1")
	good := newFixture(t, nil).deps

	cases := []struct {
		name string
		d    Deps
	}{
		{"nil composer", Deps{CKG: good.CKG, CKV: good.CKV}},
		{"nil ckg", Deps{Composer: good.Composer, CKV: good.CKV}},
		{"nil ckv", Deps{Composer: good.Composer, CKG: good.CKG}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := Register(srv, tc.d); err == nil {
				t.Fatalf("Register(%s) returned nil err", tc.name)
			}
		})
	}
}

// --- handleHealth ---

func TestHandleHealth_AllReachable_ReturnsOK(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckg.HealthVal = ckgclient.Health{Reachable: true, SchemaVersion: "v1", IndexedHead: "abc"}
		f.ckv.HealthVal = ckvclient.Health{Reachable: true, StatsHash: "h1"}
	})

	res, err := handleHealth(context.Background(), f.deps, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleHealth: %v", err)
	}
	got := unpackResult(t, res)
	if got["status"] != "ok" {
		t.Errorf("status = %v, want \"ok\"", got["status"])
	}
	if got["builder_version"] != "cks-test/0.0.1" {
		t.Errorf("builder_version = %v, want cks-test/0.0.1", got["builder_version"])
	}
	backends, ok := got["backends"].(map[string]any)
	if !ok {
		t.Fatalf("backends not an object: %T %v", got["backends"], got["backends"])
	}
	ckg, _ := backends["ckg"].(map[string]any)
	if ckg["reachable"] != true {
		t.Errorf("backends.ckg.reachable = %v, want true", ckg["reachable"])
	}
	if ckg["schema_version"] != "v1" {
		t.Errorf("backends.ckg.schema_version = %v, want v1", ckg["schema_version"])
	}
	ckv, _ := backends["ckv"].(map[string]any)
	if ckv["reachable"] != true {
		t.Errorf("backends.ckv.reachable = %v, want true", ckv["reachable"])
	}
	if ckv["stats_hash"] != "h1" {
		t.Errorf("backends.ckv.stats_hash = %v, want h1", ckv["stats_hash"])
	}
}

func TestHandleHealth_CKGDown_ReturnsDown(t *testing.T) {
	t.Parallel()
	// HLD §10: ckg unreachable -> fail closed. Health should reflect
	// that the pack-producing path is no longer usable.
	f := newFixture(t, func(f *fixture) {
		f.ckg.HealthErr = errors.New("ckg backend unreachable")
		f.ckv.HealthVal = ckvclient.Health{Reachable: true}
	})

	res, err := handleHealth(context.Background(), f.deps, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleHealth: %v", err)
	}
	got := unpackResult(t, res)
	if got["status"] != "down" {
		t.Errorf("status = %v, want \"down\" (ckg is required)", got["status"])
	}
	backends, _ := got["backends"].(map[string]any)
	ckg, _ := backends["ckg"].(map[string]any)
	if ckg["reachable"] != false {
		t.Errorf("backends.ckg.reachable = %v, want false", ckg["reachable"])
	}
	if !strings.Contains(asString(ckg["error"]), "unreachable") {
		t.Errorf("backends.ckg.error = %v, want substring \"unreachable\"", ckg["error"])
	}
}

func TestHandleHealth_CKVDown_ReturnsDegraded(t *testing.T) {
	t.Parallel()
	// HLD §10: ckv unreachable -> fall back to ckg-only. The pack path
	// still works, so the system is degraded, not down.
	f := newFixture(t, func(f *fixture) {
		f.ckg.HealthVal = ckgclient.Health{Reachable: true}
		f.ckv.HealthErr = errors.New("ckv backend unreachable")
	})

	res, err := handleHealth(context.Background(), f.deps, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleHealth: %v", err)
	}
	got := unpackResult(t, res)
	if got["status"] != "degraded" {
		t.Errorf("status = %v, want \"degraded\" (ckv optional, ckg up)", got["status"])
	}
}

// --- handleGetForTask ---

func TestHandleGetForTask_RequiresPrompt(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	req := callToolReq(map[string]any{}) // no "prompt"

	res, err := handleGetForTask(context.Background(), f.deps, req)
	if err != nil {
		t.Fatalf("handleGetForTask should return MCP error result, not Go error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError=true result for missing prompt; got %+v", res)
	}
}

func TestHandleGetForTask_HappyPath_ReturnsEvidencePack(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ckv.SearchHits = []contract.Hit{hit("login.go", 10, 30, 0.9, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("login.go", 10, 30, 8.0, contract.HitSourceCKG)}
		f.fetcher.Bodies = map[string]string{cit("login.go", 10, 30).Key(): "func Login() {}"}
	})

	req := callToolReq(map[string]any{"prompt": "find the Login handler"})
	res, err := handleGetForTask(context.Background(), f.deps, req)
	if err != nil {
		t.Fatalf("handleGetForTask: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError result: %s", resultText(res))
	}

	var pack contract.EvidencePack
	if err := decodeStructured(res, &pack); err != nil {
		t.Fatalf("decode pack: %v", err)
	}
	if pack.Query != "find the Login handler" {
		t.Errorf("Query = %q", pack.Query)
	}
	if pack.Metadata.IntegrityHash == "" {
		t.Error("IntegrityHash not stamped on returned pack")
	}
	if ok, err := contract.VerifyIntegrity(pack); err != nil || !ok {
		t.Errorf("VerifyIntegrity: ok=%v err=%v", ok, err)
	}
}

func TestHandleGetForTask_FailClosed_SurfacesAsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, func(f *fixture) {
		f.ruleset = &config.SanitizeRuleset{
			Version: 1,
			Rules: []config.SanitizeRule{{
				ID: "PK", Pattern: `PRIVATE KEY`, Action: contract.RedactionFailClosed, Severity: config.SeverityCritical,
			}},
		}
		f.ckv.SearchHits = []contract.Hit{hit("a.go", 1, 10, 0.5, contract.HitSourceCKV)}
		f.ckg.BM25Hits = []contract.Hit{hit("a.go", 1, 10, 5.0, contract.HitSourceCKG)}
		f.fetcher.Bodies = map[string]string{cit("a.go", 1, 10).Key(): "-----BEGIN PRIVATE KEY-----"}
	})

	req := callToolReq(map[string]any{"prompt": "anything"})
	res, err := handleGetForTask(context.Background(), f.deps, req)
	if err != nil {
		t.Fatalf("handleGetForTask should not return Go error for fail_closed: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("fail_closed must surface as IsError result; got %+v", res)
	}
	if !strings.Contains(resultText(res), "fail_closed") {
		t.Errorf("error text missing fail_closed marker: %q", resultText(res))
	}
	if !strings.Contains(resultText(res), "rule=PK") {
		t.Errorf("error text missing rule id PK: %q", resultText(res))
	}
}

// --- test helpers ---

func callToolReq(args map[string]any) mcpgo.CallToolRequest {
	var req mcpgo.CallToolRequest
	req.Params.Arguments = args
	return req
}

func unpackResult(t *testing.T, res *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	if res == nil {
		t.Fatal("nil CallToolResult")
	}
	if res.IsError {
		t.Fatalf("unexpected IsError result: %s", resultText(res))
	}
	var m map[string]any
	if err := decodeStructured(res, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

// decodeStructured unmarshals res.StructuredContent into target. mcp-go
// stores arbitrary structured payloads as `any`, so we marshal-then-unmarshal
// to get target-typed fields in the test.
func decodeStructured(res *mcpgo.CallToolResult, target any) error {
	if res == nil {
		return errors.New("nil result")
	}
	if res.StructuredContent == nil {
		return errors.New("nil StructuredContent")
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func resultText(res *mcpgo.CallToolResult) string {
	if res == nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcpgo.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
