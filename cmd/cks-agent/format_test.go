package main

import (
	"strings"
	"testing"
)

func TestFormatPack_EmptyPackEmitsHeaderOnly(t *testing.T) {
	t.Parallel()
	out := formatPack(evidencePack{
		Query:  "find login",
		Intent: "feature_add",
	})
	if !strings.Contains(out, "# Task") {
		t.Errorf("missing Task header: %q", out)
	}
	if !strings.Contains(out, "find login") {
		t.Errorf("missing query body")
	}
	if !strings.Contains(out, "feature_add") {
		t.Errorf("missing intent")
	}
	// An empty pack must NOT spuriously render section headers for
	// sections it has no content for.
	for _, banned := range []string{"# Relevant code", "# Call graph", "# Sanitize report"} {
		if strings.Contains(out, banned) {
			t.Errorf("empty pack emitted %q section: %q", banned, out)
		}
	}
}

func TestFormatPack_CitationsAndBodies(t *testing.T) {
	t.Parallel()
	pack := evidencePack{
		Query:  "find Login handler",
		Intent: "feature_add",
		Citations: []citation{
			{File: "login.go", StartLine: 10, EndLine: 30, CommitHash: "deadbeef"},
			{File: "auth.go", StartLine: 5, EndLine: 25, CommitHash: "deadbeef"},
		},
		Bodies: []body{
			{
				Citation: citation{File: "login.go", StartLine: 10, EndLine: 30, CommitHash: "deadbeef"},
				Text:     "func Login() {}",
			},
			{
				Citation: citation{File: "auth.go", StartLine: 5, EndLine: 25, CommitHash: "deadbeef"},
				Text:     "func validate() bool { return true }",
			},
		},
	}
	out := formatPack(pack)
	if !strings.Contains(out, "# Relevant code") {
		t.Error("missing Relevant code section")
	}
	if !strings.Contains(out, "login.go:10-30") {
		t.Errorf("missing citation header for login.go")
	}
	if !strings.Contains(out, "func Login()") {
		t.Errorf("missing login.go body")
	}
	if !strings.Contains(out, "auth.go:5-25") {
		t.Errorf("missing citation header for auth.go")
	}
	if !strings.Contains(out, "func validate()") {
		t.Errorf("missing auth.go body")
	}
	// Bodies live inside fenced code blocks. Triple-backtick on its own
	// line per CommonMark; the closing fence must come AFTER the body.
	if strings.Count(out, "```") < 4 {
		t.Errorf("expected at least 4 code fences (2 open + 2 close): %q", out)
	}
}

func TestFormatPack_CitationsWithoutBodyEmitLocationOnly(t *testing.T) {
	t.Parallel()
	// Sanitize-drop scenario: citation is preserved (caller knows a
	// citation existed) but the body was removed before release. The
	// formatter must reflect that: render the citation header without
	// a code block.
	pack := evidencePack{
		Query:  "search",
		Intent: "bug_fix",
		Citations: []citation{
			{File: "secret.go", StartLine: 1, EndLine: 10},
		},
		// no Bodies for secret.go
	}
	out := formatPack(pack)
	if !strings.Contains(out, "secret.go:1-10") {
		t.Error("citation should be listed even without a body")
	}
	if strings.Contains(out, "```") {
		t.Errorf("no body present, so no code fence should appear: %q", out)
	}
}

func TestFormatPack_GraphNeighborsRendered(t *testing.T) {
	t.Parallel()
	pack := evidencePack{
		Query: "x",
		Citations: []citation{
			{File: "a.go", StartLine: 1, EndLine: 5},
			{File: "b.go", StartLine: 1, EndLine: 5},
		},
		GraphNeighbors: []neighbor{
			{
				Source:   citation{File: "a.go", StartLine: 1, EndLine: 5},
				Target:   citation{File: "b.go", StartLine: 1, EndLine: 5},
				Relation: "calls",
				Distance: 1,
			},
		},
	}
	out := formatPack(pack)
	if !strings.Contains(out, "# Call graph") {
		t.Error("missing Call graph section")
	}
	if !strings.Contains(out, "a.go:1-5") || !strings.Contains(out, "b.go:1-5") {
		t.Errorf("neighbor endpoints not rendered: %q", out)
	}
	if !strings.Contains(out, "calls") {
		t.Errorf("relation not rendered: %q", out)
	}
}

func TestFormatPack_SanitizeReportRendered(t *testing.T) {
	t.Parallel()
	pack := evidencePack{
		Query: "x",
		SanitizeReport: []redaction{
			{RuleID: "PK", Action: "drop", Excerpt: "private key matched line 42"},
			{RuleID: "EMAIL", Action: "mask", Excerpt: "email masked at line 7"},
		},
	}
	out := formatPack(pack)
	if !strings.Contains(out, "# Sanitize report") {
		t.Error("missing Sanitize report section")
	}
	if !strings.Contains(out, "PK") || !strings.Contains(out, "EMAIL") {
		t.Errorf("rule ids missing: %q", out)
	}
	if !strings.Contains(out, "drop") || !strings.Contains(out, "mask") {
		t.Errorf("actions missing: %q", out)
	}
}

func TestFormatPack_MetadataAlwaysIncluded(t *testing.T) {
	t.Parallel()
	pack := evidencePack{
		Query: "x",
		Metadata: packMetadata{
			BudgetTokens:     4000,
			UsedTokens:       2400,
			UtilizationRatio: 0.6,
			BuilderVersion:   "cks-mcp/0.0.1-dev",
			IntegrityHash:    "abc123def",
		},
	}
	out := formatPack(pack)
	if !strings.Contains(out, "# Pack metadata") {
		t.Error("missing Pack metadata section")
	}
	if !strings.Contains(out, "cks-mcp/0.0.1-dev") {
		t.Errorf("builder version missing: %q", out)
	}
	if !strings.Contains(out, "2400") || !strings.Contains(out, "4000") {
		t.Errorf("token counts missing: %q", out)
	}
	if !strings.Contains(out, "abc123def") {
		t.Errorf("integrity hash missing: %q", out)
	}
}

func TestFormatPack_BodyTextLanguageHinting(t *testing.T) {
	t.Parallel()
	// File extension drives the code-fence language hint so downstream
	// LLMs can highlight / parse. .go -> ```go, .ts -> ```typescript,
	// unknown -> bare ```.
	cases := []struct {
		file string
		want string
	}{
		{"foo.go", "```go"},
		{"foo.ts", "```typescript"},
		{"foo.tsx", "```typescript"},
		{"foo.py", "```python"},
		{"foo.sol", "```solidity"},
		{"foo.unknown", "```"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			pack := evidencePack{
				Query: "x",
				Citations: []citation{
					{File: tc.file, StartLine: 1, EndLine: 5},
				},
				Bodies: []body{
					{Citation: citation{File: tc.file, StartLine: 1, EndLine: 5}, Text: "x := 1"},
				},
			}
			out := formatPack(pack)
			if !strings.Contains(out, tc.want) {
				t.Errorf("file=%q: want fence %q in output, got %q", tc.file, tc.want, out)
			}
		})
	}
}

func TestFormatPack_DeterministicOrdering(t *testing.T) {
	t.Parallel()
	// The formatter must not reorder citations / bodies / neighbors.
	// Tests that depend on stable diff output rely on this. The
	// composer already returns them in a deterministic order; the
	// formatter just iterates.
	pack := evidencePack{
		Query: "x",
		Citations: []citation{
			{File: "z.go", StartLine: 1, EndLine: 2},
			{File: "a.go", StartLine: 1, EndLine: 2},
		},
	}
	out1 := formatPack(pack)
	out2 := formatPack(pack)
	if out1 != out2 {
		t.Error("formatPack is not deterministic")
	}
	// First citation listed is z.go (input order preserved).
	zIdx := strings.Index(out1, "z.go")
	aIdx := strings.Index(out1, "a.go")
	if zIdx < 0 || aIdx < 0 || zIdx > aIdx {
		t.Errorf("citation order not preserved: z@%d a@%d", zIdx, aIdx)
	}
}
