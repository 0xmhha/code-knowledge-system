package ckgclient

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0xmhha/code-knowledge-graph/pkg/concurrency"
	"github.com/0xmhha/code-knowledge-graph/pkg/evidence"
	"github.com/0xmhha/code-knowledge-graph/pkg/impact"
	"github.com/0xmhha/code-knowledge-graph/pkg/store"
	"github.com/0xmhha/code-knowledge-graph/pkg/types"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// DefaultSearchLimit is the limit passed to ckg's SearchFTS when SearchOpts.K
// is zero. Mirrors ckg's typical default; tuned downward in callers that
// budget for fewer hits.
const DefaultSearchLimit = 10

// DefaultSubgraphMaxTotal bounds GetSubgraph's edge set (and, transitively, the
// incident-node citations) when the caller passes MaxTotal<=0. A depth>1
// traversal from a hub symbol returns thousands of edges; without a cap the
// translated response overflows the MCP per-result token budget. Measured on
// the go-stablenet index (processFinalize, depth=2): 80 edges -> ~33 KB, which
// stays under the ~25k-token per-result inline limit with margin (200 -> 64 KB
// already tripped it). Callers wanting a wider view pass an explicit MaxTotal
// and handle the larger payload.
const DefaultSubgraphMaxTotal = 80

// FilterOverfetchRatio scales the SearchFTS limit when a non-empty
// SearchOpts.Filter is present. Post-filter discards rows the caller
// did not want, so we pull this many times more rows up front to keep
// the kept-row count close to the caller's K. 3× balances "enough
// survivors" against "extra SQLite work per call." Phase E may tune
// this based on miss rates.
const FilterOverfetchRatio = 3

// ManifestSnapshot is the cks-internal projection of ckg's persist.Manifest.
//
// ckg's Manifest type lives in internal/persist and is not importable from
// outside the ckg module, so callers wrap it into this struct via
// storeReader.LoadManifestSnapshot. The fields covered here are the ones
// the cks adapter actually surfaces (snapshot pin + freshness).
type ManifestSnapshot struct {
	SchemaVersion  string
	CKGVersion     string
	BuildTimestamp string
	SrcRoot        string
	SrcCommit      string
}

// storeReader is the cks-internal seam between the Real adapter and the
// ckg store.Reader. It exists so tests can inject a mock without bringing
// up a SQLite fixture, and because ckg's GetManifest() returns a type
// from an internal package — LoadManifestSnapshot strips that detail
// so mocks don't need to import internal/persist (which they can't).
type storeReader interface {
	LoadManifestSnapshot() (ManifestSnapshot, error)
	// SearchFTS returns scored hits (G5): the ckg store carries a real,
	// result-set-normalized Score in [0,1] plus a backend-native RawScore.
	SearchFTS(q string, limit int) ([]store.SearchHit, error)
	FindSymbol(name string, exact bool) ([]types.Node, error)
	// FindByCanonicalID resolves ckg's globally-unique, import-path-qualified
	// symbol id (ADR-0001) to its node; found=false (nil error) on miss/empty.
	// A canonical id cannot collide across packages, so it is the preferred
	// seed when a CKV hit or domain anchor carries one.
	FindByCanonicalID(canonicalID string) (types.Node, bool, error)
	NodesByFilePath(path string) ([]types.Node, error)
	NeighborhoodByQname(qname string, depth int, reverse bool, edgeTypes ...string) ([]types.Node, []types.Edge, error)
	SubgraphByQname(qname string, depth int) ([]types.Node, []types.Edge, error)
	// G3 seam: surface ckg's pkg/impact + pkg/evidence + GetNodePRs through
	// the same store.Reader the adapter already holds, so mocks stay injectable.
	ImpactCompute(seedQname, seedFile string, depth int, includeBlobs bool) (map[string]any, error)
	EvidenceBuildPack(intent, seedQname string, k int) (*evidence.Pack, error)
	GetNodePRs(nodeID string, cutoff time.Time) ([]store.PRRef, error)
	ConcurrencyAnalyze(symbol string, depth, maxTotal int) (concurrency.Result, error)
	Close() error
}

// realStoreReader wraps a ckg store.Reader so it satisfies the cks-internal
// storeReader interface. Pure passthrough except for LoadManifestSnapshot
// which strips the persist.Manifest dependency.
type realStoreReader struct {
	r store.Reader
}

func (a *realStoreReader) LoadManifestSnapshot() (ManifestSnapshot, error) {
	m, err := a.r.GetManifest()
	if err != nil {
		return ManifestSnapshot{}, err
	}
	return ManifestSnapshot{
		SchemaVersion:  m.SchemaVersion,
		CKGVersion:     m.CKGVersion,
		BuildTimestamp: m.BuildTimestamp,
		SrcRoot:        m.SrcRoot,
		SrcCommit:      m.SrcCommit,
	}, nil
}
func (a *realStoreReader) SearchFTS(q string, limit int) ([]store.SearchHit, error) {
	return a.r.SearchFTS(q, limit, store.SearchFTSOptions{})
}
func (a *realStoreReader) FindSymbol(name string, exact bool) ([]types.Node, error) {
	return a.r.FindSymbol(name, exact, store.FindSymbolOptions{})
}
func (a *realStoreReader) FindByCanonicalID(canonicalID string) (types.Node, bool, error) {
	return a.r.FindByCanonicalID(canonicalID)
}
func (a *realStoreReader) ImpactCompute(seedQname, seedFile string, depth int, includeBlobs bool) (map[string]any, error) {
	return impact.Compute(a.r, seedQname, seedFile, impact.Options{Depth: depth, IncludeBlobs: includeBlobs})
}
func (a *realStoreReader) EvidenceBuildPack(intent, seedQname string, k int) (*evidence.Pack, error) {
	return evidence.BuildPack(a.r, evidence.Options{Intent: intent, SeedQname: seedQname, K: k})
}
func (a *realStoreReader) GetNodePRs(nodeID string, cutoff time.Time) ([]store.PRRef, error) {
	return a.r.GetNodePRs(nodeID, cutoff)
}
func (a *realStoreReader) ConcurrencyAnalyze(symbol string, depth, maxTotal int) (concurrency.Result, error) {
	return concurrency.Analyze(a.r, symbol, concurrency.Options{Depth: depth, MaxTotal: maxTotal})
}
func (a *realStoreReader) NodesByFilePath(path string) ([]types.Node, error) {
	return a.r.NodesByFilePath(path)
}
func (a *realStoreReader) NeighborhoodByQname(qname string, depth int, reverse bool, edgeTypes ...string) ([]types.Node, []types.Edge, error) {
	return a.r.NeighborhoodByQname(qname, depth, reverse, edgeTypes...)
}
func (a *realStoreReader) SubgraphByQname(qname string, depth int) ([]types.Node, []types.Edge, error) {
	return a.r.SubgraphByQname(qname, depth)
}
func (a *realStoreReader) Close() error {
	return a.r.Close()
}

// Real is the production ckg Client adapter. Holds a storeReader (typically
// a *realStoreReader wrapping store.Reader) and translates between ckg's
// (Node, Edge, Manifest) vocabulary and cks's (Hit, Citation, Neighbor,
// Health) vocabulary.
//
// Concurrency: Real itself is stateless after construction (the underlying
// store.Reader is concurrent-safe for read operations); concurrent calls
// across multiple goroutines are supported.
type Real struct {
	s      storeReader
	closed bool
}

// Compile-time guarantee Real satisfies Client.
var _ Client = (*Real)(nil)

// NewReal opens a ckg SQLite store at path and returns a Client. The store
// is opened read-only; callers must invoke Close to release the underlying
// file handle.
func NewReal(path string) (*Real, error) {
	if path == "" {
		return nil, errors.New("ckgclient: empty store path")
	}
	r, err := store.OpenReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("ckgclient: open %q: %w", path, err)
	}
	out := newRealWithStore(&realStoreReader{r: r})
	// Confirm we can read the manifest. A torn store, schema mismatch, or
	// stale db file shows up here rather than at first query.
	if _, mErr := out.s.LoadManifestSnapshot(); mErr != nil {
		_ = out.s.Close()
		return nil, fmt.Errorf("ckgclient: load manifest at %q: %w", path, mErr)
	}
	return out, nil
}

// newRealWithStore is the test seam: lets tests inject a mock storeReader
// without going through ckg's OpenReadOnly path. Production code should
// always use NewReal.
func newRealWithStore(s storeReader) *Real {
	return &Real{s: s}
}

// BM25Search forwards query to ckg's SearchFTS, then translates the
// returned Nodes into Hits stamped with HitSourceCKG.
//
// Filter handling: opts.Filter.Language and opts.Filter.PathGlob are
// applied as client-side post-filters because ckg's SearchFTS takes
// only (query, limit). To compensate for the post-filter dropping
// rows, we over-fetch by FilterOverfetchRatio× when a filter is set.
// opts.Filter.CommitHash is currently ignored — the entire index
// represents one snapshot pinned by manifest.SrcCommit, so a
// per-Citation commit filter has no incremental signal until ckg
// supports cross-commit search.
//
// Score policy (G5): ckg's SearchFTS returns a real, result-set-normalized
// Score in [0,1] (descending) on every SearchHit — cks consumes it verbatim
// rather than synthesizing a rank gradient. NB: a degenerate single-row or
// all-equal result set has every Score = 1.0 — ckg's min-max maps uniform
// strength to 1.0 (not 0.0), so "1.0" means "uniform strength," not
// "perfect match"; downstream stage2 weighting must not over-trust it.
func (r *Real) BM25Search(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	if query == "" {
		return nil, errors.New("ckgclient: empty query")
	}
	limit := opts.K
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	// Over-fetch when a filter is set so the post-filter has rows to
	// pick from. Otherwise pull exactly K rows.
	fetchLimit := limit
	hasFilter := opts.Filter.Language != "" || opts.Filter.PathGlob != ""
	if hasFilter {
		fetchLimit = limit * FilterOverfetchRatio
	}

	shits, err := r.s.SearchFTS(query, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("ckgclient: SearchFTS: %w", err)
	}
	if len(shits) == 0 {
		return nil, nil
	}

	commit, _ := r.commit()
	hits := make([]contract.Hit, 0, len(shits))
	kept := 0
	for i, sh := range shits {
		if !matchesFilter(sh.Node, opts.Filter) {
			continue
		}
		hits = append(hits, contract.Hit{
			Citation: nodeToCitation(sh.Node, commit),
			Rank:     i + 1,
			Score:    sh.Score, // real ckg score, already normalized to [0,1]
			Source:   contract.HitSourceCKG,
		})
		kept++
		if kept >= limit {
			break
		}
	}
	return hits, nil
}

// matchesFilter reports whether n satisfies every set field of f.
// Empty fields are ignored. PathGlob uses filepath.Match semantics —
// single-star, no "**" expansion — matched against n.FilePath.
func matchesFilter(n types.Node, f SearchFilter) bool {
	if f.Language != "" && f.Language != n.Language {
		return false
	}
	if f.PathGlob != "" {
		ok, err := filepath.Match(f.PathGlob, n.FilePath)
		if err != nil || !ok {
			return false
		}
	}
	return true
}

// FindSymbol delegates to ckg's FindSymbol with exact=false (suffix match
// on qualified name) — cks callers typically pass bare symbol names like
// "ProcessRequest" rather than fully-qualified ones, and suffix-match is
// the only way to resolve those against ckg's qualified-name index.
//
// opts.Kinds is applied client-side because ckg's FindSymbol does not
// take a kind filter. The mapping is lowercase cks kind to ckg NodeType
// (e.g. "function" -> "Function", "method" -> "Method").
//
// opts.PathGlob and opts.CommitHash are not yet enforced; follow-up.
// resolveFlexibleNodes resolves a possibly over-qualified dotted symbol name
// against ckg's qualified-name index, which stores Go symbols in the
// package-leaf form "pkgleaf.Type.Method" (e.g. "validator.defaultSet.QuorumSize")
// and never the dotted import path. A caller that follows the MCP tool docs and
// passes a fully-qualified "consensus.wbft.validator.defaultSet.QuorumSize" would
// otherwise match nothing (neither exact nor "%."+name suffix). We try the name
// as given, then progressively drop leading dot-segments until the store resolves
// it. Bare/exact names hit on the first try, so existing behaviour is unchanged.
func (r *Real) resolveFlexibleNodes(name string) ([]types.Node, error) {
	nodes, err := r.s.FindSymbol(name, false)
	if err != nil || len(nodes) > 0 {
		return nodes, err
	}
	segs := strings.Split(name, ".")
	for i := 1; i < len(segs); i++ {
		nodes, err = r.s.FindSymbol(strings.Join(segs[i:], "."), false)
		if err != nil {
			return nil, err
		}
		if len(nodes) > 0 {
			return nodes, nil
		}
	}
	return nil, nil
}

func (r *Real) FindSymbol(ctx context.Context, name string, opts SymbolOpts) ([]contract.Citation, error) {
	if name == "" {
		return nil, errors.New("ckgclient: empty symbol name")
	}
	nodes, err := r.resolveFlexibleNodes(name)
	if err != nil {
		return nil, fmt.Errorf("ckgclient: FindSymbol: %w", err)
	}
	commit, _ := r.commit()
	out := make([]contract.Citation, 0, len(nodes))
	for _, nd := range nodes {
		if !nodeMatchesKinds(nd, opts.Kinds) {
			continue
		}
		out = append(out, nodeToCitation(nd, commit))
	}
	return out, nil
}

// Neighbors resolves the source Citation to a ckg qualified name via
// NodesByFilePath + StartLine match, then walks NeighborhoodByQname to
// gather the requested relations. Edges whose ckg EdgeType has no cks
// Relation analog are dropped (rather than guessed) so downstream
// consumers never see a misclassified relation.
//
// Direction handling: when opts.Relations contains RelationCalledBy,
// the traversal uses ckg's reverse=true argument — the only way ckg
// surfaces inbound edges in NeighborhoodByQname's signature. If callers
// mix forward + reverse relations in one call, this implementation
// performs two ckg queries and concatenates; C.1 keeps it simple by
// supporting a single direction per call (mixed direction yields an
// error to make the limitation explicit).
func (r *Real) Neighbors(ctx context.Context, src contract.Citation, opts NeighborsOpts) ([]contract.Neighbor, error) {
	if !src.IsValid() {
		return nil, errors.New("ckgclient: invalid source citation")
	}
	if opts.Hops < 0 {
		return nil, errors.New("ckgclient: negative hops")
	}
	depth := opts.Hops
	if depth == 0 {
		depth = 1
	}

	// Resolve the source citation to a qname.
	cands, err := r.s.NodesByFilePath(src.File)
	if err != nil {
		return nil, fmt.Errorf("ckgclient: NodesByFilePath: %w", err)
	}
	qname := matchQname(cands, src)
	if qname == "" {
		return nil, fmt.Errorf("ckgclient: no node at %s:%d-%d", src.File, src.StartLine, src.EndLine)
	}

	// Decide direction + edge-type filter from opts.Relations.
	reverse, edgeTypes, err := planTraversal(opts.Relations)
	if err != nil {
		return nil, err
	}

	// Interface-dispatch bridge: when this is a reverse (callers) walk on a
	// concrete method pkg.T.M, callers that reach it through an interface are
	// recorded as `invokes` edges to the INTERFACE method pkg.I.M, not the
	// concrete method — so a reverse walk from the concrete method alone misses
	// every interface-dispatched caller. Union the concrete seed with the
	// same-named method on each interface T implements. Mirrors
	// ckg/pkg/mcphandlers.reverseCallersUnion; no-op for forward walks and for
	// seeds with no Type.Method shape / no implements edges.
	seeds := []string{qname}
	if reverse {
		seeds = append(seeds, r.interfaceMethodSeeds(qname)...)
	}
	nodeByID := make(map[string]types.Node)
	edgeSeen := make(map[string]struct{})
	nodes := make([]types.Node, 0)
	edges := make([]types.Edge, 0)
	for _, sd := range seeds {
		ns, es, err := r.s.NeighborhoodByQname(sd, depth, reverse, edgeTypes...)
		if err != nil {
			return nil, fmt.Errorf("ckgclient: NeighborhoodByQname: %w", err)
		}
		for _, n := range ns {
			if _, ok := nodeByID[n.ID]; ok {
				continue
			}
			nodeByID[n.ID] = n
			nodes = append(nodes, n)
		}
		for _, e := range es {
			k := e.Src + "|" + e.Dst + "|" + string(e.Type)
			if _, ok := edgeSeen[k]; ok {
				continue
			}
			edgeSeen[k] = struct{}{}
			edges = append(edges, e)
		}
	}

	commit, _ := r.commit()
	byID := nodeByID

	out := make([]contract.Neighbor, 0, len(edges))
	for _, e := range edges {
		rel, ok := relationFromEdgeType(e.Type, reverse)
		if !ok {
			continue
		}
		srcN, srcOK := byID[e.Src]
		dstN, dstOK := byID[e.Dst]
		if !srcOK || !dstOK {
			continue
		}
		if opts.MaxTotal > 0 && len(out) >= opts.MaxTotal {
			break
		}
		out = append(out, contract.Neighbor{
			Source:   nodeToCitation(srcN, commit),
			Target:   nodeToCitation(dstN, commit),
			Relation: rel,
			Distance: 1, // C.1: single-hop neighbor distance; depth>1 returns
			// nodes/edges flattened by ckg, but ckg does not annotate per-edge
			// hop count. Refine with BFS when depth>1 becomes a real use case.
		})
	}
	return out, nil
}

// interfaceMethodSeeds returns extra find_callers seeds for the
// interface-dispatch bridge. When methodQname is a concrete method `pkg.T.M`,
// callers that reach it through an interface are recorded as `invokes` edges to
// the INTERFACE method node (`pkg.I.M`), not the concrete method — so a reverse
// walk from the concrete method alone misses every interface-dispatched caller.
// This returns the same-named method on each interface that T implements, using
// the `implements` edges already in the graph (no reindex needed). Generalized:
// a top-level function (no receiver segment) or a type implementing nothing
// yields no extra seeds. Mirrors ckg/pkg/mcphandlers.interfaceMethodSeeds.
func (r *Real) interfaceMethodSeeds(methodQname string) []string {
	dot := strings.LastIndex(methodQname, ".")
	if dot <= 0 || dot == len(methodQname)-1 {
		return nil // no `Type.Method` shape → nothing to bridge
	}
	typeQname, methodShort := methodQname[:dot], methodQname[dot+1:]

	// Forward `implements` edges from the receiver type → interfaces it satisfies.
	nodes, edges, err := r.s.NeighborhoodByQname(typeQname, 1, false, string(types.EdgeImplements))
	if err != nil {
		return nil
	}
	ifaceIDs := make(map[string]struct{})
	for _, e := range edges {
		if e.Type == types.EdgeImplements {
			ifaceIDs[e.Dst] = struct{}{}
		}
	}
	var seeds []string
	seen := make(map[string]struct{})
	for _, n := range nodes {
		if _, ok := ifaceIDs[n.ID]; !ok || n.Type != types.NodeInterface {
			continue
		}
		mq := n.QualifiedName + "." + methodShort
		if _, dup := seen[mq]; dup {
			continue
		}
		seen[mq] = struct{}{}
		seeds = append(seeds, mq)
	}
	sort.Strings(seeds) // deterministic for prompt-cache stability
	return seeds
}

// ImpactOfChange computes the reverse-dependency closure from seedQname via
// ckg's pkg/impact.Compute, partitioned by coupling category. impact.Compute
// needs a seedFile too, which cks doesn't carry at the seam — we resolve it
// from the qname's definition node (first match) and fall back to qname-only
// resolution (impact.Compute accepts an empty seedFile).
func (r *Real) ImpactOfChange(ctx context.Context, seedQname string, opts ImpactOpts) (contract.ImpactResult, error) {
	if seedQname == "" {
		return contract.ImpactResult{}, errors.New("ckgclient: empty seed qname")
	}
	// Suffix-resolve a partial seed; impact.Compute requires an exact qname.
	if resolved := r.resolveQname(seedQname); resolved != "" {
		seedQname = resolved
	}
	seedFile := r.resolveSeedFile(seedQname)
	raw, err := r.s.ImpactCompute(seedQname, seedFile, opts.Depth, false)
	if err != nil {
		return contract.ImpactResult{}, fmt.Errorf("ckgclient: impact: %w", err)
	}
	commit, _ := r.commit()
	return impactResultFromMap(seedQname, raw, commit, opts.MaxTotal), nil
}

// EvidenceForIntent returns BM25-ranked hunk evidence for intent via ckg's
// pkg/evidence.BuildPack, flattened from per-commit Hits into a single
// hunk list. PRs are surfaced separately by GetNodePRs.
func (r *Real) EvidenceForIntent(ctx context.Context, intent string, opts EvidenceOpts) (contract.ChangeHistoryResult, error) {
	if intent == "" {
		return contract.ChangeHistoryResult{}, errors.New("ckgclient: empty intent")
	}
	pack, err := r.s.EvidenceBuildPack(intent, opts.SeedQname, opts.K)
	if err != nil {
		return contract.ChangeHistoryResult{}, fmt.Errorf("ckgclient: evidence: %w", err)
	}
	out := contract.ChangeHistoryResult{Seed: opts.SeedQname}
	if pack == nil {
		return out, nil
	}
	for _, h := range pack.Hits {
		for _, hr := range h.Hunks {
			out.Hunks = append(out.Hunks, contract.HunkEvidence{
				File:      hr.FilePath,
				StartLine: hr.StartLine,
				EndLine:   hr.EndLine,
				Patch:     hr.PatchText,
			})
		}
	}
	return out, nil
}

// GetNodePRs resolves qname to its definition node, then returns the PRs that
// touched it (ckg store.Reader.GetNodePRs takes a node ID). A zero cutoff
// means "no time filter"; opts.MaxCount truncates.
func (r *Real) GetNodePRs(ctx context.Context, qname string, opts PRRefOpts) ([]contract.PRRef, error) {
	if qname == "" {
		return nil, errors.New("ckgclient: empty qname")
	}
	nodeID := r.resolveNodeID(qname)
	if nodeID == "" {
		return nil, nil
	}
	prs, err := r.s.GetNodePRs(nodeID, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("ckgclient: GetNodePRs: %w", err)
	}
	out := make([]contract.PRRef, 0, len(prs))
	for _, p := range prs {
		if opts.MaxCount > 0 && len(out) >= opts.MaxCount {
			break
		}
		out = append(out, contract.PRRef{
			Number:   p.Number,
			Title:    p.Title,
			Summary:  p.Summary,
			BaseSHA:  p.BaseSHA,
			HeadSHA:  p.HeadSHA,
			MergedAt: p.MergedAtUTC,
			Repo:     p.Repo,
		})
	}
	return out, nil
}

// uniqueResolved picks the node that `name` resolves to from suffix-match defs
// WITHOUT ever silently guessing (ADR-0001, Phase 3): an exact qualified_name
// match wins; otherwise the single distinct match is returned; a multi-match is
// reported as unresolved (ok=false) so callers fail safe rather than binding to
// the arbitrary first-of-N (the old `defs[0]`, which could be the wrong symbol).
func uniqueResolved(defs []types.Node, name string) (types.Node, bool) {
	for _, d := range defs {
		if d.QualifiedName == name {
			return d, true
		}
	}
	seen := map[string]types.Node{}
	for _, d := range defs {
		seen[d.QualifiedName] = d
	}
	if len(seen) == 1 {
		for _, d := range seen {
			return d, true
		}
	}
	return types.Node{}, false
}

// resolveSeedFile returns the FilePath of qname's definition node. Resolution
// order: exact canonical_id (ADR-0001) → exact/unique qname; "" when nothing
// resolves OR the name is ambiguous (no silent first-of-N pick). Used to give
// impact.Compute its second seed arg.
func (r *Real) resolveSeedFile(qname string) string {
	if n, found, _ := r.s.FindByCanonicalID(qname); found {
		return n.FilePath
	}
	defs, err := r.s.FindSymbol(qname, false)
	if err != nil || len(defs) == 0 {
		return ""
	}
	if n, ok := uniqueResolved(defs, qname); ok {
		return n.FilePath
	}
	return ""
}

// resolveQname normalizes a possibly-partial symbol name to a stored
// fully-qualified name. Resolution order: exact canonical_id → exact/unique
// qname. Returns "" when the name does not resolve OR is ambiguous, in which
// case callers pass the original name through unchanged so the downstream
// exact-match traversal behaves as before.
//
// This closes the seed-resolution gap between the find_symbol/find_callers
// family (which suffix-match) and the graph-traversal family (GetSubgraph,
// ImpactOfChange, ConcurrencyImpact) whose ckg backends require an exact
// qualified_name and otherwise return empty silently — while never binding an
// ambiguous bare name to an arbitrary candidate.
func (r *Real) resolveQname(name string) string {
	if n, found, _ := r.s.FindByCanonicalID(name); found {
		return n.QualifiedName
	}
	defs, err := r.resolveFlexibleNodes(name)
	if err != nil || len(defs) == 0 {
		return ""
	}
	if n, ok := uniqueResolved(defs, name); ok {
		return n.QualifiedName
	}
	return ""
}

// resolveNodeID returns the node ID of qname's definition. Resolution order:
// exact canonical_id → exact/unique qname; "" when nothing resolves OR the name
// is ambiguous. Used to bridge qname→nodeID for store.Reader.GetNodePRs.
func (r *Real) resolveNodeID(qname string) string {
	if n, found, _ := r.s.FindByCanonicalID(qname); found {
		return n.ID
	}
	defs, err := r.resolveFlexibleNodes(qname)
	if err != nil || len(defs) == 0 {
		return ""
	}
	if n, ok := uniqueResolved(defs, qname); ok {
		return n.ID
	}
	return ""
}

// impactGroupOrder pins the ckg group key → cks category mapping in a fixed
// order so the response is deterministic (Go map iteration is random).
var impactGroupOrder = []struct {
	ckgKey string
	cat    contract.ImpactCategory
}{
	{"callers", contract.ImpactCallers},
	{"interface_impact", contract.ImpactInterface},
	{"type_users", contract.ImpactTypeUsers},
	{"distributed", contract.ImpactDistributed},
	{"concurrent", contract.ImpactConcurrent},
	{"other_refs", contract.ImpactOther},
}

// impactResultFromMap translates ckg's impact.Compute map[string]any envelope
// into a typed contract.ImpactResult. Each per-bucket entry carries "file"
// (string) + "line" (int = StartLine); end line isn't in the impact entry so
// EndLine mirrors StartLine. maxTotal caps citations across all groups.
func impactResultFromMap(seed string, raw map[string]any, commit string, maxTotal int) contract.ImpactResult {
	out := contract.ImpactResult{Seed: seed}
	if raw == nil {
		return out
	}
	if nf, _ := raw["not_found"].(bool); nf {
		return out
	}
	impactMap, _ := raw["impact"].(map[string]any)
	if impactMap == nil {
		return out
	}
	total := 0
	for _, g := range impactGroupOrder {
		entries, _ := impactMap[g.ckgKey].([]map[string]any)
		if len(entries) == 0 {
			continue
		}
		grp := contract.ImpactGroup{Category: g.cat}
		for _, e := range entries {
			if maxTotal > 0 && total >= maxTotal {
				break
			}
			file, _ := e["file"].(string)
			line, _ := e["line"].(int)
			if file == "" || line <= 0 {
				continue
			}
			grp.Hits = append(grp.Hits, contract.Citation{
				File: file, StartLine: line, EndLine: line, CommitHash: commit,
			})
			total++
		}
		if len(grp.Hits) > 0 {
			out.Groups = append(out.Groups, grp)
		}
	}
	return out
}

// GetSubgraph delegates to ckg's SubgraphByQname and translates the result.
func (r *Real) GetSubgraph(ctx context.Context, qname string, opts SubgraphOpts) ([]contract.Citation, []contract.Neighbor, error) {
	if qname == "" {
		return nil, nil, errors.New("ckgclient: empty qname")
	}
	depth := opts.Depth
	if depth == 0 {
		depth = 1
	}
	// Suffix-resolve a partial seed (e.g. "Engine.Finalize") to its stored
	// fully-qualified name; SubgraphByQname matches qualified_name exactly.
	if resolved := r.resolveQname(qname); resolved != "" {
		qname = resolved
	}
	nodes, edges, err := r.s.SubgraphByQname(qname, depth)
	if err != nil {
		return nil, nil, fmt.Errorf("ckgclient: SubgraphByQname: %w", err)
	}
	commit, _ := r.commit()

	// Bound the response. A depth>1 traversal from a hub node can return
	// thousands of nodes/edges (hundreds of KB) and overflow the MCP token
	// budget — the old code emitted *every* node as a citation and only capped
	// edges, so callers got an unusable payload. Cap the edge set, then emit
	// only the nodes incident to the kept edges plus the seed, keeping nodes
	// and edges consistent and bounded. MaxTotal<=0 means "use the default cap"
	// (a true unbounded subgraph is what caused the overflow).
	limit := opts.MaxTotal
	if limit <= 0 {
		limit = DefaultSubgraphMaxTotal
	}

	byID := make(map[string]types.Node, len(nodes))
	var seedID string
	for _, n := range nodes {
		byID[n.ID] = n
		if seedID == "" && n.QualifiedName == qname {
			seedID = n.ID
		}
	}

	kept := make(map[string]bool, limit*2)
	if seedID != "" {
		kept[seedID] = true
	}
	neighbors := make([]contract.Neighbor, 0, limit)
	for _, e := range edges {
		if len(neighbors) >= limit {
			break
		}
		rel, ok := relationFromEdgeType(e.Type, false)
		if !ok {
			continue
		}
		srcN, srcOK := byID[e.Src]
		dstN, dstOK := byID[e.Dst]
		if !srcOK || !dstOK {
			continue
		}
		neighbors = append(neighbors, contract.Neighbor{
			Source:   nodeToCitation(srcN, commit),
			Target:   nodeToCitation(dstN, commit),
			Relation: rel,
			Distance: 1,
		})
		kept[e.Src] = true
		kept[e.Dst] = true
	}

	// Emit citations only for kept nodes, iterating the original node slice so
	// the order stays deterministic.
	citations := make([]contract.Citation, 0, len(kept))
	for _, n := range nodes {
		if kept[n.ID] {
			citations = append(citations, nodeToCitation(n, commit))
		}
	}
	return citations, neighbors, nil
}

// ConcurrencyImpact computes the seed symbol's concurrency blast radius via
// ckg's pkg/concurrency.Analyze (spawns / sends_to / recvs_from /
// acquires_lock / accessed_under_lock edges, both directions) and translates
// concurrency.Module → contract.ConcurrencyModule. Depth defaults to 3.
func (r *Real) ConcurrencyImpact(ctx context.Context, symbol string, opts ConcurrencyOpts) (contract.ConcurrencyResult, error) {
	if symbol == "" {
		return contract.ConcurrencyResult{}, errors.New("ckgclient: empty symbol")
	}
	depth := opts.Depth
	if depth <= 0 {
		depth = 3
	}
	// Suffix-resolve a partial seed; concurrency.Analyze requires an exact qname.
	if resolved := r.resolveQname(symbol); resolved != "" {
		symbol = resolved
	}
	res, err := r.s.ConcurrencyAnalyze(symbol, depth, opts.MaxTotal)
	if err != nil {
		return contract.ConcurrencyResult{}, fmt.Errorf("ckgclient: concurrency: %w", err)
	}
	commit, _ := r.commit()
	out := contract.ConcurrencyResult{
		Seed:     res.Seed,
		Depth:    res.Depth,
		NotFound: res.NotFound,
	}
	for _, m := range res.Modules {
		end := m.StartLine
		out.Modules = append(out.Modules, contract.ConcurrencyModule{
			Citation: contract.Citation{
				File:       m.FilePath,
				StartLine:  m.StartLine,
				EndLine:    end,
				CommitHash: commit,
			},
			Qname:     m.Qname,
			Name:      m.Name,
			Kind:      string(m.Type),
			Direction: m.Direction,
		})
	}
	return out, nil
}

// Health round-trips a manifest read and reports reachability + the
// snapshot's schema version + indexed head commit. Unlike BM25Search,
// Health never swallows the manifest error — operators need to see why
// a backend is unreachable.
func (r *Real) Health(ctx context.Context) (Health, error) {
	snap, err := r.s.LoadManifestSnapshot()
	if err != nil {
		return Health{Reachable: false}, fmt.Errorf("ckgclient: load manifest: %w", err)
	}
	return Health{
		Reachable:     true,
		SchemaVersion: snap.SchemaVersion,
		IndexedHead:   snap.SrcCommit,
	}, nil
}

// Close releases the underlying store handle. Idempotent — repeated calls
// after the first successful close are no-ops.
func (r *Real) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.s.Close()
}

// commit returns the manifest's SrcCommit, used as the per-Citation
// CommitHash. Failure here is non-fatal for read operations (we still
// return Citations with an empty CommitHash) but is surfaced to the
// caller through Health.
func (r *Real) commit() (string, error) {
	snap, err := r.s.LoadManifestSnapshot()
	if err != nil {
		return "", err
	}
	return snap.SrcCommit, nil
}

// --- Translation helpers ---

func nodeToCitation(n types.Node, commit string) contract.Citation {
	return contract.Citation{
		File:       n.FilePath,
		StartLine:  n.StartLine,
		EndLine:    n.EndLine,
		CommitHash: commit,
	}
}

// matchQname picks the qname from candidates that best matches the
// citation's line range. Exact start+end match wins; otherwise the
// first candidate fully containing the citation's range is used.
func matchQname(cands []types.Node, src contract.Citation) string {
	for _, n := range cands {
		if n.StartLine == src.StartLine && n.EndLine == src.EndLine {
			return n.QualifiedName
		}
	}
	for _, n := range cands {
		if n.StartLine <= src.StartLine && n.EndLine >= src.EndLine {
			return n.QualifiedName
		}
	}
	return ""
}

// nodeMatchesKinds reports whether n.Type matches any of the lowercase
// kind strings in kinds. Empty kinds means "any kind".
func nodeMatchesKinds(n types.Node, kinds []string) bool {
	if len(kinds) == 0 {
		return true
	}
	t := strings.ToLower(string(n.Type))
	for _, k := range kinds {
		if strings.ToLower(k) == t {
			return true
		}
	}
	return false
}

// planTraversal converts cks Relations into the (reverse, edgeTypes)
// arguments ckg's NeighborhoodByQname takes. Mixed direction is rejected
// because ckg's API only supports a single direction per call.
//
// Edge-type mapping:
//
//	cks RelationCalls    -> ckg "calls", "invokes"
//	cks RelationCalledBy -> ckg "calls", "invokes" with reverse=true
//	cks RelationImplements -> ckg "implements"
//	cks RelationImports    -> ckg "imports"
//	cks RelationReferences -> ckg "references"
//	cks RelationTestedBy   -> ckg has no direct edge; not supported in C.1
//	cks RelationEmbeds     -> ckg "extends"   (closest analog)
//	cks RelationDefines    -> ckg "defines"
//
// Empty input means "all forward relations": cks fans out to every
// supported forward edge type.
func planTraversal(rels []contract.Relation) (bool, []string, error) {
	if len(rels) == 0 {
		return false, []string{
			string(types.EdgeCalls), string(types.EdgeInvokes),
			string(types.EdgeImplements), string(types.EdgeImports),
			string(types.EdgeReferences), string(types.EdgeExtends),
			string(types.EdgeDefines),
		}, nil
	}

	// Detect mixed direction.
	anyReverse, anyForward := false, false
	for _, r := range rels {
		if r == contract.RelationCalledBy {
			anyReverse = true
		} else {
			anyForward = true
		}
	}
	if anyReverse && anyForward {
		return false, nil, errors.New("ckgclient: mixed forward + reverse relations in one call")
	}
	reverse := anyReverse

	seen := make(map[string]struct{})
	out := make([]string, 0, len(rels)*2)
	add := func(e types.EdgeType) {
		k := string(e)
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	for _, r := range rels {
		switch r {
		case contract.RelationCalls, contract.RelationCalledBy:
			add(types.EdgeCalls)
			add(types.EdgeInvokes)
		case contract.RelationImplements:
			add(types.EdgeImplements)
		case contract.RelationImports:
			add(types.EdgeImports)
		case contract.RelationReferences:
			add(types.EdgeReferences)
		case contract.RelationEmbeds:
			add(types.EdgeExtends)
		case contract.RelationDefines:
			add(types.EdgeDefines)
		case contract.RelationTestedBy:
			// ckg currently has no direct tested_by edge; skip rather
			// than fabricate a misleading mapping.
		default:
			return false, nil, fmt.Errorf("ckgclient: unknown relation %q", r)
		}
	}
	return reverse, out, nil
}

// relationFromEdgeType maps a ckg edge type to a cks Relation, accounting
// for traversal direction. Returns (_, false) when the edge type has no
// cks analog so the caller can drop the edge.
func relationFromEdgeType(et types.EdgeType, reversed bool) (contract.Relation, bool) {
	switch et {
	case types.EdgeCalls, types.EdgeInvokes:
		if reversed {
			return contract.RelationCalledBy, true
		}
		return contract.RelationCalls, true
	case types.EdgeImplements:
		return contract.RelationImplements, true
	case types.EdgeImports:
		return contract.RelationImports, true
	case types.EdgeReferences:
		return contract.RelationReferences, true
	case types.EdgeExtends:
		return contract.RelationEmbeds, true
	case types.EdgeDefines:
		return contract.RelationDefines, true
	}
	return "", false
}
