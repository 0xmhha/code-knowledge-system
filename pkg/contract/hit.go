package contract

// HitSource identifies which backend produced (or, after fusion, dominated)
// a Hit. The composer attaches this so downstream consumers (evaluation
// metrics, audit log) can attribute results back to ckg vs ckv vs the
// fused output.
type HitSource string

const (
	HitSourceCKG   HitSource = "ckg"
	HitSourceCKV   HitSource = "ckv"
	HitSourceFused HitSource = "fused"
)

// Hit is one search result with its post-fusion ranking and score.
//
// Rank is 1-based and reflects position in the fused list (or in the
// originating backend's list when Source is not "fused"). Score is the
// composer's fused score — for RRF fusion this is the sum of reciprocal
// ranks, not directly comparable to backend-native scores (BM25, cosine).
// For backend-attributed hits (Source = ckg or ckv) Score is the
// backend's native score passed through unchanged.
//
// Hits do not carry the matched code body; that lives in EvidencePack.Bodies
// keyed by Citation, so a single body can serve many hits.
//
// Symbol, CKGNodeID, and CanonicalID are populated only for HitSourceCKV hits
// when the underlying ckv chunk carries them. They are the bridge to ckg:
// composer Stage 1 extracts Symbol (not just the file basename) as a candidate
// keyword for Stage 2's ckg fan-out; CKGNodeID is ckg's positional node id
// (sha of qname|lang|startByte — build-specific); CanonicalID is ckg's
// import-path-qualified symbol id (ADR-0001) inherited by the chunk — the
// build-stable B7 join key that resolves via ckg FindByCanonicalID / the
// canonical-first FindSymbol path. Prefer CanonicalID for joins; CKGNodeID
// only pins a node within one specific graph build.
// All three are omitempty for backward compatibility with hits that
// pre-date the alignment work.
type Hit struct {
	Citation    Citation  `json:"citation"`
	Rank        int       `json:"rank"`
	Score       float64   `json:"score"`
	Source      HitSource `json:"source,omitempty"`
	Symbol      string    `json:"symbol,omitempty"`
	CKGNodeID   string    `json:"ckg_node_id,omitempty"`
	CanonicalID string    `json:"canonical_id,omitempty"`
}

// IsValid reports whether h carries a valid Citation and a sane Rank.
// Score is unconstrained (negative scores are valid in some fusion schemes).
func (h Hit) IsValid() bool {
	return h.Citation.IsValid() && h.Rank >= 1
}
