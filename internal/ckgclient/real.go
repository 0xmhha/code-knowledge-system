package ckgclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/0xmhha/code-knowledge-graph/pkg/store"
	"github.com/0xmhha/code-knowledge-graph/pkg/types"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// DefaultSearchLimit is the limit passed to ckg's SearchFTS when SearchOpts.K
// is zero. Mirrors ckg's typical default; tuned downward in callers that
// budget for fewer hits.
const DefaultSearchLimit = 10

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
	SearchFTS(q string, limit int) ([]types.Node, error)
	FindSymbol(name, lang string, exact bool) ([]types.Node, error)
	NodesByFilePath(path string) ([]types.Node, error)
	NeighborhoodByQname(qname string, depth int, reverse bool, edgeTypes ...string) ([]types.Node, []types.Edge, error)
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
func (a *realStoreReader) SearchFTS(q string, limit int) ([]types.Node, error) {
	return a.r.SearchFTS(q, limit)
}
func (a *realStoreReader) FindSymbol(name, lang string, exact bool) ([]types.Node, error) {
	return a.r.FindSymbol(name, lang, exact)
}
func (a *realStoreReader) NodesByFilePath(path string) ([]types.Node, error) {
	return a.r.NodesByFilePath(path)
}
func (a *realStoreReader) NeighborhoodByQname(qname string, depth int, reverse bool, edgeTypes ...string) ([]types.Node, []types.Edge, error) {
	return a.r.NeighborhoodByQname(qname, depth, reverse, edgeTypes...)
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
// Filter handling: opts.Filter (Language/PathGlob/CommitHash) is NOT
// forwarded to ckg.SearchFTS — that method takes only (query, limit).
// A pure-Go post-filter could be added in a follow-up; C.1 keeps the
// surface honest by ignoring the filter and documenting it here.
//
// Score policy: ckg's SearchFTS returns nodes in FTS-rank order but does
// not return a numeric score. cks Hit.Score is synthesized as
// `1 - i/(N+1)` (descending in [1/(N+1), 1)) so downstream stages that
// rely on relative score ordering still get a deterministic gradient.
func (r *Real) BM25Search(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	if query == "" {
		return nil, errors.New("ckgclient: empty query")
	}
	limit := opts.K
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	nodes, err := r.s.SearchFTS(query, limit)
	if err != nil {
		return nil, fmt.Errorf("ckgclient: SearchFTS: %w", err)
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	commit, _ := r.commit()
	hits := make([]contract.Hit, 0, len(nodes))
	n := len(nodes)
	for i, nd := range nodes {
		hits = append(hits, contract.Hit{
			Citation: nodeToCitation(nd, commit),
			Rank:     i + 1,
			Score:    1.0 - float64(i)/float64(n+1),
			Source:   contract.HitSourceCKG,
		})
	}
	return hits, nil
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
func (r *Real) FindSymbol(ctx context.Context, name string, opts SymbolOpts) ([]contract.Citation, error) {
	if name == "" {
		return nil, errors.New("ckgclient: empty symbol name")
	}
	nodes, err := r.s.FindSymbol(name, "", false)
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

	nodes, edges, err := r.s.NeighborhoodByQname(qname, depth, reverse, edgeTypes...)
	if err != nil {
		return nil, fmt.Errorf("ckgclient: NeighborhoodByQname: %w", err)
	}

	commit, _ := r.commit()
	byID := make(map[string]types.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

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
