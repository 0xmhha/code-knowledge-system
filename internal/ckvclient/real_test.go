package ckvclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- mockMCPClient ---
//
// Stands in for mcp-go's *client.Client. Tests configure canned responses
// on CallTool and assert the (name, arguments) arrived as expected. Init
// + Close are tracked for lifecycle assertions.

type mockMCPClient struct {
	initCalls   int
	initErr     error
	initResult  *mcpgo.InitializeResult
	callOut     map[string]*mcpgo.CallToolResult
	callErr     map[string]error
	calls       []toolCall
	closed      bool
	closeErr    error
}

type toolCall struct {
	name string
	args map[string]any
}

func (m *mockMCPClient) Initialize(ctx context.Context, req mcpgo.InitializeRequest) (*mcpgo.InitializeResult, error) {
	m.initCalls++
	if m.initErr != nil {
		return nil, m.initErr
	}
	if m.initResult != nil {
		return m.initResult, nil
	}
	return &mcpgo.InitializeResult{ProtocolVersion: mcpgo.LATEST_PROTOCOL_VERSION}, nil
}

func (m *mockMCPClient) CallTool(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	m.calls = append(m.calls, toolCall{name: req.Params.Name, args: args})
	if err, ok := m.callErr[req.Params.Name]; ok && err != nil {
		return nil, err
	}
	if res, ok := m.callOut[req.Params.Name]; ok {
		return res, nil
	}
	return nil, errors.New("mock: no canned response for " + req.Params.Name)
}

func (m *mockMCPClient) Close() error {
	m.closed = true
	return m.closeErr
}

// --- helpers ---

// textResult wraps a string payload as a ckv-style text content result.
func textResult(text string) *mcpgo.CallToolResult {
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: text}},
	}
}

func errorResult(text string) *mcpgo.CallToolResult {
	return &mcpgo.CallToolResult{
		IsError: true,
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: text}},
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func mockedReal(t *testing.T, m *mockMCPClient) *Real {
	t.Helper()
	r, err := newRealWithClient(context.Background(), m)
	if err != nil {
		t.Fatalf("newRealWithClient: %v", err)
	}
	return r
}

// --- Initialize / lifecycle ---

func TestRealCKV_NewReal_RunsInitialize(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	_ = mockedReal(t, m)
	if m.initCalls != 1 {
		t.Errorf("Initialize calls = %d, want 1", m.initCalls)
	}
}

func TestRealCKV_NewReal_InitErrorClosesClient(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{initErr: errors.New("ckv mcp init failed")}
	if _, err := newRealWithClient(context.Background(), m); err == nil {
		t.Fatal("expected error from Initialize")
	}
	if !m.closed {
		t.Error("client should be Close()d on init failure")
	}
}

// --- SemanticSearch ---

func TestRealCKV_SemanticSearch_TranslatesHits(t *testing.T) {
	t.Parallel()
	resp := map[string]any{
		"hits": []map[string]any{
			{
				"chunk_id": "c1",
				"citation": map[string]any{
					"file":        "login.go",
					"start_line":  10,
					"end_line":    30,
					"commit_hash": "deadbeef",
				},
				"snippet": "func Login() {}",
				"score": map[string]any{
					"normalized":      0.95,
					"vector_distance": 0.1,
					"vector_rank":     1,
				},
				"language":    "go",
				"symbol":      "Login",
				"symbol_kind": "Function",
			},
			{
				"chunk_id": "c2",
				"citation": map[string]any{
					"file":        "auth.go",
					"start_line":  5,
					"end_line":    25,
					"commit_hash": "deadbeef",
				},
				"snippet": "func validate() bool { return true }",
				"score": map[string]any{
					"normalized":      0.72,
					"vector_distance": 0.56,
					"vector_rank":     2,
				},
				"language":    "go",
				"symbol":      "validate",
				"symbol_kind": "Function",
			},
		},
	}
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolSemanticSearch: textResult(mustJSON(t, resp)),
		},
	}
	r := mockedReal(t, m)

	hits, err := r.SemanticSearch(context.Background(), "find login flow", SearchOpts{K: 5})
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	if hits[0].Citation.File != "login.go" || hits[0].Citation.StartLine != 10 {
		t.Errorf("hit[0] citation wrong: %+v", hits[0].Citation)
	}
	if hits[0].Source != contract.HitSourceCKV {
		t.Errorf("hit[0] source = %q, want HitSourceCKV", hits[0].Source)
	}
	if hits[0].Score != 0.95 {
		t.Errorf("hit[0] score = %v, want 0.95", hits[0].Score)
	}
	if hits[0].Rank != 1 || hits[1].Rank != 2 {
		t.Errorf("ranks = %d,%d want 1,2", hits[0].Rank, hits[1].Rank)
	}
	// CommitHash must come from ckv's per-citation commit_hash field.
	if hits[0].Citation.CommitHash != "deadbeef" {
		t.Errorf("CommitHash = %q, want deadbeef", hits[0].Citation.CommitHash)
	}
}

func TestRealCKV_SemanticSearch_ForwardsFilterArgs(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolSemanticSearch: textResult(`{"hits":[]}`),
		},
	}
	r := mockedReal(t, m)

	opts := SearchOpts{
		K: 12,
		Filter: SearchFilter{
			Language:    "go",
			PathGlob:    "internal/**",
			SymbolKinds: []string{"function", "method"},
		},
	}
	if _, err := r.SemanticSearch(context.Background(), "x", opts); err != nil {
		t.Fatal(err)
	}
	if len(m.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(m.calls))
	}
	args := m.calls[0].args
	if args["intent"] != "x" {
		t.Errorf("intent = %v", args["intent"])
	}
	if got, _ := args["k"].(int); got != 12 {
		t.Errorf("k = %v, want 12", args["k"])
	}
	if args["language"] != "go" {
		t.Errorf("language = %v", args["language"])
	}
	if args["path"] != "internal/**" {
		t.Errorf("path = %v", args["path"])
	}
	// ckv accepts a single symbol_kind per call. Cks Filter.SymbolKinds
	// is a slice; the adapter forwards the first entry.
	if args["symbol_kind"] != "function" {
		t.Errorf("symbol_kind = %v, want function (first of slice)", args["symbol_kind"])
	}
}

func TestRealCKV_SemanticSearch_EmptyQueryErrors(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	r := mockedReal(t, m)
	if _, err := r.SemanticSearch(context.Background(), "", SearchOpts{}); err == nil {
		t.Fatal("expected error")
	}
	if len(m.calls) != 0 {
		t.Errorf("backend invoked despite empty query: %v", m.calls)
	}
}

func TestRealCKV_SemanticSearch_ToolErrorReported(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolSemanticSearch: errorResult("index unavailable: no manifest"),
		},
	}
	r := mockedReal(t, m)
	_, err := r.SemanticSearch(context.Background(), "q", SearchOpts{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no manifest") {
		t.Errorf("err = %v, want substring \"no manifest\"", err)
	}
}

func TestRealCKV_SemanticSearch_TransportErrorPropagates(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{
		callErr: map[string]error{toolSemanticSearch: errors.New("subprocess died")},
	}
	r := mockedReal(t, m)
	_, err := r.SemanticSearch(context.Background(), "q", SearchOpts{})
	if err == nil || !strings.Contains(err.Error(), "subprocess died") {
		t.Fatalf("err = %v, want substring \"subprocess died\"", err)
	}
}

// --- Health ---

func TestRealCKV_Health_TranslatesManifestPayload(t *testing.T) {
	t.Parallel()
	resp := map[string]any{
		"server":          "ckv",
		"server_version":  "0.1.0-S1W3",
		"embedding_model": "bge-code-v1",
		"embedding_dim":   1024,
		"indexed_head":    "abc123",
		"chunk_count":     4200,
		"built_at":        "2026-05-15T10:00:00Z",
		"src_root":        "/repo",
	}
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolHealth: textResult(mustJSON(t, resp)),
		},
	}
	r := mockedReal(t, m)

	h, err := r.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !h.Reachable {
		t.Error("Reachable should be true on successful health call")
	}
	// StatsHash is synthesized as "<model>@<head>" — a synthesis that
	// gives both reproducibility (changes when index changes) and
	// traceability (you can tell which embedder was used).
	if h.StatsHash != "bge-code-v1@abc123" {
		t.Errorf("StatsHash = %q, want \"bge-code-v1@abc123\"", h.StatsHash)
	}
	want := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	if !h.LastIndexAt.Equal(want) {
		t.Errorf("LastIndexAt = %v, want %v", h.LastIndexAt, want)
	}
}

func TestRealCKV_Health_ToolErrorReturnsUnreachable(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolHealth: errorResult("index unavailable"),
		},
	}
	r := mockedReal(t, m)
	h, err := r.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if h.Reachable {
		t.Error("Reachable should be false on error path")
	}
}

func TestRealCKV_Health_TransportErrorReturnsUnreachable(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{
		callErr: map[string]error{toolHealth: errors.New("eof")},
	}
	r := mockedReal(t, m)
	h, err := r.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if h.Reachable {
		t.Error("Reachable should be false on transport error")
	}
}

// --- Close ---

func TestRealCKV_Close_IsIdempotent(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	r := mockedReal(t, m)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !m.closed {
		t.Error("underlying Close not called")
	}
}

// --- Compile-time guarantee ---

func TestRealCKV_ImplementsClient(t *testing.T) {
	t.Parallel()
	var _ Client = (*Real)(nil)
}
