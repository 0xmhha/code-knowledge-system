package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

const ToolNameFreshness = "cks.ops.freshness"

// freshnessResponse is the wire shape for cks.ops.freshness. It mirrors
// ckvclient.FreshnessReport but lives in the mcp package so the wire
// format is independent of the backend client struct evolving.
type freshnessResponse struct {
	Fresh        bool                        `json:"fresh"`
	IndexedHead  string                      `json:"indexed_head,omitempty"`
	CurrentHead  string                      `json:"current_head,omitempty"`
	ChangedFiles []string                    `json:"changed_files,omitempty"`
	Instructions []contract.DummyInstruction `json:"instructions,omitempty"`
}

// registerFreshness wires cks.ops.freshness.
func registerFreshness(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameFreshness,
		mcpgo.WithDescription(
			"Report whether the ckv index is up-to-date with the source repository. Returns "+
				"the indexed git commit, the current git HEAD, and the list of files changed "+
				"between them. Use this before relying on retrieval results to detect a stale index.",
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleFreshness(ctx, d, req)
	})
}

func handleFreshness(ctx context.Context, d Deps, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	report, err := d.CKV.Freshness(ctx)
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: %v", ToolNameFreshness, err), nil
	}
	return mcpgo.NewToolResultStructured(freshnessResponse{
		Fresh:        report.Fresh,
		IndexedHead:  report.IndexedHead,
		CurrentHead:  report.CurrentHead,
		ChangedFiles: report.ChangedFiles,
		Instructions: collector.All(),
	}, "freshness report"), nil
}
