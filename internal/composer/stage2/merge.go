package stage2

import (
	"fmt"
	"sort"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// ScoredCitation is one citation accompanied by its accumulated evidence.
//
// Score is the sum across all evidence: each BM25 hit contributes its
// native score; each FindSymbol hit contributes Config.SymbolBonus.
//
// Sources is a human-readable evidence trail of the form
// "bm25:<keyword>=<score>" or "symbol:<keyword>=+<bonus>". Preserved so
// the audit log and Phase E evaluation can answer "why did this citation
// score this high" without rerunning the search.
type ScoredCitation struct {
	Citation contract.Citation
	Score    float64
	Sources  []string
}

// aggregator collects per-citation evidence as Stage 2 walks each
// keyword. Final results() drains the map into a sorted, capped slice.
type aggregator struct {
	byCitation  map[string]*ScoredCitation
	symbolBonus float64
}

func newAggregator(symbolBonus float64) *aggregator {
	return &aggregator{
		byCitation:  make(map[string]*ScoredCitation),
		symbolBonus: symbolBonus,
	}
}

// addBM25Hit credits the BM25 score to the citation's running total and
// records the evidence in Sources.
func (a *aggregator) addBM25Hit(keyword string, h contract.Hit) {
	sc := a.entry(h.Citation)
	sc.Score += h.Score
	sc.Sources = append(sc.Sources, fmt.Sprintf("bm25:%s=%.2f", keyword, h.Score))
}

// addSymbolHit credits the symbol bonus to the citation's running total.
func (a *aggregator) addSymbolHit(keyword string, c contract.Citation) {
	sc := a.entry(c)
	sc.Score += a.symbolBonus
	sc.Sources = append(sc.Sources, fmt.Sprintf("symbol:%s=+%.1f", keyword, a.symbolBonus))
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
