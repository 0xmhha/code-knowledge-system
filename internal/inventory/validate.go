package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// ID-shape patterns copied from shared/entry.schema.yaml. Kept here so
// the validator catches malformed IDs even when shape-only JSON-Schema
// tooling is not in the loop. If the schema's regex changes, update
// these strings — the schema YAML file is the source of truth.
var (
	entryIDPattern        = regexp.MustCompile(`^[A-Z][0-9]+\.[a-z0-9_]+(\.[a-z0-9_]+)+$`)
	subsystemPattern      = regexp.MustCompile(`^[A-Z][0-9]+$`)
	dateYYYYMMDDPattern   = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	knowledgeTypesAllowed = map[string]bool{
		"B1": true, "B2": true, "B3": true, "B4": true, "B5": true, "B6": true, "B7": true,
	}
	statusesAllowed = map[string]bool{
		"needs_author":       true,
		"draft":              true,
		"needs_verification": true,
		"verified":           true,
	}
	prioritiesAllowed = map[string]bool{
		"P0": true, "P1": true, "P2": true,
	}
	riskLevelsAllowed = map[string]bool{
		"low": true, "medium": true, "high": true,
	}
	sourcesAllowed = map[string]bool{
		"code": true, "docs": true, "paper": true,
		"domain_expert": true, "multiple": true, "code+docs": true,
	}
)

// ValidateProject runs every project-aware check (schema shape +
// cross-reference resolution) and returns a flat list of issues.
//
// The validator never modifies the project. Callers (CLI, future CI
// integration) decide what to do with the issues; a typical CLI prints
// errors and exits non-zero when HasErrors returns true.
//
// Issues are emitted in stable order: entries first in lexical ID
// order, then per-entry checks in the order they appear below.
func ValidateProject(p *Project) []Issue {
	var issues []Issue
	if p == nil {
		return []Issue{{Severity: SeverityError, Message: "project is nil"}}
	}
	for _, id := range p.EntryIDsSorted() {
		e := p.Entries[id]
		issues = append(issues, validateEntry(p, e)...)
	}
	return issues
}

// ValidateEntry runs every check for a single entry against project
// context p. Used by tools that need to pre-flight a planned mutation
// (cks-entry-verify simulates the verified transition before writing,
// to avoid leaving the file in an invalid state when a check fails).
//
// ValidateEntry returns the same Issue shape ValidateProject uses, so
// reporters can format both the same way.
func ValidateEntry(p *Project, e Entry) []Issue {
	return validateEntry(p, e)
}

// validateEntry collects every mechanical issue for one entry. Issues
// are appended in this fixed order so the output reads top-to-bottom:
//
//  1. filename rule
//  2. enum / pattern / length on required fields
//  3. status-driven conditional requirements (verified, needs_verification)
//  4. cross-references (subsystem, related_concepts)
//  5. anchor / doc reference existence on disk
//  6. soft warnings (empty code_keywords, etc.)
func validateEntry(p *Project, e Entry) []Issue {
	var out []Issue
	add := func(sev Severity, msg string) {
		out = append(out, Issue{
			Severity: sev,
			EntryID:  e.ID,
			File:     e.SourcePath,
			Message:  msg,
		})
	}

	// 1. Filename matches ID. Mismatched names break related_concepts
	// resolution and cks-glossary-gen ordering.
	wantBase := e.ID + ".yaml"
	if base := filepath.Base(e.SourcePath); base != wantBase {
		add(SeverityError, fmt.Sprintf("filename %q does not match id (want %q)", base, wantBase))
	}

	// 2. Required field shapes.
	if !entryIDPattern.MatchString(e.ID) {
		add(SeverityError, fmt.Sprintf("id %q does not match %s", e.ID, entryIDPattern.String()))
	}
	if !subsystemPattern.MatchString(e.Subsystem) {
		add(SeverityError, fmt.Sprintf("subsystem %q does not match %s", e.Subsystem, subsystemPattern.String()))
	}
	if !knowledgeTypesAllowed[e.KnowledgeType] {
		add(SeverityError, fmt.Sprintf("knowledge_type %q not in B1..B7", e.KnowledgeType))
	}
	if !statusesAllowed[e.Status] {
		add(SeverityError, fmt.Sprintf("status %q not in {needs_author, draft, needs_verification, verified}", e.Status))
	}
	if !prioritiesAllowed[e.Priority] {
		add(SeverityError, fmt.Sprintf("priority %q not in {P0, P1, P2}", e.Priority))
	}
	if n := len(e.Title); n < 4 || n > 120 {
		add(SeverityError, fmt.Sprintf("title length %d outside [4, 120]", n))
	}
	if len(e.Summary) < 10 {
		add(SeverityError, fmt.Sprintf("summary length %d below minimum 10", len(e.Summary)))
	}
	if e.RiskLevel != "" && !riskLevelsAllowed[e.RiskLevel] {
		add(SeverityError, fmt.Sprintf("risk_level %q not in {low, medium, high}", e.RiskLevel))
	}
	if e.SourceOfTruth != "" && !sourcesAllowed[e.SourceOfTruth] {
		add(SeverityError, fmt.Sprintf("source_of_truth %q not in {code, docs, paper, domain_expert, multiple, code+docs}", e.SourceOfTruth))
	}
	if e.LastVerifiedAt != "" && !dateYYYYMMDDPattern.MatchString(e.LastVerifiedAt) {
		add(SeverityError, fmt.Sprintf("last_verified_at %q must match YYYY-MM-DD", e.LastVerifiedAt))
	}

	// 3. Status-driven conditional requirements.
	switch e.Status {
	case "verified":
		if len(e.CodeAnchors) == 0 {
			add(SeverityError, "verified status requires at least one code_anchor")
		}
		if e.LastVerifiedAt == "" {
			add(SeverityError, "verified status requires last_verified_at")
		}
		if e.VerifiedBy == "" {
			add(SeverityError, "verified status requires verified_by")
		}
		if e.RiskLevel == "" {
			add(SeverityWarning, "verified status without risk_level — set low/medium/high so the resolver can rank aliases")
		}
	case "needs_verification":
		if len(e.CodeAnchors) == 0 {
			add(SeverityError, "needs_verification status requires at least one code_anchor")
		}
	}

	// 4. Cross-references.
	if e.Subsystem != "" {
		if _, ok := p.Subsystems[e.Subsystem]; !ok {
			add(SeverityError, fmt.Sprintf("subsystem %q not declared in subsystems.yaml", e.Subsystem))
		}
	}
	for _, ref := range e.RelatedConcepts {
		if _, ok := p.Entries[ref]; !ok {
			add(SeverityError, fmt.Sprintf("related_concepts entry %q does not exist", ref))
		}
		if ref == e.ID {
			add(SeverityError, "related_concepts must not reference itself")
		}
	}

	// 5. Anchor / doc-ref existence on disk. Skip when code_root is
	// unset — that is a degenerate test fixture, not production use.
	if p.CodeRoot != "" {
		for i, a := range e.CodeAnchors {
			abs := filepath.Join(p.CodeRoot, a.File)
			if _, err := os.Stat(abs); err != nil {
				add(SeverityError, fmt.Sprintf("code_anchors[%d].file %q not found under code_root", i, a.File))
			}
		}
		for i, d := range e.ExistingDocRef {
			abs := filepath.Join(p.CodeRoot, d.File)
			if _, err := os.Stat(abs); err != nil {
				add(SeverityError, fmt.Sprintf("existing_doc_ref[%d].file %q not found under code_root", i, d.File))
			}
		}
	}

	// 6. Soft warnings.
	if len(e.CodeKeywords) == 0 && e.Status != "needs_author" {
		add(SeverityWarning, "code_keywords is empty — BM25 retrieval will not match this entry")
	}
	if len(e.KoreanAliases)+len(e.EnglishAliases) == 0 && e.Status != "needs_author" {
		add(SeverityWarning, "no aliases (korean_aliases, english_aliases) — vocab resolver cannot match this entry")
	}

	return out
}
