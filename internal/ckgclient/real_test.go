package ckgclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-graph/pkg/types"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- mockStoreReader ---
//
// Stands in for the production storeReaderAdapter. Tests poke the
// canned outputs / errors then call Real's Client methods to inspect
// the translation layer.

type mockStoreReader struct {
	manifest    ManifestSnapshot
	manifestErr error

	searchOut []types.Node
	searchErr error
	searchCh  []searchCall

	symbolOut []types.Node
	symbolErr error
	symbolCh  []symbolCall

	neighOut   []types.Node
	neighEdges []types.Edge
	neighErr   error
	neighCh    []neighCall

	// pathNodes is returned by every NodesByFilePath call regardless of
	// the path argument; tests set this to the node that should resolve
	// for the citation under test.
	pathNodes []types.Node
	pathErr   error
	pathCh    []string

	closed   bool
	closeErr error
}

type searchCall struct {
	q     string
	limit int
}
type symbolCall struct {
	name  string
	lang  string
	exact bool
}
type neighCall struct {
	qname  string
	depth  int
	rev    bool
	etypes []string
}

func (m *mockStoreReader) LoadManifestSnapshot() (ManifestSnapshot, error) {
	if m.manifestErr != nil {
		return ManifestSnapshot{}, m.manifestErr
	}
	return m.manifest, nil
}
func (m *mockStoreReader) SearchFTS(q string, limit int) ([]types.Node, error) {
	m.searchCh = append(m.searchCh, searchCall{q: q, limit: limit})
	return m.searchOut, m.searchErr
}
func (m *mockStoreReader) FindSymbol(name, lang string, exact bool) ([]types.Node, error) {
	m.symbolCh = append(m.symbolCh, symbolCall{name: name, lang: lang, exact: exact})
	return m.symbolOut, m.symbolErr
}
func (m *mockStoreReader) NeighborhoodByQname(qname string, depth int, reverse bool, edgeTypes ...string) ([]types.Node, []types.Edge, error) {
	m.neighCh = append(m.neighCh, neighCall{qname: qname, depth: depth, rev: reverse, etypes: edgeTypes})
	return m.neighOut, m.neighEdges, m.neighErr
}
func (m *mockStoreReader) NodesByFilePath(path string) ([]types.Node, error) {
	m.pathCh = append(m.pathCh, path)
	return m.pathNodes, m.pathErr
}
func (m *mockStoreReader) SubgraphByQname(qname string, depth int) ([]types.Node, []types.Edge, error) {
	return m.neighOut, m.neighEdges, m.neighErr
}
func (m *mockStoreReader) Close() error {
	m.closed = true
	return m.closeErr
}

// --- helpers ---

func node(id, qname, file string, start, end int, typ types.NodeType, lang string) types.Node {
	return types.Node{
		ID:            id,
		Type:          typ,
		Name:          qname,
		QualifiedName: qname,
		FilePath:      file,
		StartLine:     start,
		EndLine:       end,
		Language:      lang,
		Confidence:    types.ConfExtracted,
	}
}

func edge(src, dst string, t types.EdgeType) types.Edge {
	return types.Edge{Src: src, Dst: dst, Type: t, Count: 1, Confidence: types.ConfExtracted}
}

// --- BM25Search ---

func TestReal_BM25Search_TranslatesNodesToHits(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{
		manifest: ManifestSnapshot{SrcCommit: "abc123"},
		searchOut: []types.Node{
			node("nid1", "pkg.A", "a.go", 10, 30, types.NodeFunction, "go"),
			node("nid2", "pkg.B", "b.go", 5, 25, types.NodeMethod, "go"),
		},
	}
	r := newRealWithStore(m)

	hits, err := r.BM25Search(context.Background(), "find login", SearchOpts{K: 7})
	if err != nil {
		t.Fatalf("BM25Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d", len(hits))
	}
	// Forwarded limit: K=7 should reach the backend as limit=7.
	if got := m.searchCh[0].limit; got != 7 {
		t.Errorf("SearchFTS limit = %d, want 7", got)
	}
	// Citation translation: CommitHash from manifest, File/Start/End from node.
	h0 := hits[0]
	if h0.Citation.File != "a.go" || h0.Citation.StartLine != 10 || h0.Citation.EndLine != 30 {
		t.Errorf("Citation = %+v, want a.go:10-30", h0.Citation)
	}
	if h0.Citation.CommitHash != "abc123" {
		t.Errorf("CommitHash = %q, want abc123 (from manifest)", h0.Citation.CommitHash)
	}
	if h0.Source != contract.HitSourceCKG {
		t.Errorf("Source = %q, want HitSourceCKG", h0.Source)
	}
	// Synthetic score: descending 1 -> 1/N.
	if !(h0.Score > hits[1].Score) {
		t.Errorf("scores not descending: %v vs %v", h0.Score, hits[1].Score)
	}
	if h0.Rank != 1 || hits[1].Rank != 2 {
		t.Errorf("Rank = %d,%d want 1,2", h0.Rank, hits[1].Rank)
	}
}

func TestReal_BM25Search_EmptyQueryErrors(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{}
	r := newRealWithStore(m)
	if _, err := r.BM25Search(context.Background(), "", SearchOpts{}); err == nil {
		t.Fatal("expected error on empty query")
	}
	if len(m.searchCh) != 0 {
		t.Errorf("backend should not be called on empty query, got %d calls", len(m.searchCh))
	}
}

func TestReal_BM25Search_DefaultsZeroKToBackendDefault(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{manifest: ManifestSnapshot{SrcCommit: "h"}}
	r := newRealWithStore(m)
	if _, err := r.BM25Search(context.Background(), "q", SearchOpts{K: 0}); err != nil {
		t.Fatal(err)
	}
	if got := m.searchCh[0].limit; got != DefaultSearchLimit {
		t.Errorf("limit = %d, want DefaultSearchLimit (%d)", got, DefaultSearchLimit)
	}
}

func TestReal_BM25Search_PathGlobPostFilter(t *testing.T) {
	t.Parallel()
	// SearchFTS returns a mix of test and production files; PathGlob
	// "*_test.go" must keep only the test rows. The over-fetch ratio
	// is exercised because the filter discards rows: we ask for K=2,
	// so the backend should be hit with K * FilterOverfetchRatio.
	m := &mockStoreReader{
		manifest: ManifestSnapshot{SrcCommit: "h"},
		searchOut: []types.Node{
			node("n1", "Foo", "a.go", 1, 5, types.NodeFunction, "go"),
			node("n2", "TestFoo", "a_test.go", 10, 20, types.NodeFunction, "go"),
			node("n3", "Bar", "b.go", 1, 5, types.NodeFunction, "go"),
			node("n4", "TestBar", "b_test.go", 10, 20, types.NodeFunction, "go"),
		},
	}
	r := newRealWithStore(m)
	hits, err := r.BM25Search(context.Background(), "q",
		SearchOpts{K: 2, Filter: SearchFilter{PathGlob: "*_test.go"}})
	if err != nil {
		t.Fatalf("BM25Search: %v", err)
	}
	if got := m.searchCh[0].limit; got != 2*FilterOverfetchRatio {
		t.Errorf("backend limit = %d, want %d (K * FilterOverfetchRatio)", got, 2*FilterOverfetchRatio)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	for _, h := range hits {
		if !strings.HasSuffix(h.Citation.File, "_test.go") {
			t.Errorf("non-test file leaked through filter: %+v", h.Citation)
		}
	}
}

func TestReal_BM25Search_LanguageFilter(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{
		manifest: ManifestSnapshot{SrcCommit: "h"},
		searchOut: []types.Node{
			node("n1", "Foo", "a.go", 1, 5, types.NodeFunction, "go"),
			node("n2", "Bar", "b.ts", 1, 5, types.NodeFunction, "ts"),
		},
	}
	r := newRealWithStore(m)
	hits, err := r.BM25Search(context.Background(), "q",
		SearchOpts{K: 5, Filter: SearchFilter{Language: "ts"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Citation.File != "b.ts" {
		t.Errorf("got %v, want only b.ts", hits)
	}
}

func TestReal_BM25Search_NoFilterKeepsExactLimit(t *testing.T) {
	t.Parallel()
	// Without a filter, the backend limit must equal K — no over-fetch.
	m := &mockStoreReader{
		manifest: ManifestSnapshot{SrcCommit: "h"},
		searchOut: []types.Node{
			node("n1", "A", "a.go", 1, 5, types.NodeFunction, "go"),
		},
	}
	r := newRealWithStore(m)
	if _, err := r.BM25Search(context.Background(), "q", SearchOpts{K: 7}); err != nil {
		t.Fatal(err)
	}
	if got := m.searchCh[0].limit; got != 7 {
		t.Errorf("backend limit = %d, want 7 (no over-fetch without filter)", got)
	}
}

func TestReal_BM25Search_BackendErrorPropagates(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{searchErr: errors.New("fts down")}
	r := newRealWithStore(m)
	_, err := r.BM25Search(context.Background(), "q", SearchOpts{K: 5})
	if err == nil || err.Error() == "" {
		t.Fatalf("err = %v, want wrapped backend error", err)
	}
}

// --- FindSymbol ---

func TestReal_FindSymbol_NoFilterReturnsAll(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{
		manifest: ManifestSnapshot{SrcCommit: "c"},
		symbolOut: []types.Node{
			node("n1", "pkg.A", "a.go", 1, 5, types.NodeFunction, "go"),
			node("n2", "pkg.B", "b.go", 1, 5, types.NodeMethod, "go"),
			node("n3", "pkg.C", "c.go", 1, 5, types.NodeStruct, "go"),
		},
	}
	r := newRealWithStore(m)
	cs, err := r.FindSymbol(context.Background(), "A", SymbolOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 {
		t.Errorf("want 3 citations, got %d", len(cs))
	}
	if m.symbolCh[0].name != "A" {
		t.Errorf("FindSymbol forwarded name = %q, want \"A\"", m.symbolCh[0].name)
	}
}

func TestReal_FindSymbol_KindsFilterClientSide(t *testing.T) {
	t.Parallel()
	// Backend returns 3; we only want function + method per opts.Kinds.
	// Struct must be filtered out.
	m := &mockStoreReader{
		manifest: ManifestSnapshot{SrcCommit: "c"},
		symbolOut: []types.Node{
			node("n1", "pkg.A", "a.go", 1, 5, types.NodeFunction, "go"),
			node("n2", "pkg.B", "b.go", 1, 5, types.NodeMethod, "go"),
			node("n3", "pkg.C", "c.go", 1, 5, types.NodeStruct, "go"),
		},
	}
	r := newRealWithStore(m)
	cs, err := r.FindSymbol(context.Background(), "X", SymbolOpts{Kinds: []string{"function", "method"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("want 2 after Kinds filter, got %d", len(cs))
	}
	for _, c := range cs {
		if c.File == "c.go" {
			t.Errorf("Struct leaked past Kinds filter: %+v", c)
		}
	}
}

func TestReal_FindSymbol_UsesLanguageFilter(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{manifest: ManifestSnapshot{SrcCommit: "c"}}
	r := newRealWithStore(m)
	_, _ = r.FindSymbol(context.Background(), "X", SymbolOpts{})
	if m.symbolCh[0].lang != "" {
		t.Errorf("lang = %q, want empty (no filter)", m.symbolCh[0].lang)
	}
	// Note: cks SymbolOpts has no Language field today (only Kinds/PathGlob/
	// CommitHash). Real.FindSymbol passes "" as the ckg language argument
	// which means "any language". This guards against regression.
}

// --- Neighbors ---

func TestReal_Neighbors_TranslatesEdgesToCksRelations(t *testing.T) {
	t.Parallel()
	src := contract.Citation{File: "src.go", StartLine: 10, EndLine: 30, CommitHash: "abc"}
	srcNode := node("S", "pkg.Src", "src.go", 10, 30, types.NodeFunction, "go")
	dstNode := node("D", "pkg.Dst", "dst.go", 1, 5, types.NodeFunction, "go")
	m := &mockStoreReader{
		manifest:   ManifestSnapshot{SrcCommit: "abc"},
		pathNodes:  []types.Node{srcNode}, // qname lookup resolves src -> "pkg.Src"
		neighOut:   []types.Node{srcNode, dstNode},
		neighEdges: []types.Edge{edge("S", "D", types.EdgeCalls)},
	}
	r := newRealWithStore(m)
	out, err := r.Neighbors(context.Background(), src, NeighborsOpts{Hops: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 neighbor, got %d", len(out))
	}
	// Confirm qname resolution went through NodesByFilePath first.
	if len(m.pathCh) != 1 || m.pathCh[0] != "src.go" {
		t.Errorf("NodesByFilePath calls = %v, want [src.go]", m.pathCh)
	}
	if len(m.neighCh) != 1 || m.neighCh[0].qname != "pkg.Src" {
		t.Errorf("NeighborhoodByQname qname = %q, want pkg.Src", m.neighCh[0].qname)
	}
	n := out[0]
	if n.Relation != contract.RelationCalls {
		t.Errorf("Relation = %q, want %q", n.Relation, contract.RelationCalls)
	}
	if n.Source.File != "src.go" || n.Target.File != "dst.go" {
		t.Errorf("endpoints wrong: %+v", n)
	}
	if n.Distance != 1 {
		t.Errorf("Distance = %d, want 1", n.Distance)
	}
}

func TestReal_Neighbors_DropsUntranslatableEdges(t *testing.T) {
	t.Parallel()
	// ckg has many edge types that cks's RelationXxx vocabulary does not
	// cover (uses_type, reads_field, listens_on, etc.). Real should DROP
	// those rather than fabricate a Relation that downstream consumers
	// would mis-classify.
	src := contract.Citation{File: "s.go", StartLine: 1, EndLine: 5, CommitHash: "h"}
	srcNode := node("S", "pkg.S", "s.go", 1, 5, types.NodeFunction, "go")
	dstNode := node("D", "pkg.D", "d.go", 1, 5, types.NodeStruct, "go")
	m := &mockStoreReader{
		manifest:  ManifestSnapshot{SrcCommit: "h"},
		pathNodes: []types.Node{srcNode},
		neighOut:  []types.Node{srcNode, dstNode},
		neighEdges: []types.Edge{
			edge("S", "D", types.EdgeUsesType),   // drop
			edge("S", "D", types.EdgeReadsField), // drop
			edge("S", "D", types.EdgeCalls),      // keep
		},
	}
	r := newRealWithStore(m)
	out, err := r.Neighbors(context.Background(), src, NeighborsOpts{Hops: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly the EdgeCalls neighbor, got %d", len(out))
	}
	if out[0].Relation != contract.RelationCalls {
		t.Errorf("Relation = %q, want calls", out[0].Relation)
	}
}

func TestReal_Neighbors_CalledByReversesDirection(t *testing.T) {
	t.Parallel()
	// RelationCalledBy in cks's vocabulary is the reverse of RelationCalls.
	// ckg expresses this via NeighborhoodByQname's `reverse` bool argument.
	src := contract.Citation{File: "x.go", StartLine: 1, EndLine: 5, CommitHash: "h"}
	srcNode := node("X", "pkg.X", "x.go", 1, 5, types.NodeFunction, "go")
	m := &mockStoreReader{
		manifest:  ManifestSnapshot{SrcCommit: "h"},
		pathNodes: []types.Node{srcNode},
		neighOut:  []types.Node{srcNode},
	}
	r := newRealWithStore(m)
	if _, err := r.Neighbors(context.Background(), src, NeighborsOpts{
		Hops:      1,
		Relations: []contract.Relation{contract.RelationCalledBy},
	}); err != nil {
		t.Fatal(err)
	}
	// Two calls expected — cks's "called_by" maps to ckg's reverse-direction
	// traversal of "calls" + "invokes".
	if len(m.neighCh) != 1 {
		t.Fatalf("expected 1 ckg call, got %d", len(m.neighCh))
	}
	if !m.neighCh[0].rev {
		t.Errorf("reverse should be true for called_by")
	}
}

// --- Health ---

func TestReal_Health_OK(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{
		manifest: ManifestSnapshot{SchemaVersion: "1.8", SrcCommit: "deadbeef"},
	}
	r := newRealWithStore(m)
	h, err := r.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !h.Reachable {
		t.Error("Reachable should be true on successful manifest read")
	}
	if h.SchemaVersion != "1.8" {
		t.Errorf("SchemaVersion = %q, want 1.8", h.SchemaVersion)
	}
	if h.IndexedHead != "deadbeef" {
		t.Errorf("IndexedHead = %q, want deadbeef", h.IndexedHead)
	}
}

func TestReal_Health_ManifestErrorPropagates(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{manifestErr: errors.New("db locked")}
	r := newRealWithStore(m)
	h, err := r.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if h.Reachable {
		t.Error("Reachable should be false on manifest error")
	}
}

// --- Close ---

func TestReal_Close_IsIdempotent(t *testing.T) {
	t.Parallel()
	m := &mockStoreReader{}
	r := newRealWithStore(m)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if !m.closed {
		t.Error("underlying Close not called")
	}
}

// --- Compile-time guarantee ---

func TestReal_ImplementsClient(t *testing.T) {
	t.Parallel()
	var _ Client = (*Real)(nil)
}
