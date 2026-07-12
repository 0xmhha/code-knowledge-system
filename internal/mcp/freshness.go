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
			"Compare the index snapshot against the source repository: indexed commit vs "+
				"HEAD and the changed-file list. Call at session start and after "+
				"pulls/commits -- stale files mean citations may mislead. If stale, run "+
				"cks.ops.index (files you created this session are never in the index; read "+
				"them directly).",
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
