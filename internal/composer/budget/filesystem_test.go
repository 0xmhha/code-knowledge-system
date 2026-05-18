package budget

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFilesystemFetcher_ReturnsCitedLineRange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "a/b.go", "line1\nline2\nline3\nline4\nline5\n")

	f := &FilesystemFetcher{Root: root}
	got, err := f.Fetch(context.Background(), contract.Citation{
		File: "a/b.go", StartLine: 2, EndLine: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "line2\nline3\nline4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilesystemFetcher_SingleLine(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "x.go", "only-line")
	f := &FilesystemFetcher{Root: root}
	got, err := f.Fetch(context.Background(), contract.Citation{
		File: "x.go", StartLine: 1, EndLine: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "only-line" {
		t.Errorf("got %q, want \"only-line\"", got)
	}
}

func TestFilesystemFetcher_MissingFileReturnsEmpty(t *testing.T) {
	t.Parallel()
	// Per BodyFetcher contract: missing file -> ("", nil), so Stage 4
	// can skip the citation without aborting the whole allocation.
	root := t.TempDir()
	f := &FilesystemFetcher{Root: root}
	got, err := f.Fetch(context.Background(), contract.Citation{
		File: "no-such.go", StartLine: 1, EndLine: 5,
	})
	if err != nil {
		t.Fatalf("missing file should be silent: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFilesystemFetcher_OutOfRangeReturnsEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "x.go", "a\nb\nc")
	f := &FilesystemFetcher{Root: root}

	cases := []struct {
		name       string
		start, end int
	}{
		{"zero start", 0, 1},
		{"negative", -1, 2},
		{"inverted", 5, 2},
		{"both past EOF", 10, 12},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := f.Fetch(context.Background(), contract.Citation{
				File: "x.go", StartLine: tc.start, EndLine: tc.end,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got != "" {
				t.Errorf("got %q, want empty", got)
			}
		})
	}
}

func TestFilesystemFetcher_EndPastEOFClamped(t *testing.T) {
	t.Parallel()
	// end > line count is clamped to EOF (not rejected) — the citation
	// is "mostly valid" and the tail is what's available.
	root := t.TempDir()
	writeFile(t, root, "x.go", "a\nb\nc")
	f := &FilesystemFetcher{Root: root}
	got, err := f.Fetch(context.Background(), contract.Citation{
		File: "x.go", StartLine: 2, EndLine: 99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "b\nc" {
		t.Errorf("got %q, want \"b\\nc\"", got)
	}
}

func TestFilesystemFetcher_RejectsPathEscape(t *testing.T) {
	t.Parallel()
	// Citation with "../../etc/passwd" must not escape Root. Returns
	// ("", nil) so the citation is just skipped.
	root := t.TempDir()
	writeFile(t, root, "ok.go", "line1\nline2")
	// place a sibling file that escape would hit
	parent := filepath.Dir(root)
	writeFile(t, parent, "secret.txt", "ESCAPED")

	f := &FilesystemFetcher{Root: root}
	got, err := f.Fetch(context.Background(), contract.Citation{
		File: "../secret.txt", StartLine: 1, EndLine: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("path escape leaked: %q", got)
	}
	if strings.Contains(got, "ESCAPED") {
		t.Error("escape contents reached caller")
	}
}

func TestFilesystemFetcher_AbsolutePathIgnoresRoot(t *testing.T) {
	t.Parallel()
	// When citation.File is absolute, Root is ignored — useful when
	// the indexed source root differs from where cks-mcp runs.
	dir := t.TempDir()
	abs := writeFile(t, dir, "abs.go", "abs-line-1\nabs-line-2")
	f := &FilesystemFetcher{Root: "/some/unrelated/dir"}
	got, err := f.Fetch(context.Background(), contract.Citation{
		File: abs, StartLine: 1, EndLine: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "abs-line-1" {
		t.Errorf("got %q", got)
	}
}

func TestFilesystemFetcher_EmptyRootKeepsRelative(t *testing.T) {
	t.Parallel()
	// Root="" makes the fetcher resolve relative paths against cwd.
	// Useful for cks-mcp invoked from the repo root.
	prev, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	writeFile(t, dir, "rel.go", "rel-1\nrel-2")

	f := &FilesystemFetcher{} // Root empty
	got, err := f.Fetch(context.Background(), contract.Citation{
		File: "rel.go", StartLine: 1, EndLine: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "rel-1\nrel-2" {
		t.Errorf("got %q", got)
	}
}

// Compile-time guarantee.
var _ BodyFetcher = (*FilesystemFetcher)(nil)
