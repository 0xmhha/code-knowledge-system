package stage2

import (
	"fmt"
	"sort"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// ScoredCitation is one citation accompanied by its accumulated evidence.
//
// Score is the Reciprocal Rank Fusion (RRF) total across every ranked list
// the citation appeared in:
//
//	Score = Σ (weight_i / (RRFK + rank_i))
//
// where each BM25Search result list and each FindSymbol result list (per
// keyword) is a separate ranked input. BM25 lists carry BMWeight (1.0 by
// default); FindSymbol lists carry SymbolWeight (1.5 by default — exact
// symbol matches outweigh keyword-only overlap by 50%). rank is 1-based.
//
// RRF is rank-only fusion (Cormack et al., 2009): backend score
// magnitudes never enter the merge, so BM25's wide score range stops
// dominating the SymbolBonus heuristic that the previous score-sum
// aggregator used.
//
// Sources is a human-readable evidence trail of the form
// "bm25:<keyword>@rank=<rank>(+<contribution>)" or
// "symbol:<keyword>@rank=<rank>(+<contribution>)". Preserved so the
// audit log and Phase E evaluation can answer "why did this citation
// score this high" without rerunning the search.
type ScoredCitation struct {
	Citation contract.Citation
	Score    float64
	Sources  []string
}

// DefaultRRFK is the RRF tuning constant from Cormack et al. (2009).
// 60 is the value that paper recommends and that most rank-fusion
// implementations adopt; smaller values give more weight to the head
// of each ranked list, larger values flatten the influence of rank.
const DefaultRRFK = 60

// aggregator collects per-citation evidence as Stage 2 walks each
// keyword. Final results() drains the map into a sorted, capped slice.
type aggregator struct {
	byCitation   map[string]*ScoredCitation
	rrfK         int
	bmWeight     float64
	symbolWeight float64
}

func newAggregator(rrfK int, bmWeight, symbolWeight float64) *aggregator {
	return &aggregator{
		byCitation:   make(map[string]*ScoredCitation),
		rrfK:         rrfK,
		bmWeight:     bmWeight,
		symbolWeight: symbolWeight,
	}
}

// addBM25List credits every hit in a ranked BM25 result list. rank is
// derived from list position (1-based). Empty lists are a no-op.
func (a *aggregator) addBM25List(keyword string, hits []contract.Hit) {
	for i, h := range hits {
		rank := i + 1
		contribution := a.bmWeight / float64(a.rrfK+rank)
		sc := a.entry(h.Citation)
		sc.Score += contribution
		sc.Sources = append(sc.Sources,
			fmt.Sprintf("bm25:%s@rank=%d(+%.5f)", keyword, rank, contribution))
	}
}

// addSymbolList credits every citation in a ranked FindSymbol result
// list. rank is 1-based by list position. Empty lists are a no-op.
func (a *aggregator) addSymbolList(keyword string, cits []contract.Citation) {
	for i, c := range cits {
		rank := i + 1
		contribution := a.symbolWeight / float64(a.rrfK+rank)
		sc := a.entry(c)
		sc.Score += contribution
		sc.Sources = append(sc.Sources,
			fmt.Sprintf("symbol:%s@rank=%d(+%.5f)", keyword, rank, contribution))
	}
}

func (a *aggregator) entry(c contract.Citation) *ScoredCitation {
	key := c.Key()
	sc, ok := a.byCitation[key]
	if !ok {
		sc = &ScoredCitation{Citation: c}
		a.byCitation[key] = sc
	}
	return sc
}

// results returns the accumulated citations sorted by descending Score.
// Ties are broken by File path for deterministic output (eval reports
// must reproduce). When cap > 0, the slice is truncated to that length.
func (a *aggregator) results(cap int) []ScoredCitation {
	if len(a.byCitation) == 0 {
		return nil
	}

	out := make([]ScoredCitation, 0, len(a.byCitation))
	for _, sc := range a.byCitation {
		out = append(out, *sc)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		// Stable tiebreaker: file path lexical order, then start line.
		if out[i].Citation.File != out[j].Citation.File {
			return out[i].Citation.File < out[j].Citation.File
		}
		return out[i].Citation.StartLine < out[j].Citation.StartLine
	})

	if cap > 0 && len(out) > cap {
		out = out[:cap]
	}
	return out
}
