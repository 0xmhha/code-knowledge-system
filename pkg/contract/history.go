package contract

import "time"

// PRRef is a reference to a pull request that modified a symbol or file.
type PRRef struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	BaseSHA   string    `json:"base_sha,omitempty"`
	HeadSHA   string    `json:"head_sha,omitempty"`
	MergedAt  time.Time `json:"merged_at,omitempty"`
	Repo      string    `json:"repo,omitempty"`
}

// HunkEvidence is one code change (hunk) relevant to an intent query,
// ranked by BM25 similarity.
type HunkEvidence struct {
	File      string  `json:"file"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Patch     string  `json:"patch,omitempty"`
	Score     float64 `json:"score"`
}

// ChangeHistoryResult is the response of a change-history query.
// It combines PR metadata with ranked hunk evidence.
type ChangeHistoryResult struct {
	Seed  string         `json:"seed"`
	PRs   []PRRef        `json:"prs,omitempty"`
	Hunks []HunkEvidence `json:"hunks,omitempty"`
}
