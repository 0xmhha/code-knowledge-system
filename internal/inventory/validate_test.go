package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateProject_CatchesCommonAuthorMistakes runs the validator
// against a tiny fixture that intentionally encodes the categories of
// mistakes the script half of the verification checklist exists to
// catch. The goal is a sanity guard, not exhaustive coverage — each
// case here represents a class of bug that has appeared in real
// inventory files at least once.
func TestValidateProject_CatchesCommonAuthorMistakes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	codeRoot := filepath.Join(root, "code")
	mustMkdir(t, codeRoot)
	// One real file the good entry can anchor to.
	mustWrite(t, filepath.Join(codeRoot, "real.go"), "package main\n")

	projectDir := filepath.Join(root, "project")
	mustMkdir(t, filepath.Join(projectDir, "entries"))

	mustWrite(t, filepath.Join(projectDir, "project.yaml"), `id: sample
name: sample
code_root: `+codeRoot+`
schema_version: 1
`)
	mustWrite(t, filepath.Join(projectDir, "subsystems.yaml"), `- id: A1
  name: "Sample subsystem"
  description: "x"
  code_paths:
    - .
`)
	// Good entry — should not contribute any error issues.
	mustWrite(t, filepath.Join(projectDir, "entries", "A1.good.entry.yaml"), `id: A1.good.entry
subsystem: A1
knowledge_type: B1
title: "Good entry"
summary: "Long enough summary for the validator."
status: needs_verification
priority: P0
code_anchors:
  - file: real.go
    symbol: x
code_keywords:
  - x
korean_aliases:
  - "샘플"
`)
	// Bad entry — filename mismatches id, anchor points at a missing
	// file, related_concepts references a missing entry, knowledge_type
	// is out of enum. The fixture also has needs_verification status
	// without a code_anchor, which is itself an error.
	mustWrite(t, filepath.Join(projectDir, "entries", "A1.bad.misnamed.yaml"), `id: A1.bad.actual
subsystem: A1
knowledge_type: BX
title: "Bad entry"
summary: "Long enough summary for the validator."
status: needs_verification
priority: P0
related_concepts:
  - A1.does.not.exist
`)

	p, err := LoadProject(projectDir)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	issues := ValidateProject(p)
	var msgs []string
	for _, iss := range issues {
		msgs = append(msgs, iss.Message)
	}
	joined := strings.Join(msgs, "\n")

	for _, want := range []string{
		`filename "A1.bad.misnamed.yaml" does not match id`,
		`knowledge_type "BX" not in B1..B7`,
		`related_concepts entry "A1.does.not.exist" does not exist`,
		`needs_verification status requires at least one code_anchor`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected an issue mentioning %q, got:\n%s", want, joined)
		}
	}

	// And the good entry should not contribute any error-level issue.
	for _, iss := range issues {
		if iss.EntryID == "A1.good.entry" && iss.Severity == SeverityError {
			t.Errorf("good entry produced an unexpected error: %s", iss.Message)
		}
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
