package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

const ToolNameConcurrencyImpact = "cks.context.concurrency_impact"

// concurrencyImpactResponse is the wire shape for concurrency_impact.
type concurrencyImpactResponse struct {
	Seed         string                      `json:"seed"`
	Result       contract.ConcurrencyResult  `json:"result"`
	Instructions []contract.DummyInstruction `json:"instructions,omitempty"`
}

// registerConcurrencyImpact wires cks.context.concurrency_impact (G7/S1).
func registerConcurrencyImpact(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameConcurrencyImpact,
		mcpgo.WithDescription(
			"Compute the concurrency blast radius of a symbol: the goroutines, channels, and locks "+
				"it spawns, sends to, or acquires, plus the modules reached over concurrency edges "+
				"(spawns, sends_to, recvs_from, acquires_lock, accessed_under_lock) in either "+
				"direction. Use during PLANNING to answer \"what concurrency state does changing this "+
				"touch, and what races could it introduce?\".",
		),
		mcpgo.WithString("symbol", mcpgo.Required(),
			mcpgo.Description("Fully-qualified symbol name to seed the concurrency analysis.")),
		mcpgo.WithNumber("depth",
			mcpgo.Description("Maximum traversal depth over concurrency edges (default: 3).")),
		mcpgo.WithNumber("max_total",
			mcpgo.Description("Cap on total modules returned (0 = no cap).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleConcurrencyImpact(ctx, d, req)
	})
}

func handleConcurrencyImpact(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	symbol := req.GetString("symbol", "")
	if symbol == "" {
		return mcpgo.NewToolResultError(ToolNameConcurrencyImpact + ": missing required argument \"symbol\""), nil
	}
	opts := ckgclient.ConcurrencyOpts{
		Depth:    intArg(req, "depth", 0),
		MaxTotal: intArg(req, "max_total", 0),
	}

	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	result, err := d.CKG.ConcurrencyImpact(ctx, symbol, opts)
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: %v", ToolNameConcurrencyImpact, err), nil
	}
	return mcpgo.NewToolResultStructured(concurrencyImpactResponse{
		Seed:         symbol,
		Result:       result,
		Instructions: collector.All(),
	}, "concurrency impact result"), nil
}
