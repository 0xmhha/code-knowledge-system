package budget

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// FilesystemFetcher implements BodyFetcher by reading the cited
// line range directly from disk.
//
// Pairing with ckg: ckg's NodesByFilePath / Citation produces relative
// file paths against the indexed source root. FilesystemFetcher
// resolves those against the Root directory configured here.
//
// Snapshot consistency: this fetcher reads the working tree, not the
// indexed snapshot. If the tree has diverged from the index (commit
// hash differs from Citation.CommitHash) the body returned may not
// match what was indexed. The fetcher does NOT enforce this — cks's
// EvidencePack already carries CommitHash so the consuming LLM can
// detect drift if it matters. A strict-mode follow-up could add a
// match check; for Phase-0 dogfood the working tree IS the snapshot.
//
// Safety: paths are joined with filepath.Join and rejected outside
// Root via filepath.Rel + ".." prefix check, so a citation with a
// "../../etc/passwd" file path cannot escape the configured root.
type FilesystemFetcher struct {
	// Root is the directory that citation.File paths are resolved
	// against. When Citation.File is absolute, Root is ignored.
	// Empty Root keeps citation.File untouched (used by tests).
	Root string

	// DocsRoots are additional roots tried (in order, after Root) for
	// citations whose file does not exist under Root — the `ckv build
	// --docs` corpus dirs (manifest.DocsRoots). Doc/markdown chunks cite
	// files relative to a corpus dir, not the code Root, so without this
	// they have no body and Stage 4 skips them. Same escape protection as
	// Root applies to each.
	DocsRoots []string
}

// Fetch implements BodyFetcher. Returns ("", nil) for any "body
// genuinely unavailable" outcome (missing file in every root,
// out-of-range line span, escape attempt) — Stage 4 treats those as
// skip-not-error per the BodyFetcher contract.
func (f *FilesystemFetcher) Fetch(_ context.Context, c contract.Citation) (string, error) {
	roots := append([]string{f.Root}, f.DocsRoots...)
	if filepath.IsAbs(c.File) {
		roots = roots[:1] // absolute path ignores root; a single attempt suffices
	}
	for _, root := range roots {
		path, ok := resolveUnder(root, c.File)
		if !ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // try the next root (e.g. a docs corpus)
			}
			return "", fmt.Errorf("budget: read %q: %w", path, err)
		}
		return extractLines(string(data), c.StartLine, c.EndLine), nil
	}
	return "", nil
}

// resolveUnder translates citation.File into an absolute path under root
// and rejects any path that would escape it. The second return is false
// when the path is rejected; the caller treats that as "not in this root".
func resolveUnder(root, file string) (string, bool) {
	if filepath.IsAbs(file) {
		return filepath.Clean(file), true
	}
	if root == "" {
		return filepath.Clean(file), true
	}
	joined := filepath.Join(root, file)
	rel, err := filepath.Rel(root, joined)
	if err != nil {
		return "", false
	}
	// Reject any path that climbs above root via .. segments.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return joined, true
}

// extractLines slices text by 1-based inclusive line range. Returns
// "" for any out-of-range / inverted range request — per the
// BodyFetcher contract that signals "body genuinely unavailable" to
// Stage 4, which will skip the citation cleanly.
//
// Line splitting: \n only. CRLF is preserved (the \r becomes part of
// the previous line's content). Source code returned should match
// the user's editor view; mismatches on CRLF files are rare in
// cks-indexed codebases (typically Go which mandates \n).
func extractLines(text string, start, end int) string {
	if start < 1 || end < start {
		return ""
	}
	lines := strings.Split(text, "\n")
	if start > len(lines) {
		return ""
	}
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start-1:end], "\n")
}
