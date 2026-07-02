// Package mcp exposes the cks composer pipeline as an MCP (Model Context
// Protocol) server over stdio.
//
// Register wires the 13 agent-facing cks.* tools (the C1 surface): the
// context tools (get_for_task, semantic_search, search_text, find_symbol,
// find_callers, find_callees, get_subgraph, impact_analysis,
// concurrency_impact, change_history) and the ops tools (health, freshness,
// index). The exact registered set is pinned against the SSoT fixture by
// schema_golden_test.go (M2.a).
//
// The package is intentionally thin: Register attaches handlers to an
// already-constructed *server.MCPServer so callers retain control over
// the server's name/version and any non-tool capabilities (resources,
// prompts) that future phases may add. Run is a convenience that
// constructs the server and serves stdio in one call.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/composer"
	"github.com/0xmhha/code-knowledge-system/internal/embedder"
	"github.com/0xmhha/code-knowledge-system/internal/vocab"
)

// ToolNameGetForTask is the wire name of the get_for_task tool. Exported
// so callers (and other tests) can reference it without string drift.
const ToolNameGetForTask = "cks.context.get_for_task"

// ToolNameHealth is the wire name of the health tool.
const ToolNameHealth = "cks.ops.health"

// Deps bundles everything an MCP handler needs. Keep this struct small:
// the slim C.5 surface deliberately resists envelope sprawl (HLD §7.5
// envelope/auth fields land with Phase 3, not here).
type Deps struct {
	// Composer drives cks.context.get_for_task. Must be non-nil.
	Composer *composer.Composer

	// CKG and CKV are reported by cks.ops.health. They are NOT used to
	// short-circuit composer calls — the composer holds its own references
	// to its stage dependencies. Health is a transparent proxy.
	CKG ckgclient.Client
	CKV ckvclient.Client

	// BuilderVersion is echoed in cks.ops.health responses and helps a
	// caller correlate health output with running binary build tags.
	// Empty string is acceptable; the field is informational, not load-bearing.
	BuilderVersion string

	// InstanceName is this server's identity (MCP handshake name + echoed in
	// cks.ops.health). When several cks-mcp instances run on different ports
	// (one per dataset), it tells a caller connecting by ip:port which one it
	// reached. Empty defaults to "cks".
	InstanceName string
	// InstanceDescription is optional human-facing metadata surfaced in
	// cks.ops.health alongside InstanceName.
	InstanceDescription string

	// Embed describes the embedding backend this instance serves (provider,
	// model, endpoint, dimension). Surfaced by cks.ops.health so a caller can
	// tell which model + instance it reached. Filled from config, so it
	// reports intended identity even when the backend is down.
	Embed embedder.Capability

	// Vocab is the glossary resolver shared with the composer's Stage 1.
	// When non-nil it backs the opt-in `expand` flag on semantic_search /
	// search_text, so direct callers get the same concept→symbol query
	// expansion the get_for_task path already enjoys. Nil → expand is a
	// no-op (the verbatim query is used).
	Vocab *vocab.Resolver

	// Index configures the cks.ops.index maintenance tool (G8). Zero value
	// (no binaries) disables it — the tool then tells the agent to run the
	// indexers manually. Not used by the query path.
	Index IndexConfig
}

// Register attaches both tools to s. Returns an error when required Deps
// fields are nil — the slim surface refuses to start half-wired rather
// than silently emit "tool not registered" at call time.
func Register(s *mcpserver.MCPServer, d Deps) error {
	if s == nil {
		return errors.New("mcp: nil MCPServer")
	}
	if d.Composer == nil {
		return errors.New("mcp: nil Deps.Composer")
	}
	if d.CKG == nil {
		return errors.New("mcp: nil Deps.CKG")
	}
	if d.CKV == nil {
		return errors.New("mcp: nil Deps.CKV")
	}

	registerGetForTask(s, d)
	registerHealth(s, d)
	registerFindSymbol(s, d)
	registerFindCallers(s, d)
	registerFindCallees(s, d)
	registerGetSubgraph(s, d)
	registerImpactAnalysis(s, d)
	registerConcurrencyImpact(s, d)
	registerChangeHistory(s, d)
	registerSemanticSearch(s, d)
	registerSearchText(s, d)
	registerFreshness(s, d)
	registerOpsIndex(s, d)
	// Phase D flow-aware direct-call tools (D-4). Backend bodies are stubbed
	// until CKV ships pkg/ckv.Engine flow methods (coordination §9.2-R).
	registerGetFlow(s, d)
	registerExpandFlow(s, d)
	registerFindBranches(s, d)
	registerGetInvariantEnforcement(s, d)
	return nil
}

// build constructs an MCP server (name from InstanceName, default "cks";
// version v from BuilderVersion or a fallback) with all tools registered.
// Shared by the stdio (Run) and Streamable-HTTP (RunHTTP) entry points so the
// registered surface is identical across transports.
func build(d Deps) (*mcpserver.MCPServer, error) {
	v := d.BuilderVersion
	if v == "" {
		v = "0.0.0"
	}
	name := d.InstanceName
	if name == "" {
		name = "cks"
	}
	s := mcpserver.NewMCPServer(name, v)
	if err := Register(s, d); err != nil {
		return nil, fmt.Errorf("mcp: register: %w", err)
	}
	return s, nil
}

// Run registers the tools and serves stdio until ctx is cancelled or stdin
// closes. Intended entry point for cmd/cks-mcp's default (stdio) transport.
func Run(ctx context.Context, d Deps) error {
	s, err := build(d)
	if err != nil {
		return err
	}
	// mcp-go's stdio server does not currently surface ctx into its
	// loop; the caller's ctx still gates anything Compose / Health do
	// (each handler threads ctx through to the composer / clients).
	_ = ctx
	if err := mcpserver.ServeStdio(s); err != nil {
		return fmt.Errorf("mcp: serve stdio: %w", err)
	}
	return nil
}

// HTTPPolicy controls who may connect to the Streamable HTTP listener.
//
// AllowRemote false → loopback clients only. AllowRemote true with no
// AllowedCIDRs → loopback + private/LAN (RFC1918/ULA) + link-local, matching a
// trusted local network. AllowRemote true with AllowedCIDRs → loopback plus
// exactly those networks. This is network-scope filtering, not per-client
// auth: any host on an allowed network can connect.
type HTTPPolicy struct {
	AllowRemote  bool
	AllowedCIDRs []string
}

// RunHTTP serves the MCP surface over Streamable HTTP on addr until ctx is
// cancelled, then shuts the listener down gracefully. Unlike stdio (one
// client per subprocess) this lets several cks instances run on different
// ports and accept connections from remote Claude Code clients; cks.ops.health
// advertises which DB/model+indexed commit each instance serves so a caller
// can identify the instance it reached. Client source IPs are filtered per
// policy (clientACL).
func RunHTTP(ctx context.Context, d Deps, addr string, policy HTTPPolicy) error {
	s, err := build(d)
	if err != nil {
		return err
	}
	allow, err := clientACL(policy)
	if err != nil {
		return err
	}
	mcpHandler := mcpserver.NewStreamableHTTPServer(s)
	srv := &http.Server{Addr: addr, Handler: aclMiddleware(mcpHandler, allow)}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("mcp: serve http on %q: %w", addr, err)
		}
		return nil
	}
}

// clientACL compiles policy into a predicate over client IPs. Loopback is
// always allowed. When AllowRemote is false, only loopback passes. With
// explicit CIDRs, only loopback + those networks pass; otherwise the default
// LAN policy (private + link-local) applies.
func clientACL(policy HTTPPolicy) (func(net.IP) bool, error) {
	var nets []*net.IPNet
	for _, cidr := range policy.AllowedCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("mcp: invalid allowed CIDR %q: %w", cidr, err)
		}
		nets = append(nets, n)
	}
	return func(ip net.IP) bool {
		if ip == nil {
			return false
		}
		if ip.IsLoopback() {
			return true
		}
		if !policy.AllowRemote {
			return false
		}
		if len(nets) > 0 {
			for _, n := range nets {
				if n.Contains(ip) {
					return true
				}
			}
			return false
		}
		// Default LAN policy: private (RFC1918 + ULA fc00::/7) and link-local.
		return ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}, nil
}

// aclMiddleware rejects requests whose source IP fails allow. It uses the
// direct TCP peer (RemoteAddr), not X-Forwarded-For, which a client could spoof.
func aclMiddleware(next http.Handler, allow func(net.IP) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !allow(net.ParseIP(host)) {
			http.Error(w, "forbidden: client network not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// registerGetForTask wires the cks.context.get_for_task tool. The schema
// is deliberately one input field; richer envelope fields land in a
// later phase.
func registerGetForTask(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameGetForTask,
		mcpgo.WithDescription(
			"Compose a sanitized EvidencePack from a vibe prompt. Runs intent classification, "+
				"keyword extraction (ckv+BM25), citation search (ckg), graph expansion, token "+
				"budgeting, and policy sanitize. Returns a SHA-256 integrity-stamped pack ready "+
				"for an upper-layer LLM consumer.",
		),
		mcpgo.WithString("prompt", mcpgo.Required(),
			mcpgo.Description("Natural-language task description. Example: \"find where ProcessRequest validates input\".")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleGetForTask(ctx, d, req)
	})
}

// registerHealth wires the cks.ops.health tool.
func registerHealth(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameHealth,
		mcpgo.WithDescription(
			"Aggregate cks backend health. Reports ok | degraded | down based on ckg/ckv "+
				"reachability per HLD §10. ckg is required; ckv unavailable yields degraded.",
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleHealth(ctx, d, req)
	})
}
