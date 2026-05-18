package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// --- mockMCPClient ---
//
// Mirrors the seam used in internal/ckvclient.Real_test — fake the
// upstream MCP client so the agent pipeline can be exercised without
// spawning a real cks-mcp subprocess.

type mockMCPClient struct {
	initCalls  int
	initErr    error
	initResult *mcpgo.InitializeResult

	callOut map[string]*mcpgo.CallToolResult
	callErr map[string]error
	calls   []toolCall

	closed   bool
	closeErr error
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

func structuredResult(payload any) *mcpgo.CallToolResult {
	return mcpgo.NewToolResultStructured(payload, "evidence pack")
}

func errorResult(text string) *mcpgo.CallToolResult {
	return &mcpgo.CallToolResult{
		IsError: true,
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: text}},
	}
}

// --- Agent.Run ---

func TestAgent_Run_EmptyPromptErrors(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	a := &Agent{client: m}
	var buf bytes.Buffer
	err := a.Run(context.Background(), "", &buf)
	if err == nil {
		t.Fatal("expected error on empty prompt")
	}
	if len(m.calls) != 0 {
		t.Errorf("backend invoked despite empty prompt: %v", m.calls)
	}
}

func TestAgent_Run_HappyPath_WritesFormattedMarkdown(t *testing.T) {
	t.Parallel()
	pack := evidencePack{
		Query:  "find login flow",
		Intent: "feature_add",
		Citations: []citation{
			{File: "login.go", StartLine: 10, EndLine: 30, CommitHash: "abc"},
		},
		Bodies: []body{
			{
				Citation: citation{File: "login.go", StartLine: 10, EndLine: 30, CommitHash: "abc"},
				Text:     "func Login() {}",
			},
		},
		Metadata: packMetadata{
			BuilderVersion: "cks-mcp/test",
			IntegrityHash:  "h1",
		},
	}
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolGetForTask: structuredResult(pack),
		},
	}
	a := &Agent{client: m}

	var buf bytes.Buffer
	if err := a.Run(context.Background(), "find login flow", &buf); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Tool call wiring
	if len(m.calls) != 1 || m.calls[0].name != toolGetForTask {
		t.Errorf("calls = %v, want one to %q", m.calls, toolGetForTask)
	}
	if m.calls[0].args["prompt"] != "find login flow" {
		t.Errorf("prompt forwarded = %v, want \"find login flow\"", m.calls[0].args["prompt"])
	}

	out := buf.String()
	// Sanity-check key sections from the formatter, not the exact bytes.
	for _, want := range []string{
		"# Task",
		"find login flow",
		"feature_add",
		"# Relevant code",
		"login.go:10-30",
		"func Login()",
		"# Pack metadata",
		"cks-mcp/test",
		"h1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestAgent_Run_GetForTaskFailClosed_ReturnsError(t *testing.T) {
	t.Parallel()
	// cks.context.get_for_task surfaces fail_closed as an IsError result
	// with the rule id in the text. The agent must surface that, not
	// silently emit an empty pack.
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolGetForTask: errorResult("cks.context.get_for_task: composer: sanitize fail_closed: rule=PK"),
		},
	}
	a := &Agent{client: m}
	var buf bytes.Buffer
	err := a.Run(context.Background(), "anything", &buf)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "fail_closed") || !strings.Contains(err.Error(), "rule=PK") {
		t.Errorf("err missing fail_closed / rule id: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("agent wrote partial output on error: %q", buf.String())
	}
}

func TestAgent_Run_DecodeErrorPropagates(t *testing.T) {
	t.Parallel()
	// Hand back a non-EvidencePack structured payload — the agent must
	// fail with a clear decode error rather than render garbage.
	bad := mcpgo.NewToolResultStructured(map[string]any{"not": "a pack"}, "evidence pack")
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{toolGetForTask: bad},
	}
	a := &Agent{client: m}
	var buf bytes.Buffer
	err := a.Run(context.Background(), "x", &buf)
	// A struct with only an unknown field decodes into evidencePack with
	// every field zero — that IS a valid (but empty) pack. So this
	// specifically tests the path where Query is empty after decode,
	// which means the server gave us garbage.
	if err == nil || !strings.Contains(err.Error(), "decoded pack has empty query") {
		t.Fatalf("err = %v, want \"decoded pack has empty query\" sentinel", err)
	}
}

func TestAgent_Run_TextFallback_ParsesEmbeddedJSON(t *testing.T) {
	t.Parallel()
	// Older / non-structured callers may receive the pack as a JSON-text
	// content block instead of StructuredContent. The agent should fall
	// back to decoding the text payload.
	pack := evidencePack{
		Query:    "x",
		Intent:   "bug_fix",
		Metadata: packMetadata{IntegrityHash: "abc"},
	}
	raw, _ := json.Marshal(pack)
	textOnly := &mcpgo.CallToolResult{
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: string(raw)}},
	}
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{toolGetForTask: textOnly},
	}
	a := &Agent{client: m}

	var buf bytes.Buffer
	if err := a.Run(context.Background(), "x", &buf); err != nil {
		t.Fatalf("text-fallback path errored: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "bug_fix") || !strings.Contains(out, "abc") {
		t.Errorf("text-fallback decode produced wrong output: %q", out)
	}
}

// --- newAgent (constructor) ---

func TestNewAgent_RunsInitialize(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	a, err := newAgentWithClient(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if m.initCalls != 1 {
		t.Errorf("Initialize calls = %d, want 1", m.initCalls)
	}
	if a.client == nil {
		t.Error("client not stored")
	}
}

func TestNewAgent_InitErrorClosesClient(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{initErr: errors.New("cks-mcp init failed")}
	if _, err := newAgentWithClient(context.Background(), m); err == nil {
		t.Fatal("expected error")
	}
	if !m.closed {
		t.Error("client should be closed on init failure")
	}
}

// --- Close ---

func TestAgent_Close_Idempotent(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	a, err := newAgentWithClient(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if !m.closed {
		t.Error("underlying Close not called")
	}
}
