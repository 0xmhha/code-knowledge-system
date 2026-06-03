// Package mcp exposes the cks composer pipeline as an MCP (Model Context
// Protocol) server over stdio.
//
// Register wires the 13 agent-facing cks.* tools (the C1 surface): the
// context tools (get_for_task, semantic_search, search_text, find_symbol,
// find_callers, find_callees, get_subgraph, impact_analysis,
// concurrency_impact, change_history) and the ops tools (health, freshness,
// index). The exact registered set is pinned against the SSoT fixture by
// schema_golden_test.go (M2.a).
//
// The package is intentionally thin: Register attaches handlers to an
// already-constructed *server.MCPServer so callers retain control over
// the server's name/version and any non-tool capabilities (resources,
// prompts) that future phases may add. Run is a convenience that
// constructs the server and serves stdio in one call.
package mcp

import (
	"context"
	"errors"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/composer"
)

// ToolNameGetForTask is the wire name of the get_for_task tool. Exported
// so callers (and other tests) can reference it without string drift.
const ToolNameGetForTask = "cks.context.get_for_task"

// ToolNameHealth is the wire name of the health tool.
const ToolNameHealth = "cks.ops.health"

// Deps bundles everything an MCP handler needs. Keep this struct small:
// the slim C.5 surface deliberately resists envelope sprawl (HLD §7.5
// envelope/auth fields land with Phase 3, not here).
type Deps struct {
	// Composer drives cks.context.get_for_task. Must be non-nil.
	Composer *composer.Composer

	// CKG and CKV are reported by cks.ops.health. They are NOT used to
	// short-circuit composer calls — the composer holds its own references
	// to its stage dependencies. Health is a transparent proxy.
	CKG ckgclient.Client
	CKV ckvclient.Client

	// BuilderVersion is echoed in cks.ops.health responses and helps a
	// caller correlate health output with running binary build tags.
	// Empty string is acceptable; the field is informational, not load-bearing.
	BuilderVersion string

	// Index configures the cks.ops.index maintenance tool (G8). Zero value
	// (no binaries) disables it — the tool then tells the agent to run the
	// indexers manually. Not used by the query path.
	Index IndexConfig
}

// Register attaches both tools to s. Returns an error when required Deps
// fields are nil — the slim surface refuses to start half-wired rather
// than silently emit "tool not registered" at call time.
func Register(s *mcpserver.MCPServer, d Deps) error {
	if s == nil {
		return errors.New("mcp: nil MCPServer")
	}
	if d.Composer == nil {
		return errors.New("mcp: nil Deps.Composer")
	}
	if d.CKG == nil {
		return errors.New("mcp: nil Deps.CKG")
	}
	if d.CKV == nil {
		return errors.New("mcp: nil Deps.CKV")
	}

	registerGetForTask(s, d)
	registerHealth(s, d)
	registerFindSymbol(s, d)
	registerFindCallers(s, d)
	registerFindCallees(s, d)
	registerGetSubgraph(s, d)
	registerImpactAnalysis(s, d)
	registerConcurrencyImpact(s, d)
	registerChangeHistory(s, d)
	registerSemanticSearch(s, d)
	registerSearchText(s, d)
	registerFreshness(s, d)
	registerOpsIndex(s, d)
	return nil
}

// Run constructs an MCP server named "cks" (version v from BuilderVersion
// or a fallback), registers the tools, and serves stdio until ctx is
// cancelled or stdin closes. Intended entry point for cmd/cks-mcp.
func Run(ctx context.Context, d Deps) error {
	v := d.BuilderVersion
	if v == "" {
		v = "0.0.0"
	}
	s := mcpserver.NewMCPServer("cks", v)
	if err := Register(s, d); err != nil {
		return fmt.Errorf("mcp: register: %w", err)
	}
	// mcp-go's stdio server does not currently surface ctx into its
	// loop; the caller's ctx still gates anything Compose / Health do
	// (each handler threads ctx through to the composer / clients).
	_ = ctx
	if err := mcpserver.ServeStdio(s); err != nil {
		return fmt.Errorf("mcp: serve stdio: %w", err)
	}
	return nil
}

// registerGetForTask wires the cks.context.get_for_task tool. The schema
// is deliberately one input field; richer envelope fields land in a
// later phase.
func registerGetForTask(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameGetForTask,
		mcpgo.WithDescription(
			"Compose a sanitized EvidencePack from a vibe prompt. Runs intent classification, "+
				"keyword extraction (ckv+BM25), citation search (ckg), graph expansion, token "+
				"budgeting, and policy sanitize. Returns a SHA-256 integrity-stamped pack ready "+
				"for an upper-layer LLM consumer.",
		),
		mcpgo.WithString("prompt", mcpgo.Required(),
			mcpgo.Description("Natural-language task description. Example: \"find where ProcessRequest validates input\".")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleGetForTask(ctx, d, req)
	})
}

// registerHealth wires the cks.ops.health tool.
func registerHealth(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameHealth,
		mcpgo.WithDescription(
			"Aggregate cks backend health. Reports ok | degraded | down based on ckg/ckv "+
				"reachability per HLD §10. ckg is required; ckv unavailable yields degraded.",
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleHealth(ctx, d, req)
	})
}
