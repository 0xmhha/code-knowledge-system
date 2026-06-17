package eval

import (
	"context"
	"errors"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- mockMCPClient ---

type mockMCPClient struct {
	initCalls int
	initErr   error
	callOut   map[string]*mcpgo.CallToolResult
	callErr   map[string]error
	calls     []toolCall
	closed    bool
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
	return nil
}

// --- helpers ---

func packResult(pack contract.EvidencePack) *mcpgo.CallToolResult {
	return mcpgo.NewToolResultStructured(pack, "evidence pack")
}

func errorResult(text string) *mcpgo.CallToolResult {
	return &mcpgo.CallToolResult{
		IsError: true,
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: text}},
	}
}

// --- Runner.Execute ---

func TestRunner_Execute_HappyPath_ComputesMetrics(t *testing.T) {
	t.Parallel()
	pack := contract.EvidencePack{
		Query: "find login",
		Citations: []contract.Citation{
			cit("login.go", 10, 30),
			cit("auth.go", 5, 25),
		},
		Metadata: contract.PackMetadata{
			BudgetTokens:     4000,
			UsedTokens:       1200,
			UtilizationRatio: 0.3,
		},
	}
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolGetForTask: packResult(pack),
		},
	}
	r := &Runner{client: m}

	s := &Scenario{
		Version:   1,
		Name:      "test-1",
		Prompt:    "find login",
		MatchMode: MatchOverlap,
		Runs:      1,
		ExpectedCitations: []contract.Citation{
			cit("login.go", 10, 30),
			cit("auth.go", 5, 25),
		},
	}

	result, err := r.Execute(context.Background(), s)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Name != "test-1" {
		t.Errorf("Name = %q", result.Name)
	}
	if !approxEq(result.Metrics.FilePrecision, 1.0) {
		t.Errorf("FilePrecision = %.4f", result.Metrics.FilePrecision)
	}
	if !approxEq(result.Metrics.FileRecall, 1.0) {
		t.Errorf("FileRecall = %.4f", result.Metrics.FileRecall)
	}
	if !approxEq(result.Metrics.TokenUtilization, 0.3) {
		t.Errorf("TokenUtilization = %.4f", result.Metrics.TokenUtilization)
	}
	if result.Metrics.CitationCount != 2 {
		t.Errorf("CitationCount = %d", result.Metrics.CitationCount)
	}
	// Each Execute call (runs=1) issues exactly one tool call.
	if len(m.calls) != 1 || m.calls[0].name != toolGetForTask {
		t.Errorf("calls = %v", m.calls)
	}
	if m.calls[0].args["prompt"] != "find login" {
		t.Errorf("prompt arg = %v", m.calls[0].args["prompt"])
	}
}

func TestRunner_Execute_TakesMedianAcrossRuns(t *testing.T) {
	t.Parallel()
	// Runs=3 → three identical calls. Backend returns the same pack
	// (cks is deterministic with fixed inputs), so the median equals
	// the per-run value.
	pack := contract.EvidencePack{
		Query: "x",
		Citations: []contract.Citation{
			cit("a.go", 1, 10),
		},
	}
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolGetForTask: packResult(pack),
		},
	}
	r := &Runner{client: m}

	s := &Scenario{
		Version: 1, Name: "x", Prompt: "x",
		MatchMode: MatchOverlap, Runs: 3,
		ExpectedCitations: []contract.Citation{
			cit("a.go", 1, 10),
		},
	}
	result, err := r.Execute(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.calls) != 3 {
		t.Errorf("tool calls = %d, want 3", len(m.calls))
	}
	if !approxEq(result.Metrics.FileRecall, 1.0) {
		t.Errorf("recall after median = %.4f, want 1.0", result.Metrics.FileRecall)
	}
	if result.Runs != 3 {
		t.Errorf("Runs = %d, want 3", result.Runs)
	}
}

func TestRunner_Execute_ToolErrorRecordedAsError(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{
		callOut: map[string]*mcpgo.CallToolResult{
			toolGetForTask: errorResult("cks.context.get_for_task: composer: sanitize fail_closed: rule=PK"),
		},
	}
	r := &Runner{client: m}
	s := &Scenario{
		Version: 1, Name: "fc", Prompt: "x", MatchMode: MatchOverlap, Runs: 1,
	}
	result, err := r.Execute(context.Background(), s)
	// Per-run errors are recorded on the result, not returned — a
	// scenario with one bad run still produces a row in the report.
	if err != nil {
		t.Fatalf("Execute should not error on tool failure: %v", err)
	}
	if result.Error == "" {
		t.Error("Result.Error should carry the tool error text")
	}
}

func TestRunner_Execute_RejectsEmptyScenario(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	r := &Runner{client: m}
	if _, err := r.Execute(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil scenario")
	}
}

// --- New / Close ---

func TestRunner_NewWithClient_RunsInitialize(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	if _, err := newRunnerWithClient(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if m.initCalls != 1 {
		t.Errorf("Initialize calls = %d, want 1", m.initCalls)
	}
}

func TestRunner_Close_Idempotent(t *testing.T) {
	t.Parallel()
	m := &mockMCPClient{}
	r, err := newRunnerWithClient(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
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
