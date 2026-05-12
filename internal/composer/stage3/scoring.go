package stage3

import (
	"sort"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// neighborAggregator collects ScoredNeighbors keyed by Target Citation.
// When multiple seeds reach the same target the running Score is updated
// to the MAX of competing values — the closest path wins.
//
// Why max (not sum): graph hubs (e.g., a logging utility called from
// hundreds of sites) would dominate a sum-based score regardless of
// semantic relevance. Max preserves "the best evidence we have for this
// target" without over-weighting popularity.
type neighborAggregator struct {
	byTarget map[string]*ScoredNeighbor
}

func newNeighborAggregator() *neighborAggregator {
	return &neighborAggregator{
		byTarget: make(map[string]*ScoredNeighbor),
	}
}

// add registers an edge with the given derived score. On duplicate
// Target (multi-path), keeps the higher score and updates Edge to the
// closer-path edge; the provenance string is always appended.
func (a *neighborAggregator) add(n contract.Neighbor, score float64, source string) {
	key := n.Target.Key()
	existing, ok := a.byTarget[key]
	if !ok {
		a.byTarget[key] = &ScoredNeighbor{
			Edge:    n,
			Score:   score,
			Sources: []string{source},
		}
		return
	}
	existing.Sources = append(existing.Sources, source)
	if score > existing.Score {
		existing.Score = score
		// Update the canonical Edge to reflect the higher-scoring path
		// (closer hop, larger seed score, or both).
		existing.Edge = n
	}
}

// results returns the accumulated neighbors sorted by descending Score,
// with file/start-line tiebreaker for deterministic output. cap > 0
// truncates to that length.
func (a *neighborAggregator) results(cap int) []ScoredNeighbor {
	if len(a.byTarget) == 0 {
		return nil
	}
	out := make([]ScoredNeighbor, 0, len(a.byTarget))
	for _, sn := range a.byTarget {
		out = append(out, *sn)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Edge.Target.File != out[j].Edge.Target.File {
			return out[i].Edge.Target.File < out[j].Edge.Target.File
		}
		return out[i].Edge.Target.StartLine < out[j].Edge.Target.StartLine
	})
	if cap > 0 && len(out) > cap {
		out = out[:cap]
	}
	return out
}

// sortStrings is a tiny wrapper so the searcher can sort relation-type
// strings without importing "sort" twice. Keeping it here colocates the
// sorting helpers.
func sortStrings(s []string) { sort.Strings(s) }
