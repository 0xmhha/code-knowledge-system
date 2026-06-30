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
			"Return a whole flow as a topological step sequence (cycle-safe). Each step carries "+
				"its symbol, canonical_id, citation, calls/reads/writes/emits, branches, and "+
				"invariants. Select the flow by exactly one of flow_id, entry_point, or invariant_id.",
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
			"Return the steps adjacent to a step. direction \"up\" walks producers, \"down\" walks "+
				"consumers. Bounded by hops and limit. Use to trace a value's produce→store→consume "+
				"lifecycle one hop at a time.",
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
			"Map a free-text symptom to ranked when→then@at failure-condition candidates "+
				"(branch.when is part of the flow embedding). Use to go from an observed symptom "+
				"to the branch/condition that produces it.",
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
			"Enumerate every site that enforces an invariant (the H-guardrail enabler: a "+
				"code-derived implementation invariant and the places that must uphold it). "+
				"Use to check whether a change preserves an invariant at all enforcement points.",
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
