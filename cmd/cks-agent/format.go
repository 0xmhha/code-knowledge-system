package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// --- Wire mirrors of cks's contract.EvidencePack ---
//
// These structs intentionally duplicate the shape of
// github.com/0xmhha/code-knowledge-system/pkg/contract.EvidencePack
// rather than importing it. cks-agent talks to cks-mcp only over MCP
// stdio; keeping its own wire structs means the binary can be lifted
// out of this repo and dropped into a sibling agent project that
// targets the same MCP surface without dragging cks's package layout
// along.
//
// Field names + JSON tags must stay aligned with contract.EvidencePack
// — drift is caught at runtime (decode error) rather than build time.
// The trade-off is intentional: a runtime mismatch is the price of
// keeping cks-agent extractable.

type citation struct {
	File       string `json:"file"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	CommitHash string `json:"commit_hash"`
}

type body struct {
	Citation      citation `json:"citation"`
	Text          string   `json:"text"`
	TokenEstimate int      `json:"token_estimate,omitempty"`
}

type neighbor struct {
	Source   citation `json:"source"`
	Target   citation `json:"target"`
	Relation string   `json:"relation"`
	Distance int      `json:"distance"`
}

type redaction struct {
	RuleID  string `json:"rule_id"`
	Path    string `json:"path,omitempty"`
	Action  string `json:"action"`
	Excerpt string `json:"excerpt,omitempty"`
}

type packMetadata struct {
	BudgetTokens     int     `json:"budget_tokens"`
	UsedTokens       int     `json:"used_tokens"`
	UtilizationRatio float64 `json:"utilization_ratio,omitempty"`
	BuiltAt          string  `json:"built_at"`
	BuilderVersion   string  `json:"builder_version,omitempty"`
	CKGSchemaVersion string  `json:"ckg_schema_version,omitempty"`
	CKVStatsHash     string  `json:"ckv_stats_hash,omitempty"`
	IntegrityHash    string  `json:"integrity_hash,omitempty"`
}

type evidencePack struct {
	Intent         string       `json:"intent,omitempty"`
	Query          string       `json:"query"`
	Citations      []citation   `json:"citations"`
	Bodies         []body       `json:"bodies,omitempty"`
	GraphNeighbors []neighbor   `json:"graph_neighbors,omitempty"`
	SanitizeReport []redaction  `json:"sanitize_report,omitempty"`
	Metadata       packMetadata `json:"metadata"`
}

// --- Formatter ---

// fenceLang maps file extensions to code-fence language hints. Unknown
// extensions get a bare ``` so downstream LLMs aren't pushed toward a
// wrong syntax.
var fenceLang = map[string]string{
	".go":   "go",
	".ts":   "typescript",
	".tsx":  "typescript",
	".js":   "javascript",
	".jsx":  "javascript",
	".py":   "python",
	".sol":  "solidity",
	".rs":   "rust",
	".java": "java",
	".kt":   "kotlin",
	".rb":   "ruby",
	".sh":   "bash",
	".yaml": "yaml",
	".yml":  "yaml",
	".json": "json",
	".sql":  "sql",
}

// formatPack renders an evidencePack into an LLM-ready markdown document.
//
// Sections appear only when they have content — an empty pack emits the
// Task + Metadata sections only, so downstream LLMs are not told "here is
// the relevant code" with an empty body that would tempt them to
// hallucinate one.
//
// Body iteration preserves citation order (the composer returns them in
// a deterministic score-descending sequence; the formatter is a
// transparent passthrough).
func formatPack(p evidencePack) string {
	var b strings.Builder
	writeTask(&b, p)
	writeRelevantCode(&b, p)
	writeCallGraph(&b, p)
	writeSanitizeReport(&b, p)
	writeMetadata(&b, p)
	return b.String()
}

func writeTask(b *strings.Builder, p evidencePack) {
	b.WriteString("# Task\n\n")
	b.WriteString(p.Query)
	b.WriteString("\n")
	if p.Intent != "" {
		fmt.Fprintf(b, "\n**Detected intent**: %s\n", p.Intent)
	}
	b.WriteString("\n")
}

func writeRelevantCode(b *strings.Builder, p evidencePack) {
	if len(p.Citations) == 0 {
		return
	}
	b.WriteString("# Relevant code\n\n")

	// Build a lookup of citation key -> body text so we render bodies in
	// citation order (not body order — body slice can be a subset due
	// to sanitize-drop, and we want every citation to show up).
	bodyByKey := make(map[string]string, len(p.Bodies))
	for _, body := range p.Bodies {
		bodyByKey[citationKey(body.Citation)] = body.Text
	}

	for _, c := range p.Citations {
		fmt.Fprintf(b, "## %s\n\n", renderCitation(c))
		text, ok := bodyByKey[citationKey(c)]
		if !ok || text == "" {
			// Sanitize-drop: citation kept, body removed. Annotate so
			// the LLM understands why no code follows.
			b.WriteString("_body redacted by sanitize policy_\n\n")
			continue
		}
		lang := fenceLang[strings.ToLower(filepath.Ext(c.File))]
		fmt.Fprintf(b, "```%s\n%s\n```\n\n", lang, text)
	}
}

func writeCallGraph(b *strings.Builder, p evidencePack) {
	if len(p.GraphNeighbors) == 0 {
		return
	}
	b.WriteString("# Call graph\n\n")
	for _, n := range p.GraphNeighbors {
		fmt.Fprintf(b, "- `%s` —%s→ `%s`\n",
			renderCitation(n.Source), n.Relation, renderCitation(n.Target))
	}
	b.WriteString("\n")
}

func writeSanitizeReport(b *strings.Builder, p evidencePack) {
	if len(p.SanitizeReport) == 0 {
		return
	}
	b.WriteString("# Sanitize report\n\n")
	for _, r := range p.SanitizeReport {
		if r.Excerpt != "" {
			fmt.Fprintf(b, "- `%s` (%s): %s\n", r.RuleID, r.Action, r.Excerpt)
		} else {
			fmt.Fprintf(b, "- `%s` (%s)\n", r.RuleID, r.Action)
		}
	}
	b.WriteString("\n")
}

func writeMetadata(b *strings.Builder, p evidencePack) {
	b.WriteString("# Pack metadata\n\n")
	if p.Metadata.BuilderVersion != "" {
		fmt.Fprintf(b, "- Builder: %s\n", p.Metadata.BuilderVersion)
	}
	if p.Metadata.BuiltAt != "" {
		fmt.Fprintf(b, "- Built at: %s\n", p.Metadata.BuiltAt)
	}
	if p.Metadata.BudgetTokens > 0 {
		fmt.Fprintf(b, "- Tokens used/budget: %d/%d (%.1f%%)\n",
			p.Metadata.UsedTokens,
			p.Metadata.BudgetTokens,
			p.Metadata.UtilizationRatio*100,
		)
	}
	if p.Metadata.IntegrityHash != "" {
		fmt.Fprintf(b, "- Integrity: sha256:%s\n", p.Metadata.IntegrityHash)
	}
	b.WriteString("\n")
}

// renderCitation mirrors contract.Citation.String() — keeping the shape
// here too so cks-agent doesn't reach into pkg/contract.
func renderCitation(c citation) string {
	loc := c.File
	if c.StartLine == c.EndLine {
		loc = fmt.Sprintf("%s:%d", c.File, c.StartLine)
	} else if c.StartLine > 0 {
		loc = fmt.Sprintf("%s:%d-%d", c.File, c.StartLine, c.EndLine)
	}
	if c.CommitHash != "" {
		return loc + "@" + c.CommitHash
	}
	return loc
}

// citationKey produces the same deduplication key as
// contract.Citation.Key() so body lookups across citations work.
func citationKey(c citation) string {
	return fmt.Sprintf("%s:%d-%d", c.File, c.StartLine, c.EndLine)
}
