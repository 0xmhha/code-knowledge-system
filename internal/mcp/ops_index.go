package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/domainexport"
	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

const ToolNameOpsIndex = "cks.ops.index"

// IndexConfig carries the paths the cks.ops.index tool shells out to. The
// query path is fully in-process (G1), but the index/reindex maintenance op
// has no in-process ckv/ckg API, so it runs the CLIs as a subprocess — an
// explicit, infrequent, agent-triggered call, NOT the hot retrieval loop
// (00 §C3, plan G8). Empty binaries disable the tool (it returns an error
// telling the agent to run the indexers manually).
type IndexConfig struct {
	CKVBinary     string // ckv binary path; "" disables ckv reindex
	CKGBinary     string // ckg binary path; "" disables ckg build
	CKVDataPath   string // ckv --out
	CKGDataPath   string // ckg --out
	SourceRoot    string // --src for both
	EmbedModel    string // ckv --model-name (paired with --embedder=ollama)
	OllamaURL     string // CKV_OLLAMA_ENDPOINT for the ckv subprocess (optional)
	CKGPolicyFile string // ckg --policy-file (governed_by edges); "" omits the flag
	// Channel ②: DomainProjectDir is the cks domain-knowledge project dir
	// to export before building; DomainCorpusDir is the export output AND
	// the ckv --docs root. Both empty disables channel ② (no corpus step).
	DomainProjectDir string
	DomainCorpusDir  string
}

func (c IndexConfig) enabled() bool { return c.CKVBinary != "" || c.CKGBinary != "" }

// indexRunner runs one index subcommand to completion. Overridable in tests
// so the orchestration (both sub-actions, failure handling) is exercised
// without real ckv/ckg binaries.
var indexRunner = func(ctx context.Context, name string, args, env []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = env
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w (%s)", name, args, err, truncOneLineIdx(string(out), 500))
	}
	return nil
}

// indexSubResult is the per-backend outcome in the ops.index response.
type indexSubResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type opsIndexResponse struct {
	Mode string         `json:"mode"`
	CKV  indexSubResult `json:"ckv"`
	CKG  indexSubResult `json:"ckg"`
}

// registerOpsIndex wires cks.ops.index (G8/S2).
func registerOpsIndex(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameOpsIndex,
		mcpgo.WithDescription(
			"Refresh the ckv (vector) and ckg (graph) indexes for the configured source "+
				"tree. Use ONLY after cks.ops.freshness reports stale. mode=incremental "+
				"reindexes files changed since the indexed commit; mode=full rebuilds "+
				"everything (slow).",
		),
		mcpgo.WithString("mode",
			mcpgo.Description("\"incremental\" (default) or \"full\".")),
		mcpgo.WithString("since_commit",
			mcpgo.Description("Base commit for an incremental reindex (default: the index's recorded head).")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleOpsIndex(ctx, d, req)
	})
}

func handleOpsIndex(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	mode := req.GetString("mode", "incremental")
	if mode != "incremental" && mode != "full" {
		return mcpgo.NewToolResultErrorf("%s: mode must be \"incremental\" or \"full\", got %q", ToolNameOpsIndex, mode), nil
	}
	since := req.GetString("since_commit", "")

	ic := d.Index
	if !ic.enabled() {
		return mcpgo.NewToolResultErrorf(
			"%s: no index binaries configured (backends.ckv.binary_path / backends.ckg.binary_path); "+
				"run `ckv %s` + `ckg build` against the source tree manually",
			ToolNameOpsIndex, map[string]string{"full": "build", "incremental": "reindex"}[mode],
		), nil
	}

	resp := opsIndexResponse{Mode: mode}

	// Channel ②: regenerate the corpus so the ckv build below (--docs)
	// embeds the latest entries + authoritative docs. Only runs on full
	// builds — incremental/reindex does not pass --docs to ckv, so the
	// export is wasted work and can block a cheap reindex. Disabled when
	// the project dir is unset.
	if mode == "full" && ic.DomainProjectDir != "" && ic.DomainCorpusDir != "" {
		proj, err := inventory.LoadProject(ic.DomainProjectDir)
		if err != nil {
			resp.CKV.Error = fmt.Sprintf("domain export: load project: %v", err)
			return mcpgo.NewToolResultStructured(resp, "index refresh FAILED (domain export)"), nil
		}
		if _, err := domainexport.Export(proj, ic.DomainCorpusDir); err != nil {
			resp.CKV.Error = fmt.Sprintf("domain export: %v", err)
			return mcpgo.NewToolResultStructured(resp, "index refresh FAILED (domain export)"), nil
		}
	}

	if ic.CKVBinary != "" {
		args := ckvIndexArgs(ic, mode, since)
		var env []string
		if ic.OllamaURL != "" {
			env = append(env, "CKV_OLLAMA_ENDPOINT="+ic.OllamaURL)
		}
		if err := indexRunner(ctx, ic.CKVBinary, args, env); err != nil {
			resp.CKV.Error = err.Error()
		} else {
			resp.CKV.OK = true
		}
	}

	if ic.CKGBinary != "" {
		// ckg has no incremental path (cold rebuild); --src + --out always.
		// --policy-file (when configured) rebuilds governed_by edges with the
		// index so impact_analysis/get_for_task can surface policy nodes.
		//
		// ckg build --out wants the output *directory* (it mkdirs <out> and
		// writes graph.db inside). CKGDataPath is the graph.db file (the query
		// path opens it directly), so pass its parent dir. --force lets the
		// refresh overwrite the existing graph.db (ckg has no incremental path,
		// so ops.index always triggers a cold rebuild).
		ckgOut := filepath.Dir(ic.CKGDataPath)
		args := []string{"build", "--src", ic.SourceRoot, "--out", ckgOut, "--force"}
		if ic.CKGPolicyFile != "" {
			args = append(args, "--policy-file", ic.CKGPolicyFile)
		}
		if err := indexRunner(ctx, ic.CKGBinary, args, nil); err != nil {
			resp.CKG.Error = err.Error()
		} else {
			resp.CKG.OK = true
		}
	}

	// Per-backend OK/Error fields convey partial failure; the agent reads
	// resp.CKV.OK / resp.CKG.OK to decide whether a retry or manual run is
	// needed. A single rolled-up error string keeps the text fallback useful.
	text := "index refresh result"
	if (ic.CKVBinary != "" && !resp.CKV.OK) || (ic.CKGBinary != "" && !resp.CKG.OK) {
		text = fmt.Sprintf("index refresh FAILED (ckv ok=%v, ckg ok=%v)", resp.CKV.OK, resp.CKG.OK)
	}
	return mcpgo.NewToolResultStructured(resp, text), nil
}

// ckvIndexArgs builds the ckv subcommand. Reindex reuses the manifest's
// src_root; full build needs --src. Both forward the Ollama embedder so the
// re-embedded chunks land in the same model space as the query path.
func ckvIndexArgs(ic IndexConfig, mode, since string) []string {
	embed := []string{"--embedder=ollama"}
	if ic.EmbedModel != "" {
		embed = append(embed, "--model-name="+ic.EmbedModel)
	}
	if mode == "full" {
		args := []string{"build", "--src", ic.SourceRoot, "--out", ic.CKVDataPath}
		if ic.DomainCorpusDir != "" {
			args = append(args, "--docs", ic.DomainCorpusDir)
		}
		return append(args, embed...)
	}
	// ckv reindex resolves the source tree from --src (default "."), NOT from
	// the index manifest, and runs git there to diff against the indexed head.
	// Without --src it defaults to the cks-mcp process cwd (not the repo), which
	// has no git HEAD. Pass the configured source root explicitly.
	args := []string{"reindex", "--src", ic.SourceRoot, "--out", ic.CKVDataPath}
	if since != "" {
		args = append(args, "--since", since)
	}
	return append(args, embed...)
}

func truncOneLineIdx(s string, n int) string {
	if len(s) > n {
		s = s[:n] + "…"
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' {
			r = ' '
		}
		out = append(out, r)
	}
	return string(out)
}
