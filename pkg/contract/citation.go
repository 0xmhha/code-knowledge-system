package contract

import "fmt"

// Citation is a canonical reference to a code location. Shape matches ckg
// and ckv so results from either backend merge into a single EvidencePack
// without re-keying.
//
// StartLine and EndLine are 1-based and inclusive (matches editor and
// standard `git blame -L` convention). EndLine may equal StartLine for a
// single-line citation. CommitHash is the full 40-char SHA of the commit
// that produced the snapshot the indexer was run against; empty string is
// allowed (rare; usually indicates a stale index) but always present in
// JSON output for explicit consistency.
type Citation struct {
	File       string `json:"file"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	CommitHash string `json:"commit_hash"`
}

// String renders the citation as `path:start-end[@commit]`, the same form
// used by ckg's evidence reports and by go-stablenet's review skill.
func (c Citation) String() string {
	if c.CommitHash == "" {
		if c.StartLine == c.EndLine {
			return fmt.Sprintf("%s:%d", c.File, c.StartLine)
		}
		return fmt.Sprintf("%s:%d-%d", c.File, c.StartLine, c.EndLine)
	}
	if c.StartLine == c.EndLine {
		return fmt.Sprintf("%s:%d@%s", c.File, c.StartLine, c.CommitHash)
	}
	return fmt.Sprintf("%s:%d-%d@%s", c.File, c.StartLine, c.EndLine, c.CommitHash)
}

// IsValid reports whether c carries a non-empty File and a sane line range.
// Empty CommitHash is tolerated (see field doc).
func (c Citation) IsValid() bool {
	if c.File == "" {
		return false
	}
	if c.StartLine <= 0 || c.EndLine <= 0 {
		return false
	}
	return c.StartLine <= c.EndLine
}

// Key returns a stable identifier suitable for map deduplication of
// citations. Two citations with the same File/StartLine/EndLine collapse to
// one key regardless of CommitHash, because cks typically merges results
// from a single index snapshot (one CommitHash); when commits diverge the
// caller is expected to bucket by commit explicitly.
func (c Citation) Key() string {
	return fmt.Sprintf("%s:%d-%d", c.File, c.StartLine, c.EndLine)
}
