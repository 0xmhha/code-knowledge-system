package mcp

import (
	"context"
	"strings"
	"unicode"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// ftsOrQuery turns a free-text query into an explicit FTS5 OR expression for
// the search_text tool. search_text is documented as "terms joined as OR",
// but FTS5 parses a bare multi-word string as an implicit AND of barewords —
// so a natural-language or glossary-expanded query matches nothing unless one
// document contains every term, and any FTS5-special character (?, ", *, :, …)
// raises a syntax error. Tokenizing on non-word runes, quoting each token, and
// joining with OR honours the contract and makes multi-term / expanded queries
// usable. This lives at the tool boundary only; the composer's internal BM25
// seeding (which feeds clean per-keyword terms) is intentionally untouched.
func ftsOrQuery(query string) string {
	toks := strings.FieldsFunc(query, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	})
	quoted := make([]string, 0, len(toks))
	seen := make(map[string]struct{}, len(toks))
	for _, t := range toks {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		quoted = append(quoted, `"`+t+`"`)
	}
	if len(quoted) == 0 {
		return query
	}
	return strings.Join(quoted, " OR ")
}

const (
	ToolNameSemanticSearch = "cks.context.semantic_search"
	ToolNameSearchText     = "cks.context.search_text"
)

// searchResponse is the shared wire shape for both search tools. Source
// in each Hit identifies which backend produced it.
type searchResponse struct {
	Query        string                      `json:"query"`
	Hits         []contract.Hit              `json:"hits"`
	Instructions []contract.DummyInstruction `json:"instructions,omitempty"`
}

// registerSemanticSearch wires cks.context.semantic_search.
func registerSemanticSearch(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameSemanticSearch,
		mcpgo.WithDescription(
			"Vector-similarity search over the ckv index. Use this when the query is a "+
				"natural-language description and you want chunks whose meaning matches, not just "+
				"chunks whose tokens overlap. For exact-keyword lookups use search_text.",
		),
		mcpgo.WithString("query", mcpgo.Required(),
			mcpgo.Description("Natural-language query (e.g., \"validator quorum check at finalize\").")),
		mcpgo.WithNumber("k",
			mcpgo.Description("Maximum hits to return (0 = backend default).")),
		mcpgo.WithString("language",
			mcpgo.Description("Restrict to a language (e.g., \"go\").")),
		mcpgo.WithString("path_glob",
			mcpgo.Description("Restrict to file paths matching this glob.")),
		mcpgo.WithString("kinds",
			mcpgo.Description("Comma-separated symbol kinds (e.g., \"function,method\").")),
		withExcludeTests(),
		withExpand(),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleSemanticSearch(ctx, d, req)
	})
}

func handleSemanticSearch(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	query := req.GetString("query", "")
	if query == "" {
		return mcpgo.NewToolResultError(ToolNameSemanticSearch + ": missing required argument \"query\""), nil
	}
	opts := ckvclient.SearchOpts{
		K: intArg(req, "k", 0),
		Filter: ckvclient.SearchFilter{
			Language: req.GetString("language", ""),
			PathGlob: req.GetString("path_glob", ""),
		},
	}
	if kinds := req.GetString("kinds", ""); kinds != "" {
		opts.Filter.SymbolKinds = splitCSV(kinds)
	}

	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	hits, err := d.CKV.SemanticSearch(ctx, maybeExpand(d, req, query), opts)
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: %v", ToolNameSemanticSearch, err), nil
	}
	if excludeTestsArg(req) {
		hits = filterHitsTests(hits)
	}
	return mcpgo.NewToolResultStructured(searchResponse{
		Query:        query,
		Hits:         hits,
		Instructions: collector.All(),
	}, "semantic_search result"), nil
}

// registerSearchText wires cks.context.search_text.
func registerSearchText(s *mcpserver.MCPServer, d Deps) {
	tool := mcpgo.NewTool(ToolNameSearchText,
		mcpgo.WithDescription(
			"BM25 keyword search over the ckg full-text index. Use this when the query terms "+
				"should match exactly (function names, identifiers, error strings). For meaning-"+
				"based retrieval use semantic_search.",
		),
		mcpgo.WithString("query", mcpgo.Required(),
			mcpgo.Description("Keyword query (terms joined as OR by default).")),
		mcpgo.WithNumber("k",
			mcpgo.Description("Maximum hits to return (0 = backend default).")),
		mcpgo.WithString("language",
			mcpgo.Description("Restrict to a language (e.g., \"go\").")),
		mcpgo.WithString("path_glob",
			mcpgo.Description("Restrict to file paths matching this glob.")),
		withExcludeTests(),
		withExpand(),
	)
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleSearchText(ctx, d, req)
	})
}

func handleSearchText(ctx context.Context, d Deps, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	query := req.GetString("query", "")
	if query == "" {
		return mcpgo.NewToolResultError(ToolNameSearchText + ": missing required argument \"query\""), nil
	}
	opts := ckgclient.SearchOpts{
		K: intArg(req, "k", 0),
		Filter: ckgclient.SearchFilter{
			Language: req.GetString("language", ""),
			PathGlob: req.GetString("path_glob", ""),
		},
	}

	collector := contract.NewInstructionCollector()
	ctx = contract.WithCollector(ctx, collector)

	hits, err := d.CKG.BM25Search(ctx, ftsOrQuery(maybeExpand(d, req, query)), opts)
	if err != nil {
		return mcpgo.NewToolResultErrorf("%s: %v", ToolNameSearchText, err), nil
	}
	if excludeTestsArg(req) {
		hits = filterHitsTests(hits)
	}
	return mcpgo.NewToolResultStructured(searchResponse{
		Query:        query,
		Hits:         hits,
		Instructions: collector.All(),
	}, "search_text result"), nil
}
