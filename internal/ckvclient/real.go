package ckvclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	mcpgoclient "github.com/mark3labs/mcp-go/client"
	mcpgotransport "github.com/mark3labs/mcp-go/client/transport"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Tool names on ckv's MCP surface (see ckv/pkg/mcp/server.go).
const (
	toolSemanticSearch = "cks.context.semantic_search"
	toolHealth         = "cks.ops.health"
)

// mcpClient is the cks-internal seam over the upstream mcp-go *client.Client.
// Production code injects mcpgoclient.NewClient; tests inject a mockMCPClient
// so we never have to spawn a real ckv subprocess in unit tests.
//
// The signatures intentionally mirror mcpgoclient.Client so the production
// type satisfies this interface by duck typing.
type mcpClient interface {
	Initialize(ctx context.Context, req mcpgo.InitializeRequest) (*mcpgo.InitializeResult, error)
	CallTool(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	Close() error
}

// RealOpts configures NewReal.
//
// The ckv module currently exposes no Go-level constructor for its query.Engine
// (the relevant Open functions live in internal/), so the Real adapter spawns
// the ckv binary in `mcp` mode and proxies cks API calls through MCP stdio.
// Once ckv exposes a public Open, this adapter can be reimplemented to skip
// the subprocess hop without changing the cks-side API.
type RealOpts struct {
	// BinaryPath is the absolute path to the ckv binary. Empty means look
	// up "ckv" on $PATH.
	BinaryPath string

	// DataPath is forwarded as --out=<path> to `ckv mcp`. This is the
	// ckv-data directory produced by `ckv build`.
	DataPath string

	// Embedder forwards as the global `--embedder=<name>` flag. Empty
	// uses ckv's own default (mock).
	Embedder string

	// ModelDir forwards as the global `--model-dir=<path>` flag when set.
	// Required by the bgeonnx embedder.
	ModelDir string

	// Env extends the subprocess environment. The parent process's
	// PATH is inherited automatically; Env supplements with extra
	// variables (e.g. logging knobs).
	Env []string
}

// Real is the ckv Client adapter that proxies calls through ckv's MCP
// stdio surface. NewReal spawns the ckv subprocess and performs the MCP
// initialize handshake; Close shuts it down.
//
// Concurrency: a single Real instance must not be called from multiple
// goroutines without external serialization — the underlying mcp-go
// client serializes JSON-RPC ids but the subprocess processes calls
// sequentially anyway. cks composer stages today are sequential per
// request so this is not a constraint in practice.
type Real struct {
	client mcpClient
	closed bool
}

// Compile-time guarantees:
//   - Real satisfies the cks Client contract.
//   - mcp-go's production *client.Client satisfies our mcpClient seam,
//     so signature drift in mcp-go (e.g. Initialize/CallTool changes)
//     fails the build rather than the first subprocess call.
var (
	_ Client    = (*Real)(nil)
	_ mcpClient = (*mcpgoclient.Client)(nil)
)

// NewReal spawns `ckv mcp --out=<DataPath>` and returns a connected
// Client. The subprocess inherits stderr from the parent so panics and
// log output land visibly; stdout is reserved for JSON-RPC frames.
func NewReal(ctx context.Context, opts RealOpts) (*Real, error) {
	if opts.DataPath == "" {
		return nil, errors.New("ckvclient: empty DataPath")
	}
	bin := opts.BinaryPath
	if bin == "" {
		bin = "ckv"
	}
	args := make([]string, 0, 5)
	if opts.Embedder != "" {
		args = append(args, "--embedder="+opts.Embedder)
	}
	if opts.ModelDir != "" {
		args = append(args, "--model-dir="+opts.ModelDir)
	}
	args = append(args, "mcp", "--out="+opts.DataPath)

	tp := mcpgotransport.NewStdio(bin, opts.Env, args...)
	c := mcpgoclient.NewClient(tp)
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("ckvclient: start subprocess: %w", err)
	}
	return newRealWithClient(ctx, c)
}

// newRealWithClient is the test seam. Performs the initialize handshake
// and returns a Real wrapping c. On handshake failure the client is
// closed so callers don't have to worry about leaked subprocesses.
func newRealWithClient(ctx context.Context, c mcpClient) (*Real, error) {
	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpgo.Implementation{
		Name:    "cks-mcp",
		Version: "0.0.1",
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("ckvclient: initialize: %w", err)
	}
	return &Real{client: c}, nil
}

// SemanticSearch sends a cks.context.semantic_search tool call.
//
// Filter mapping:
//   - opts.K → "k"
//   - opts.Filter.Language → "language"
//   - opts.Filter.PathGlob → "path"
//   - opts.Filter.SymbolKinds[0] → "symbol_kind"  (ckv's MCP takes one
//     kind per call; the slice is collapsed to its first entry. C.2
//     accepts this rather than expanding to multiple round-trips.)
//   - opts.Filter.CommitHash → not forwarded (no MCP argument).
func (r *Real) SemanticSearch(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	if query == "" {
		return nil, errors.New("ckvclient: empty query")
	}
	args := map[string]any{"intent": query}
	if opts.K > 0 {
		args["k"] = opts.K
	}
	if opts.Filter.Language != "" {
		args["language"] = opts.Filter.Language
	}
	if opts.Filter.PathGlob != "" {
		args["path"] = opts.Filter.PathGlob
	}
	if len(opts.Filter.SymbolKinds) > 0 {
		args["symbol_kind"] = opts.Filter.SymbolKinds[0]
	}

	req := mcpgo.CallToolRequest{}
	req.Params.Name = toolSemanticSearch
	req.Params.Arguments = args

	res, err := r.client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ckvclient: CallTool: %w", err)
	}
	if res != nil && res.IsError {
		return nil, fmt.Errorf("ckvclient: tool error: %s", resultText(res))
	}
	return parseSemanticSearchResult(res)
}

// Health calls the ckv health tool and translates the manifest payload
// into ckvclient.Health. On any failure (transport, tool error, decode)
// Reachable is false and the error is wrapped.
func (r *Real) Health(ctx context.Context) (Health, error) {
	req := mcpgo.CallToolRequest{}
	req.Params.Name = toolHealth

	res, err := r.client.CallTool(ctx, req)
	if err != nil {
		return Health{Reachable: false}, fmt.Errorf("ckvclient: CallTool: %w", err)
	}
	if res != nil && res.IsError {
		return Health{Reachable: false}, fmt.Errorf("ckvclient: health: %s", resultText(res))
	}
	return parseHealthResult(res)
}

// Close shuts down the underlying mcp-go client; for the production
// transport (Stdio) this terminates the ckv subprocess. Idempotent.
func (r *Real) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.client.Close()
}

// --- Wire payload mirrors ---
//
// The ckv MCP server encodes its results as JSON inside a single
// `content[0].text` field (NewToolResultText). The structs below mirror
// ckv/internal/query.{Response,Hit} so we can decode that JSON without
// importing ckv's internal package.

type queryHitWire struct {
	ChunkID    string             `json:"chunk_id"`
	Citation   citationWire       `json:"citation"`
	Snippet    string             `json:"snippet"`
	Score      hitScoreWire       `json:"score"`
	Language   string             `json:"language"`
	Symbol     string             `json:"symbol,omitempty"`
	SymbolKind string             `json:"symbol_kind,omitempty"`
	CKGNodeID  string             `json:"ckg_node_id,omitempty"`
}

type citationWire struct {
	File       string `json:"file"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	CommitHash string `json:"commit_hash"`
}

type hitScoreWire struct {
	Normalized     float64 `json:"normalized"`
	VectorDistance float64 `json:"vector_distance"`
	VectorRank     int     `json:"vector_rank"`
}

type queryResponseWire struct {
	Hits     []queryHitWire `json:"hits"`
	Warnings []string       `json:"warnings,omitempty"`
}

type healthWire struct {
	Server         string `json:"server"`
	ServerVersion  string `json:"server_version"`
	EmbeddingModel string `json:"embedding_model"`
	EmbeddingDim   int    `json:"embedding_dim"`
	IndexedHead    string `json:"indexed_head"`
	ChunkCount     int    `json:"chunk_count"`
	BuiltAt        string `json:"built_at"`
	SrcRoot        string `json:"src_root"`
}

// --- Translation helpers ---

func parseSemanticSearchResult(res *mcpgo.CallToolResult) ([]contract.Hit, error) {
	txt := firstText(res)
	if txt == "" {
		return nil, errors.New("ckvclient: empty semantic_search result")
	}
	var wire queryResponseWire
	if err := json.Unmarshal([]byte(txt), &wire); err != nil {
		return nil, fmt.Errorf("ckvclient: decode result: %w", err)
	}
	hits := make([]contract.Hit, 0, len(wire.Hits))
	for i, h := range wire.Hits {
		rank := h.Score.VectorRank
		if rank <= 0 {
			rank = i + 1
		}
		hits = append(hits, contract.Hit{
			Citation: contract.Citation{
				File:       h.Citation.File,
				StartLine:  h.Citation.StartLine,
				EndLine:    h.Citation.EndLine,
				CommitHash: h.Citation.CommitHash,
			},
			Rank:   rank,
			Score:  h.Score.Normalized,
			Source: contract.HitSourceCKV,
		})
	}
	return hits, nil
}

func parseHealthResult(res *mcpgo.CallToolResult) (Health, error) {
	txt := firstText(res)
	if txt == "" {
		return Health{Reachable: false}, errors.New("ckvclient: empty health result")
	}
	var w healthWire
	if err := json.Unmarshal([]byte(txt), &w); err != nil {
		return Health{Reachable: false}, fmt.Errorf("ckvclient: decode health: %w", err)
	}
	// StatsHash isn't in ckv's MCP health payload directly. Synthesize as
	// "<model>@<indexed_head>": changes when the snapshot changes, and
	// names which embedder produced it so operators can correlate index
	// version with embedder version.
	statsHash := w.EmbeddingModel
	if w.IndexedHead != "" {
		if statsHash == "" {
			statsHash = w.IndexedHead
		} else {
			statsHash = w.EmbeddingModel + "@" + w.IndexedHead
		}
	}
	var lastIndex time.Time
	if w.BuiltAt != "" {
		if t, err := time.Parse(time.RFC3339, w.BuiltAt); err == nil {
			lastIndex = t.UTC()
		}
	}
	return Health{
		Reachable:   true,
		StatsHash:   statsHash,
		LastIndexAt: lastIndex,
	}, nil
}

// firstText returns the text of the first TextContent block, or "".
func firstText(res *mcpgo.CallToolResult) string {
	if res == nil {
		return ""
	}
	for _, c := range res.Content {
		if tc, ok := c.(mcpgo.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// resultText concatenates all TextContent in res — used when surfacing
// tool error messages back to callers as the wrapped err string.
func resultText(res *mcpgo.CallToolResult) string {
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
