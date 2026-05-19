package ckvclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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

	// CallTimeout bounds each tool call. When zero, defaults to
	// DefaultCallTimeout — see below for rationale. Set explicitly to
	// a smaller value when the ckv backend is known to be fast, or
	// larger when running against a slow ONNX embedder. Hangs are the
	// primary defense this provides; transport-closed errors already
	// trigger a restart on their own.
	CallTimeout time.Duration
}

// DefaultCallTimeout caps each cks.context.semantic_search / cks.ops.health
// call when RealOpts.CallTimeout is unset. Dogfood with the mock embedder
// completes in ~250ms per scenario, so 10s leaves ample headroom while
// preventing a hung ckv subprocess from blocking the entire eval run.
// Real bgeonnx loads on first call can take 1-3s, so this cap also
// keeps room for that without falsely tripping.
const DefaultCallTimeout = 10 * time.Second

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
	mu     sync.Mutex
	client mcpClient
	// spawn rebuilds the underlying mcpClient when transport breaks.
	// NewReal supplies a closure that captures RealOpts so a restart
	// hits the exact same subprocess configuration. The Test seam
	// (newRealWithClient) installs a no-op spawn — restart there is
	// a programming error.
	spawn       clientSpawnFunc
	callTimeout time.Duration
	closed      bool
}

// clientSpawnFunc creates and initializes a new mcpClient. Returned
// client must have already completed the MCP initialize handshake so
// callers can use it immediately. Used by NewReal for the first spawn
// and by Real.restartLocked when a transport closes mid-session.
type clientSpawnFunc func(ctx context.Context) (mcpClient, error)

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
//
// The opts are captured for re-use: when the subprocess transport
// closes mid-session, Real spawns a fresh one with the same
// configuration before failing the call back to the caller.
func NewReal(ctx context.Context, opts RealOpts) (*Real, error) {
	if opts.DataPath == "" {
		return nil, errors.New("ckvclient: empty DataPath")
	}
	timeout := opts.CallTimeout
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	spawn := func(ctx context.Context) (mcpClient, error) {
		return spawnAndInitialize(ctx, opts)
	}
	r, err := newRealWithSpawn(ctx, spawn)
	if err != nil {
		return nil, err
	}
	r.callTimeout = timeout
	return r, nil
}

// spawnAndInitialize builds an mcp-go stdio client per opts and runs
// the initialize handshake. Extracted so NewReal and the restart path
// share one definition; an opts change picks both up automatically.
func spawnAndInitialize(ctx context.Context, opts RealOpts) (mcpClient, error) {
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
	if err := initializeClient(ctx, c); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// initializeClient runs the MCP initialize handshake on c. Shared
// between the first-spawn path and the restart path.
func initializeClient(ctx context.Context, c mcpClient) error {
	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpgo.Implementation{
		Name:    "cks-mcp",
		Version: "0.0.1",
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		return fmt.Errorf("ckvclient: initialize: %w", err)
	}
	return nil
}

// newRealWithSpawn wires a spawn closure as the client factory. The
// production path (NewReal) uses spawnAndInitialize; restart-aware
// tests inject a sequence-driven mock factory.
func newRealWithSpawn(ctx context.Context, spawn clientSpawnFunc) (*Real, error) {
	c, err := spawn(ctx)
	if err != nil {
		return nil, err
	}
	return &Real{client: c, spawn: spawn}, nil
}

// newRealWithClient is the legacy test seam — a single in-memory mock
// client that has already gone through Initialize is installed and a
// no-op spawn is wired so a restart fails loudly (the test that needs
// restart should use newRealWithSpawn). Existing test suites use this
// shape; keeping it stable saves a churny rewrite.
func newRealWithClient(ctx context.Context, c mcpClient) (*Real, error) {
	if err := initializeClient(ctx, c); err != nil {
		_ = c.Close()
		return nil, err
	}
	spawn := func(_ context.Context) (mcpClient, error) {
		return nil, errors.New("ckvclient: restart not configured (newRealWithClient test seam)")
	}
	return &Real{client: c, spawn: spawn}, nil
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

	res, err := r.callToolWithRestart(ctx, req)
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

	res, err := r.callToolWithRestart(ctx, req)
	if err != nil {
		return Health{Reachable: false}, fmt.Errorf("ckvclient: CallTool: %w", err)
	}
	if res != nil && res.IsError {
		return Health{Reachable: false}, fmt.Errorf("ckvclient: health: %s", resultText(res))
	}
	return parseHealthResult(res)
}

// callToolWithRestart wraps mcpClient.CallTool with one
// transport-error retry. On the first transport-closed signal it tears
// down the broken client, spawns a fresh subprocess via r.spawn, and
// replays the same request once. Two failures in a row return the
// underlying error to the caller. Context cancellations are NOT
// retried — they indicate the caller wants to abort.
//
// Lock discipline: the mu protects swapping r.client during restart.
// CallTool itself can block on stdio I/O for seconds, so the lock is
// dropped around the network call and reacquired only when mutating
// r.client. Concurrent callers therefore see consistent client state
// at the moment of their own call — adequate for cks's single-pipeline
// composer; a stronger guarantee would require request-level
// serialization which the underlying mcp-go client already provides.
func (r *Real) callToolWithRestart(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	c, err := r.activeClient()
	if err != nil {
		return nil, err
	}
	res, err := r.callToolTimeBounded(ctx, c, req)
	if err == nil || !isTransportClosed(err) {
		return res, err
	}
	// Transport closed mid-call. Try to recover; one retry attempt.
	if rerr := r.restart(ctx); rerr != nil {
		return nil, fmt.Errorf("transport closed; restart failed: %w (original: %v)", rerr, err)
	}
	c2, err2 := r.activeClient()
	if err2 != nil {
		return nil, err2
	}
	return r.callToolTimeBounded(ctx, c2, req)
}

// callToolTimeBounded invokes c.CallTool under a derived context that
// expires after r.callTimeout. Without the bound, a ckv subprocess that
// has the transport alive but stops responding (mcp-go awaiting an
// id-keyed reply) would hang cks-mcp forever. The timeout lets the
// caller see context.DeadlineExceeded and decide whether to retry.
// Transport-closed errors still bypass this branch and trigger
// restart-and-retry, since those have a clear corrective action.
func (r *Real) callToolTimeBounded(ctx context.Context, c mcpClient, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r.callTimeout <= 0 {
		return c.CallTool(ctx, req)
	}
	cctx, cancel := context.WithTimeout(ctx, r.callTimeout)
	defer cancel()
	return c.CallTool(cctx, req)
}

// activeClient returns the current client under lock. Returns an error
// when Close has already been called — callers must not see a nil
// pointer through the seam.
func (r *Real) activeClient() (mcpClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("ckvclient: closed")
	}
	if r.client == nil {
		return nil, errors.New("ckvclient: no active client")
	}
	return r.client, nil
}

// restart closes the existing (broken) client and installs a freshly
// spawned one. Holds the lock for the entire swap so a concurrent
// caller cannot observe the half-constructed state.
func (r *Real) restart(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("ckvclient: closed")
	}
	if r.client != nil {
		// Best-effort close on the broken transport.
		_ = r.client.Close()
		r.client = nil
	}
	c, err := r.spawn(ctx)
	if err != nil {
		return err
	}
	r.client = c
	return nil
}

// isTransportClosed reports whether err signals a broken stdio
// transport (subprocess died, pipe closed, etc.). Recognizes both the
// mcp-go sentinel and the textual marker — wrappers downstream of
// mcp-go can lose the sentinel via fmt.Errorf without %w.
func isTransportClosed(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, mcpgotransport.ErrTransportClosed) {
		return true
	}
	return strings.Contains(err.Error(), "transport closed")
}

// Close shuts down the underlying mcp-go client; for the production
// transport (Stdio) this terminates the ckv subprocess. Idempotent.
// Holding the lock prevents a racing restart from re-spawning a
// client mid-shutdown.
func (r *Real) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.client == nil {
		return nil
	}
	err := r.client.Close()
	r.client = nil
	return err
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
