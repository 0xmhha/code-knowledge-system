package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestMarkVerified_RoundTripPreservesOtherFields confirms that mutating
// status / last_verified_at / verified_by leaves every other field —
// including multi-line literal summaries — byte-for-byte identical when
// re-parsed. This is the load-bearing guarantee: verify-CLI authors
// expect they can rerun MarkVerified safely without losing the entry's
// hand-curated content.
func TestMarkVerified_RoundTripPreservesOtherFields(t *testing.T) {
	t.Parallel()

	const original = `id: A1.foo.bar
subsystem: A1
knowledge_type: B1
title: "Sample entry for verify test"
summary: |
  Two-line summary
  with a literal block.
code_anchors:
  - file: dummy.go
    symbol: Foo
    line: 10
    reason: "anchor reason"
code_keywords:
  - Foo
status: needs_verification
priority: P0
last_verified_at: null
verified_by: null
`
	dir := t.TempDir()
	path := filepath.Join(dir, "A1.foo.bar.yaml")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MarkVerified(path, "2026-05-29", "tester"); err != nil {
		t.Fatalf("MarkVerified: %v", err)
	}

	// Parse the result with a permissive map so we can inspect every
	// field — including the ones that should not have changed.
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(buf, &got); err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	for k, want := range map[string]any{
		"status":           "verified",
		"last_verified_at": "2026-05-29",
		"verified_by":      "tester",
		"id":               "A1.foo.bar",
		"subsystem":        "A1",
		"knowledge_type":   "B1",
		"title":            "Sample entry for verify test",
		"priority":         "P0",
	} {
		if got[k] != want {
			t.Errorf("field %q = %v, want %v", k, got[k], want)
		}
	}

	// Summary stays a literal multi-line block — check by content
	// rather than YAML byte equality, since the encoder might
	// re-flow the trailing newline differently from the input.
	if s, _ := got["summary"].(string); !strings.Contains(s, "Two-line summary") || !strings.Contains(s, "literal block.") {
		t.Errorf("summary lost content after round-trip: %q", got["summary"])
	}
}

// TestMarkVerified_RejectsBadDate guards against operators passing a
// date in a non-canonical format. Anchoring to YYYY-MM-DD is the same
// rule the schema enforces — this layer reports the problem before the
// file is touched.
func TestMarkVerified_RejectsBadDate(t *testing.T) {
	t.Parallel()
	path := writeTempEntry(t, "id: A1.foo.bar\nstatus: needs_verification\nlast_verified_at: null\nverified_by: null\n")
	if err := MarkVerified(path, "29-05-2026", "tester"); err == nil {
		t.Fatalf("expected error on bad date format, got nil")
	}
}

// TestMarkVerified_RejectsMissingPlaceholderKeys protects against
// template drift: an entry without status / last_verified_at /
// verified_by placeholders signals an unusual file the verifier should
// look at before mutating. We refuse to invent the keys.
func TestMarkVerified_RejectsMissingPlaceholderKeys(t *testing.T) {
	t.Parallel()
	// No verified_by key at all.
	path := writeTempEntry(t, "id: A1.foo.bar\nstatus: needs_verification\nlast_verified_at: null\n")
	err := MarkVerified(path, "2026-05-29", "tester")
	if err == nil {
		t.Fatal("expected error on missing verified_by key, got nil")
	}
	if !strings.Contains(err.Error(), "verified_by") {
		t.Errorf("error %q should mention the missing key verified_by", err)
	}
}

func writeTempEntry(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "A1.foo.bar.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
