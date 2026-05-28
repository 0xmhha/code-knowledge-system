package mcp

import (
	"context"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Tool names — exported so callers and tests can reference them
// without string drift.
const (
	ToolNameFindSymbol  = "cks.context.find_symbol"
	ToolNameFindCallers = "cks.context.find_callers"
	ToolNameFindCallees = "cks.context.find_callees"
	ToolNameGetSubgraph = "cks.context.get_subgraph"
)

// findSymbolResponse wraps the CKG citations + any dummy directives the
// backend emitted on the way out.
type findSymbolResponse struct {
	Symbol       string                       `json:"symbol"`
	Citations    []contract.Citation          `json:"citations"`
	Instructions []contract.DummyInstruction  `json:"instructions,omitempty"`
}

// graphNeighborsResponse is the wire shape for find_callers / find_callees.
type graphNeighborsResponse struct {
	Seed         contract.Citation            `json:"seed"`
	Direction    string                       `json:"direction"` // "callers" | "callees"
	Neighbors    []contract.Neighbor          `json:"neighbors"`
	Instructions []contract.DummyInstruction  `json:"instructions,omitempty"`
}

// subgraphResponse is the wire shape for get_subgraph.
type subgraphResponse struct {
	Seed         string                       `json:"seed"`
	Nodes        []contract.Citation          `json:"nodes"`
	Edges        []contract.Neighbor          `json:"edges"`
	Instructions []contract.DummyInstruction  `json:"instructions,omitempty"`
}

// registerFindSymbol wires cks.context.find_symbol.
func registerFindSymbol(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameFindSymbol,
		mcpgo.WithDescription(
			"Resolve a symbol name to its definition citations via ckg's qualified-name index. "+
				"Suffix match by default; pass the fully-qualified name for an exact hit.",
		),
		mcpgo.WithString("name", mcpgo.Required(),
			mcpgo.Description("Symbol name (e.g., \"ProcessRequest\" or \"pkg.Type.Method\").")),
		mcpgo.WithString("language",
			mcpgo.Description("Restrict to a language (e.g., \"go\").")),
		mcpgo.WithString("kinds",
			mcpgo.Description("Comma-separated symbol kinds (e.g., \"function,method\").")),
		mcpgo.WithString("path_glob",
			mcpgo.Description("Restrict to file paths matching this glob.")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleFindSymbol(ctx, d, req)
	})
}

func handleFindSymbol(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	name := req.GetString("name", "")
	if name == "" {
		return mcpgo.NewToolResultError(ToolNameFindSymbol + ": missing required argument \"name\""), nil
	}
	opts := ckgclient.SymbolOpts{
		PathGlob: req.GetString("path_glob", ""),
	}
	if kinds := req.GetString("kinds", ""); kinds != "" {
		opts.Kinds = splitCSV(kinds)
	}

	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	citations, err := d.CKG.FindSymbol(ctx, name, opts)
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: %v", ToolNameFindSymbol, err), nil
	}
	return mcpgo.NewToolResultStructured(findSymbolResponse{
		Symbol:       name,
		Citations:    citations,
		Instructions: collector.All(),
	}, "find_symbol result"), nil
}

// registerFindCallers wires cks.context.find_callers.
func registerFindCallers(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameFindCallers,
		mcpgo.WithDescription(
			"List code that calls the given symbol (reverse call graph). Walks ckg's "+
				"calls/invokes edges up to the requested depth.",
		),
		mcpgo.WithString("symbol", mcpgo.Required(),
			mcpgo.Description("Fully-qualified symbol name (e.g., \"consensus.wbft.Finalize\").")),
		mcpgo.WithNumber("depth",
			mcpgo.Description("Maximum traversal depth (default 1).")),
		mcpgo.WithNumber("max_total",
			mcpgo.Description("Cap on total neighbours (0 = no cap).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleFindRelatives(ctx, d, req, ToolNameFindCallers, "callers", []contract.Relation{contract.RelationCalledBy})
	})
}

// registerFindCallees wires cks.context.find_callees.
func registerFindCallees(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameFindCallees,
		mcpgo.WithDescription(
			"List code called by the given symbol (forward call graph). Walks ckg's "+
				"calls/invokes edges up to the requested depth.",
		),
		mcpgo.WithString("symbol", mcpgo.Required(),
			mcpgo.Description("Fully-qualified symbol name (e.g., \"consensus.wbft.Finalize\").")),
		mcpgo.WithNumber("depth",
			mcpgo.Description("Maximum traversal depth (default 1).")),
		mcpgo.WithNumber("max_total",
			mcpgo.Description("Cap on total neighbours (0 = no cap).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleFindRelatives(ctx, d, req, ToolNameFindCallees, "callees", []contract.Relation{contract.RelationCalls})
	})
}

// handleFindRelatives is the shared body for find_callers / find_callees.
// The differences are the displayed direction string and which Relation
// drives the ckg traversal.
func handleFindRelatives(
	ctx context.Context,
	d Deps,
	req mcpgo.CallToolRequest,
	toolName, direction string,
	relations []contract.Relation,
) (*mcpgo.CallToolResult, error) {
	symbol := req.GetString("symbol", "")
	if symbol == "" {
		return mcpgo.NewToolResultError(toolName + ": missing required argument \"symbol\""), nil
	}

	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	// First, resolve the symbol to a citation. ckg.Neighbors takes a
	// Citation, not a qname, so we trampoline through FindSymbol.
	cits, err := d.CKG.FindSymbol(ctx, symbol, ckgclient.SymbolOpts{})
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: resolve symbol: %v", toolName, err), nil
	}
	if len(cits) == 0 {
		return mcpgo.NewToolResultStructured(graphNeighborsResponse{
			Seed:         contract.Citation{File: symbol},
			Direction:    direction,
			Instructions: collector.All(),
		}, toolName+" result"), nil
	}

	opts := ckgclient.NeighborsOpts{
		Relations: relations,
		Hops:      intArg(req, "depth", 1),
		MaxTotal:  intArg(req, "max_total", 0),
	}
	neighbors, err := d.CKG.Neighbors(ctx, cits[0], opts)
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: %v", toolName, err), nil
	}
	return mcpgo.NewToolResultStructured(graphNeighborsResponse{
		Seed:         cits[0],
		Direction:    direction,
		Neighbors:    neighbors,
		Instructions: collector.All(),
	}, toolName+" result"), nil
}

// registerGetSubgraph wires cks.context.get_subgraph.
func registerGetSubgraph(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameGetSubgraph,
		mcpgo.WithDescription(
			"Return every node and edge within depth hops of the seed symbol, traversing all "+
				"edge types (calls, implements, imports, tested_by, …). Use this when find_callers/"+
				"find_callees are too narrow.",
		),
		mcpgo.WithString("symbol", mcpgo.Required(),
			mcpgo.Description("Fully-qualified symbol name to seed the traversal.")),
		mcpgo.WithNumber("depth",
			mcpgo.Description("Maximum traversal depth (default 1).")),
		mcpgo.WithNumber("max_total",
			mcpgo.Description("Cap on total nodes (0 = no cap).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleGetSubgraph(ctx, d, req)
	})
}

func handleGetSubgraph(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	symbol := req.GetString("symbol", "")
	if symbol == "" {
		return mcpgo.NewToolResultError(ToolNameGetSubgraph + ": missing required argument \"symbol\""), nil
	}
	opts := ckgclient.SubgraphOpts{
		Depth:    intArg(req, "depth", 1),
		MaxTotal: intArg(req, "max_total", 0),
	}

	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	nodes, edges, err := d.CKG.GetSubgraph(ctx, symbol, opts)
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: %v", ToolNameGetSubgraph, err), nil
	}
	return mcpgo.NewToolResultStructured(subgraphResponse{
		Seed:         symbol,
		Nodes:        nodes,
		Edges:        edges,
		Instructions: collector.All(),
	}, "subgraph result"), nil
}

// splitCSV splits a comma-separated string, trimming whitespace and dropping empties.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// intArg pulls an integer-shaped argument from the request, defaulting
// when missing. mcp-go surfaces JSON numbers as float64 by default.
func intArg(req mcpgo.CallToolRequest, key string, dflt int) int {
	v := req.GetFloat(key, float64(dflt))
	return int(v)
}

