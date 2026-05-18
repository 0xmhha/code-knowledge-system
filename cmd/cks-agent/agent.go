package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	mcpgoclient "github.com/mark3labs/mcp-go/client"
	mcpgotransport "github.com/mark3labs/mcp-go/client/transport"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// toolGetForTask names cks-mcp's get-for-task tool. cks-agent only
// needs this one tool from cks's surface for Phase-D Slim (prompt
// builder). Future agent capabilities can register additional tool
// names here.
const toolGetForTask = "cks.context.get_for_task"

// mcpClient is the seam over the upstream mcp-go *client.Client.
// Production code uses mcpgoclient.Client; tests inject a mock so the
// agent pipeline runs without spawning cks-mcp.
//
// This matches the pattern in internal/ckvclient.Real — keeping it
// here (not factored to a shared helper) is deliberate: cks-agent is
// designed to extract to a sibling repo, and duplicated mock seams
// are cheaper than a shared dependency that drags cks's package layout
// along.
type mcpClient interface {
	Initialize(ctx context.Context, req mcpgo.InitializeRequest) (*mcpgo.InitializeResult, error)
	CallTool(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	Close() error
}

// Compile-time guarantee mcp-go's *client.Client still satisfies our
// seam. Signature drift in mcp-go fails the build here rather than at
// first call.
var _ mcpClient = (*mcpgoclient.Client)(nil)

// AgentOpts configures NewAgent. CKSMCPBinary defaults to looking up
// "cks-mcp" on $PATH so a developer with `go install ./cmd/cks-mcp`
// can run the agent without touching config.
type AgentOpts struct {
	// CKSMCPBinary is the absolute path to the cks-mcp binary. Empty
	// means look up "cks-mcp" on $PATH.
	CKSMCPBinary string
	// CKSMCPConfig forwards as -config <path> to cks-mcp. Empty means
	// cks-mcp uses its own defaults (config.Default()).
	CKSMCPConfig string
	// Env extends the subprocess environment.
	Env []string
}

// Agent owns one cks-mcp subprocess connection and runs the
// vibe-prompt → markdown pipeline. Not safe for concurrent calls
// across goroutines without external serialization.
type Agent struct {
	client mcpClient
	closed bool
}

// NewAgent spawns `cks-mcp [-config <path>]` and runs the MCP
// initialize handshake. Failure to spawn or initialize closes any
// partial state before returning.
func NewAgent(ctx context.Context, opts AgentOpts) (*Agent, error) {
	bin := opts.CKSMCPBinary
	if bin == "" {
		bin = "cks-mcp"
	}
	args := make([]string, 0, 2)
	if opts.CKSMCPConfig != "" {
		args = append(args, "-config", opts.CKSMCPConfig)
	}
	tp := mcpgotransport.NewStdio(bin, opts.Env, args...)
	c := mcpgoclient.NewClient(tp)
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("cks-agent: start cks-mcp: %w", err)
	}
	return newAgentWithClient(ctx, c)
}

// newAgentWithClient is the test seam — accepts an arbitrary mcpClient
// (including the in-test mock) and runs the same initialize sequence.
func newAgentWithClient(ctx context.Context, c mcpClient) (*Agent, error) {
	req := mcpgo.InitializeRequest{}
	req.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcpgo.Implementation{
		Name:    "cks-agent",
		Version: "0.0.1",
	}
	if _, err := c.Initialize(ctx, req); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("cks-agent: initialize: %w", err)
	}
	return &Agent{client: c}, nil
}

// Run executes the agent pipeline: call cks.context.get_for_task,
// decode the returned EvidencePack, render it as markdown, and write
// to out.
//
// Errors at any stage are returned without partial output — out is
// untouched until the entire markdown document is ready.
func (a *Agent) Run(ctx context.Context, prompt string, out io.Writer) error {
	if prompt == "" {
		return errors.New("cks-agent: empty prompt")
	}

	req := mcpgo.CallToolRequest{}
	req.Params.Name = toolGetForTask
	req.Params.Arguments = map[string]any{"prompt": prompt}

	res, err := a.client.CallTool(ctx, req)
	if err != nil {
		return fmt.Errorf("cks-agent: CallTool: %w", err)
	}
	if res != nil && res.IsError {
		return fmt.Errorf("cks-agent: %s", concatText(res))
	}

	pack, err := decodePack(res)
	if err != nil {
		return fmt.Errorf("cks-agent: %w", err)
	}
	// A structurally-empty pack is most likely a server returning
	// garbage; surface that distinctly from a deliberate empty pack
	// (which would still carry the query string).
	if pack.Query == "" {
		return errors.New("cks-agent: decoded pack has empty query")
	}

	md := formatPack(pack)
	if _, err := io.WriteString(out, md); err != nil {
		return fmt.Errorf("cks-agent: write: %w", err)
	}
	return nil
}

// Close shuts down the underlying mcp-go client (and subprocess in the
// production transport path). Idempotent.
func (a *Agent) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	return a.client.Close()
}

// --- helpers ---

// decodePack extracts an evidencePack from a CallToolResult. Prefers
// StructuredContent; falls back to parsing the JSON text in the first
// text content block. Either path produces the same evidencePack
// because cks-mcp uses NewToolResultStructured which embeds both
// shapes.
func decodePack(res *mcpgo.CallToolResult) (evidencePack, error) {
	if res == nil {
		return evidencePack{}, errors.New("nil tool result")
	}
	if res.StructuredContent != nil {
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return evidencePack{}, fmt.Errorf("marshal structured: %w", err)
		}
		var p evidencePack
		if err := json.Unmarshal(raw, &p); err != nil {
			return evidencePack{}, fmt.Errorf("decode structured pack: %w", err)
		}
		return p, nil
	}
	txt := concatText(res)
	if txt == "" {
		return evidencePack{}, errors.New("empty tool result")
	}
	var p evidencePack
	if err := json.Unmarshal([]byte(txt), &p); err != nil {
		return evidencePack{}, fmt.Errorf("decode text pack: %w", err)
	}
	return p, nil
}

// concatText joins all TextContent blocks from res into one string.
// Used for both error surfacing and the text-fallback decode path.
func concatText(res *mcpgo.CallToolResult) string {
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
