package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

const (
	ToolNameImpactAnalysis = "cks.context.impact_analysis"
	ToolNameChangeHistory  = "cks.context.change_history"
)

// impactAnalysisResponse is the wire shape for impact_analysis.
type impactAnalysisResponse struct {
	Seed         string                      `json:"seed"`
	Result       contract.ImpactResult       `json:"result"`
	Instructions []contract.DummyInstruction `json:"instructions,omitempty"`
}

// changeHistoryResponse is the wire shape for change_history.
// PRs and Hunks are reported separately because callers typically want
// either provenance (PRs) or diff evidence (Hunks), not always both.
type changeHistoryResponse struct {
	Seed         string                      `json:"seed"`
	PRs          []contract.PRRef            `json:"prs,omitempty"`
	Hunks        []contract.HunkEvidence     `json:"hunks,omitempty"`
	Instructions []contract.DummyInstruction `json:"instructions,omitempty"`
}

// registerImpactAnalysis wires cks.context.impact_analysis.
func registerImpactAnalysis(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameImpactAnalysis,
		mcpgo.WithDescription(
			"Reverse-dependency closure of a symbol, grouped by coupling category "+
				"(callers, interface, type_users, distributed, concurrent). Use during "+
				"PLANNING to answer 'what breaks if I change this?' and to find co-consumers "+
				"of a shared dependency before editing it.",
		),
		mcpgo.WithString("symbol", mcpgo.Required(),
			mcpgo.Description("Fully-qualified symbol name to seed the impact analysis.")),
		mcpgo.WithNumber("depth",
			mcpgo.Description("Maximum traversal depth (default: backend default, typically 2).")),
		mcpgo.WithNumber("max_total",
			mcpgo.Description("Cap on total citations across all categories (0 = no cap).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleImpactAnalysis(ctx, d, req)
	})
}

func handleImpactAnalysis(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	symbol := req.GetString("symbol", "")
	if symbol == "" {
		return mcpgo.NewToolResultError(ToolNameImpactAnalysis + ": missing required argument \"symbol\""), nil
	}
	opts := ckgclient.ImpactOpts{
		Depth:    intArg(req, "depth", 0),
		MaxTotal: intArg(req, "max_total", 0),
	}

	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	result, err := d.CKG.ImpactOfChange(ctx, symbol, opts)
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: %v", ToolNameImpactAnalysis, err), nil
	}
	return mcpgo.NewToolResultStructured(impactAnalysisResponse{
		Seed:         symbol,
		Result:       result,
		Instructions: collector.All(),
	}, "impact analysis result"), nil
}

// registerChangeHistory wires cks.context.change_history.
//
// The handler runs two ckg calls when both intent and seed_qname are
// provided: EvidenceForIntent surfaces BM25-ranked hunks, GetNodePRs
// surfaces the PR breadcrumb metadata. Either input alone runs only the
// corresponding call.
func registerChangeHistory(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameChangeHistory,
		mcpgo.WithDescription(
			"Modification history for a symbol or intent: PR refs (provenance) and "+
				"BM25-ranked diff hunks. Use AFTER traversal or reproduction has implicated a "+
				"site -- anchoring history on an unverified hypothesis amplifies confirmation "+
				"bias (history 'evidence' for the wrong suspect reads convincing).",
		),
		mcpgo.WithString("intent",
			mcpgo.Description("Natural-language query the hunks should match against (optional if symbol is set).")),
		mcpgo.WithString("symbol",
			mcpgo.Description("Fully-qualified symbol name to look up PR refs for (optional if intent is set).")),
		mcpgo.WithNumber("k",
			mcpgo.Description("Maximum hunks to return when intent is set (0 = backend default).")),
		mcpgo.WithNumber("max_count",
			mcpgo.Description("Maximum PR refs to return when symbol is set (0 = no cap).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleChangeHistory(ctx, d, req)
	})
}

func handleChangeHistory(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	intent := req.GetString("intent", "")
	symbol := req.GetString("symbol", "")
	if intent == "" && symbol == "" {
		return mcpgo.NewToolResultError(ToolNameChangeHistory + ": at least one of \"intent\" or \"symbol\" is required"), nil
	}

	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	out := changeHistoryResponse{Seed: firstNonEmpty(symbol, intent)}

	if intent != "" {
		evOpts := ckgclient.EvidenceOpts{
			SeedQname: symbol,
			K:         intArg(req, "k", 0),
		}
		ev, err := d.CKG.EvidenceForIntent(ctx, intent, evOpts)
		if err != nil {
			return mcpgo.NewToolResultErrorf("%s: EvidenceForIntent: %v", ToolNameChangeHistory, err), nil
		}
		out.Hunks = ev.Hunks
		// EvidenceForIntent may also return PR provenance; merge it
		// in here so the response has a single PR list.
		out.PRs = append(out.PRs, ev.PRs...)
	}

	if symbol != "" {
		prOpts := ckgclient.PRRefOpts{
			MaxCount: intArg(req, "max_count", 0),
		}
		prs, err := d.CKG.GetNodePRs(ctx, symbol, prOpts)
		if err != nil {
			return mcpgo.NewToolResultErrorf("%s: GetNodePRs: %v", ToolNameChangeHistory, err), nil
		}
		out.PRs = append(out.PRs, prs...)
	}

	out.Instructions = collector.All()
	return mcpgo.NewToolResultStructured(out, "change history result"), nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
