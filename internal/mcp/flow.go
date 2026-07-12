package mcp

// Phase D flow-aware MCP tools (D-4, agreed Phase 2 deliverable). These expose
// the CKV flow corpus on the cks.context.* surface as DIRECT-CALL tools —
// separate from the get_for_task composite — so coding-agent's analyzer /
// diagnose can drive root-cause-lifecycle (produce→store→consume) by tool call.
//
// Wiring status: the tools are registered now (surface visible to callers), but
// the backend bodies live in ckvclient and are stubbed until CKV ships the
// pkg/ckv.Engine flow methods (coordination §9.2-R, T4). Until then a call
// returns a clear "flow surface not yet available" error rather than a wrong
// answer. Deps.CKV is type-asserted to ckvclient.FlowClient; a backend that
// does not implement it yields the same unavailable signal.

import (
	"context"
	"errors"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
)

// Tool names — exported so callers and tests can reference them without drift.
const (
	ToolNameGetFlow                 = "cks.context.get_flow"
	ToolNameExpandFlow              = "cks.context.expand_flow"
	ToolNameFindBranches            = "cks.context.find_branches"
	ToolNameGetInvariantEnforcement = "cks.context.get_invariant_enforcement"
	ToolNameFindInvariants          = "cks.context.find_invariants"
	ToolNameGetConventions          = "cks.context.get_conventions"
)

// flowClient extracts the optional flow surface from Deps.CKV, or returns a
// tool error when the backend does not expose it (Smart Dummy / pre-Phase-D).
func flowClient(d Deps, toolName string) (ckvclient.FlowClient, *mcpgo.CallToolResult) {
	fc, ok := d.CKV.(ckvclient.FlowClient)
	if !ok {
		return nil, mcpgo.NewToolResultErrorf("%s: flow surface not available on this backend", toolName)
	}
	return fc, nil
}

// flowErrResult maps a flow backend error onto a tool result, giving the
// ErrFlowUnsupported sentinel a stable, caller-readable message.
func flowErrResult(toolName string, err error) *mcpgo.CallToolResult {
	if errors.Is(err, ckvclient.ErrFlowUnsupported) {
		return mcpgo.NewToolResultErrorf("%s: flow surface not yet available (pending CKV Phase D release)", toolName)
	}
	return mcpgo.NewToolResultErrorf("%s: %v", toolName, err)
}

// registerGetFlow wires cks.context.get_flow.
func registerGetFlow(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameGetFlow,
		mcpgo.WithDescription(
			"Return a curated flow as a topological step sequence (cycle-safe): symbol, "+
				"citation, calls/reads/writes/emits, branches, invariants per step. Select by "+
				"flow_id / entry-point ID / invariant_id -- these are curated IDs matched "+
				"exactly, NOT code symbols. From a natural-language symptom, call "+
				"find_branches first, then get_flow(flow_id).",
		),
		mcpgo.WithString("flow_id", mcpgo.Description("Flow identifier.")),
		mcpgo.WithString("entry_point", mcpgo.Description("Entry-point symbol of the flow.")),
		mcpgo.WithString("invariant_id", mcpgo.Description("Invariant whose enforcing flow to return.")),
		mcpgo.WithNumber("max_steps", mcpgo.Description("Cap on returned steps (0 = backend default).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleGetFlow(ctx, d, req)
	})
}

func handleGetFlow(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	q := ckvclient.FlowQuery{
		FlowID:      req.GetString("flow_id", ""),
		EntryPoint:  req.GetString("entry_point", ""),
		InvariantID: req.GetString("invariant_id", ""),
		MaxSteps:    intArg(req, "max_steps", 0),
	}
	if q.FlowID == "" && q.EntryPoint == "" && q.InvariantID == "" {
		return mcpgo.NewToolResultError(ToolNameGetFlow + ": provide one of flow_id, entry_point, or invariant_id"), nil
	}
	fc, errRes := flowClient(d, ToolNameGetFlow)
	if errRes != nil {
		return errRes, nil
	}
	flow, err := fc.GetFlow(ctx, q)
	if err != nil {
		return flowErrResult(ToolNameGetFlow, err), nil
	}
	return mcpgo.NewToolResultStructured(flow, "get_flow result"), nil
}

// registerExpandFlow wires cks.context.expand_flow.
func registerExpandFlow(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameExpandFlow,
		mcpgo.WithDescription(
			"Steps adjacent to a flow step: direction 'up' traverses producers, 'down' "+
				"traverses consumers. Use to trace a value's produce->store->consume "+
				"lifecycle one hop at a time when the whole flow is too large.",
		),
		mcpgo.WithString("step_id", mcpgo.Required(), mcpgo.Description("Origin step id.")),
		mcpgo.WithString("direction", mcpgo.Description("\"up\" (producers) or \"down\" (consumers). Default \"down\".")),
		mcpgo.WithNumber("hops", mcpgo.Description("Traversal hops (default 1).")),
		mcpgo.WithNumber("limit", mcpgo.Description("Cap on returned neighbors (0 = backend default).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleExpandFlow(ctx, d, req)
	})
}

func handleExpandFlow(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	stepID := req.GetString("step_id", "")
	if stepID == "" {
		return mcpgo.NewToolResultError(ToolNameExpandFlow + ": missing required argument \"step_id\""), nil
	}
	direction := req.GetString("direction", "down")
	if direction != "up" && direction != "down" {
		return mcpgo.NewToolResultError(ToolNameExpandFlow + ": direction must be \"up\" or \"down\""), nil
	}
	fc, errRes := flowClient(d, ToolNameExpandFlow)
	if errRes != nil {
		return errRes, nil
	}
	exp, err := fc.ExpandFlow(ctx, ckvclient.ExpandFlowQuery{
		StepID:    stepID,
		Direction: direction,
		Hops:      intArg(req, "hops", 1),
		Limit:     intArg(req, "limit", 0),
	})
	if err != nil {
		return flowErrResult(ToolNameExpandFlow, err), nil
	}
	return mcpgo.NewToolResultStructured(exp, "expand_flow result"), nil
}

// findBranchesResponse is the wire shape for find_branches.
type findBranchesResponse struct {
	Matches []ckvclient.BranchMatch `json:"matches"`
}

// registerFindBranches wires cks.context.find_branches.
func registerFindBranches(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameFindBranches,
		mcpgo.WithDescription(
			"Map a free-text symptom to ranked when->then@at failure-condition candidates "+
				"from the flow corpus. This is the natural-language ENTRY POINT into flows -- "+
				"use it before get_flow when all you have is a symptom.",
		),
		mcpgo.WithString("symptom_text", mcpgo.Required(), mcpgo.Description("Free-text symptom description.")),
		mcpgo.WithNumber("k", mcpgo.Description("Max matches to return (default 10).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleFindBranches(ctx, d, req)
	})
}

func handleFindBranches(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	symptom := req.GetString("symptom_text", "")
	if symptom == "" {
		return mcpgo.NewToolResultError(ToolNameFindBranches + ": missing required argument \"symptom_text\""), nil
	}
	fc, errRes := flowClient(d, ToolNameFindBranches)
	if errRes != nil {
		return errRes, nil
	}
	matches, err := fc.FindBranches(ctx, symptom, intArg(req, "k", 10))
	if err != nil {
		return flowErrResult(ToolNameFindBranches, err), nil
	}
	return mcpgo.NewToolResultStructured(findBranchesResponse{Matches: matches}, "find_branches result"), nil
}

// registerGetInvariantEnforcement wires cks.context.get_invariant_enforcement.
func registerGetInvariantEnforcement(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameGetInvariantEnforcement,
		mcpgo.WithDescription(
			"Enumerate every site that enforces a curated invariant. Use to check a "+
				"planned change against domain rules (the 'empty block implies same state "+
				"root' class) before implementing.",
		),
		mcpgo.WithString("inv_id", mcpgo.Required(), mcpgo.Description("Invariant identifier.")),
		mcpgo.WithNumber("max", mcpgo.Description("Cap on enforcement sites (0 = backend default).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleGetInvariantEnforcement(ctx, d, req)
	})
}

func handleGetInvariantEnforcement(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	invID := req.GetString("inv_id", "")
	if invID == "" {
		return mcpgo.NewToolResultError(ToolNameGetInvariantEnforcement + ": missing required argument \"inv_id\""), nil
	}
	fc, errRes := flowClient(d, ToolNameGetInvariantEnforcement)
	if errRes != nil {
		return errRes, nil
	}
	enf, err := fc.GetInvariantEnforcement(ctx, invID, intArg(req, "max", 0))
	if err != nil {
		return flowErrResult(ToolNameGetInvariantEnforcement, err), nil
	}
	return mcpgo.NewToolResultStructured(enf, "get_invariant_enforcement result"), nil
}

// registerFindInvariants wires cks.context.find_invariants.
func registerFindInvariants(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameFindInvariants,
		mcpgo.WithDescription(
			"List curated invariants (domain rules) matching a filter. Scope by "+
				"file, policy category, or minimum confidence tier. Use on the "+
				"diagnose path to check a planned change against the rules that "+
				"govern the touched code before implementing.",
		),
		mcpgo.WithString("file", mcpgo.Description("Restrict to one source file (\"\" = any).")),
		mcpgo.WithString("category", mcpgo.Description("Filter by policy category (\"\" = any).")),
		mcpgo.WithNumber("tier_min", mcpgo.Description("Minimum confidence tier 1|2|3 (0 = backend default).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleFindInvariants(ctx, d, req)
	})
}

func handleFindInvariants(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	fc, errRes := flowClient(d, ToolNameFindInvariants)
	if errRes != nil {
		return errRes, nil
	}
	hits, err := fc.FindInvariants(ctx, req.GetString("file", ""), req.GetString("category", ""), intArg(req, "tier_min", 0))
	if err != nil {
		return flowErrResult(ToolNameFindInvariants, err), nil
	}
	return mcpgo.NewToolResultStructured(hits, "find_invariants result"), nil
}

// registerGetConventions wires cks.context.get_conventions.
func registerGetConventions(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameGetConventions,
		mcpgo.WithDescription(
			"Return per-package AST-convention summaries (the codebase's idioms: "+
				"error handling, locking, naming) under a package prefix. Use to "+
				"match new code to how the surrounding package already does things.",
		),
		mcpgo.WithString("package_prefix", mcpgo.Description("Package path prefix (\"\" = all packages).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleGetConventions(ctx, d, req)
	})
}

func handleGetConventions(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	fc, errRes := flowClient(d, ToolNameGetConventions)
	if errRes != nil {
		return errRes, nil
	}
	hits, err := fc.GetConventions(ctx, req.GetString("package_prefix", ""))
	if err != nil {
		return flowErrResult(ToolNameGetConventions, err), nil
	}
	return mcpgo.NewToolResultStructured(hits, "get_conventions result"), nil
}
