// Package inventory loads, validates, and renders the domain-knowledge
// inventory of a single project (docs/domain-knowledge/projects/<id>/).
//
// Three callers share this package:
//
//   - cks-inventory-check  — runs ValidateProject and reports issues.
//   - cks-entry-verify     — promotes one entry to status: verified and
//     re-syncs inventory.md counts via UpdateInventoryCounts.
//   - cks-glossary-gen     — could in principle reuse LoadEntries here;
//     today it still walks entries/*.yaml directly because it cares only
//     about a narrow subset of fields. Refactoring it onto this package
//     is a deferred cleanup, not a correctness gap.
//
// The package is deliberately project-aware: cross-reference checks
// (subsystem existence, related_concepts resolution, code_anchors[].file
// existence under code_root) need the full project context that the raw
// JSON-Schema in shared/entry.schema.yaml cannot express.
package inventory

import "sort"

// Project is the in-memory view of a single docs/domain-knowledge/projects/<id>/
// directory: the project.yaml header, the subsystems index, and every
// entry under entries/.
type Project struct {
	// Dir is the absolute path of the project directory on disk.
	// Cross-reference checks resolve paths relative to CodeRoot, but the
	// project's own files (inventory.md, glossary.yaml, etc.) live here.
	Dir string

	// ID matches the project's id field in project.yaml and the
	// directory name under docs/domain-knowledge/projects/.
	ID string

	// Name is the human-readable project name (project.yaml name).
	Name string

	// CodeRoot is the absolute path to the working tree the entries
	// describe. code_anchors[].file and existing_doc_ref[].file resolve
	// underneath this root.
	CodeRoot string

	// SchemaVersion is the project's schema_version field. Today every
	// project is on version 1; bumping requires migrating entries first.
	SchemaVersion int

	// Subsystems is keyed by subsystem ID (e.g. "A1"). The order in
	// subsystems.yaml is preserved by SubsystemOrder.
	Subsystems     map[string]Subsystem
	SubsystemOrder []string

	// Entries is keyed by entry ID. The iteration order callers want
	// (sorted by ID) is provided by EntryIDsSorted.
	Entries map[string]Entry

	// AuthoritativeDocs mirrors project.yaml's authoritative_docs: the
	// curated docs (relative to CodeRoot) the embedding corpus copies in.
	AuthoritativeDocs []AuthoritativeDoc
}

// Subsystem is one record from subsystems.yaml. Code paths are stored
// for documentation only; the validator does not currently enforce that
// every code_anchor in a subsystem's entries lives under one of its
// code_paths (that would force authors to choose a single home for
// cross-cutting concerns and we do not yet have evidence the constraint
// is worth the friction).
type Subsystem struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	CodePaths   []string `yaml:"code_paths"`
}

// Entry is one record from entries/<id>.yaml.
//
// Field order in the struct mirrors SCHEMA.md so a future Marshal that
// wants deterministic output can use it directly. Today writes go
// through verify.go, which mutates the on-disk YAML node tree instead of
// marshalling this struct — that is how comments, blank lines, and
// hand-chosen field ordering survive verify operations.
type Entry struct {
	// SourcePath is the absolute path of the YAML file this entry was
	// loaded from. Not a YAML field — set by LoadEntry, used by the
	// validator for issue reporting and by verify.go for write-back.
	SourcePath string `yaml:"-"`

	// Required fields per SCHEMA.md.
	ID            string `yaml:"id"`
	Subsystem     string `yaml:"subsystem"`
	KnowledgeType string `yaml:"knowledge_type"`
	Title         string `yaml:"title"`
	Summary       string `yaml:"summary"`
	Status        string `yaml:"status"`
	Priority      string `yaml:"priority"`

	// Optional but recommended fields.
	CodeAnchors     []CodeAnchor `yaml:"code_anchors,omitempty"`
	CodeKeywords    []string     `yaml:"code_keywords,omitempty"`
	KoreanAliases   []string     `yaml:"korean_aliases,omitempty"`
	EnglishAliases  []string     `yaml:"english_aliases,omitempty"`
	RelatedConcepts []string     `yaml:"related_concepts,omitempty"`
	ExistingDocRef  []DocRef     `yaml:"existing_doc_ref,omitempty"`

	// Conditional fields (relevant only for certain knowledge types).
	Invariants     []string   `yaml:"invariants,omitempty"`
	Pitfalls       []string   `yaml:"pitfalls,omitempty"`
	ProcedureSteps []string   `yaml:"procedure_steps,omitempty"`
	Constants      []Constant `yaml:"constants,omitempty"`

	// Provenance.
	RiskLevel      string `yaml:"risk_level,omitempty"`
	SourceOfTruth  string `yaml:"source_of_truth,omitempty"`
	LastVerifiedAt string `yaml:"last_verified_at,omitempty"`
	VerifiedBy     string `yaml:"verified_by,omitempty"`
}

// CodeAnchor points at a specific spot in source. File is repo-relative
// (resolved against Project.CodeRoot). Symbol and line are advisory;
// only File is required by the schema.
type CodeAnchor struct {
	File   string `yaml:"file"`
	Symbol string `yaml:"symbol,omitempty"`
	Line   int    `yaml:"line,omitempty"`
	// Kind classifies the anchor (symbol-identity Phase 3, A1-3):
	//   "def" — points at a symbol's definition; Symbol must be uniquely
	//           resolvable and Line == its definition line (anchor-refresh may
	//           mechanically refresh the line).
	//   "loc" — points at an arbitrary Line inside EnclosingSymbol (a call site,
	//           a gate, a branch); no def-line rule, never repointed.
	// Empty is treated as "def" for back-compat with pre-kind anchors.
	Kind string `yaml:"kind,omitempty"`
	// EnclosingSymbol names the symbol whose range contains Line, for loc
	// anchors. Used by anchor-refresh (range-containment validation) and
	// inventory-check. Empty for def anchors.
	EnclosingSymbol string `yaml:"enclosing_symbol,omitempty"`
	Reason          string `yaml:"reason,omitempty"`
}

// AnchorKindDef / AnchorKindLoc are the two CodeAnchor.Kind values; an empty
// Kind is treated as def. Centralized so tooling agrees on the spelling.
const (
	AnchorKindDef = "def"
	AnchorKindLoc = "loc"
)

// ResolvedKind returns the anchor's kind, defaulting an empty Kind to def.
func (a CodeAnchor) ResolvedKind() string {
	if a.Kind == AnchorKindLoc {
		return AnchorKindLoc
	}
	return AnchorKindDef
}

// DocRef points at a specific section of an existing project doc.
// File is repo-relative to Project.CodeRoot (existing project docs live
// under the same tree the code does, e.g. .claude/docs/...).
type DocRef struct {
	File       string `yaml:"file"`
	Section    string `yaml:"section,omitempty"`
	Subsection string `yaml:"subsection,omitempty"`
}

// AuthoritativeDoc is one entry in project.yaml's authoritative_docs:
// a curated, human-maintained document (path relative to CodeRoot) that
// the domain-knowledge corpus embeds wholesale. Role is a short label of
// what the doc covers.
type AuthoritativeDoc struct {
	File string `yaml:"file"`
	Role string `yaml:"role,omitempty"`
}

// Constant is one row in the constants list for B7 entries. Value is
// any scalar — schema does not constrain its type.
type Constant struct {
	Name       string `yaml:"name"`
	Value      any    `yaml:"value"`
	Unit       string `yaml:"unit,omitempty"`
	SourceFile string `yaml:"source_file,omitempty"`
}

// Severity classifies a validator issue.
type Severity string

const (
	// SeverityError marks an issue that blocks indexing. Validator
	// returns non-zero exit code when any error issue is present.
	SeverityError Severity = "error"
	// SeverityWarning marks an issue worth fixing but not blocking.
	// Examples: a verified entry with risk_level unset, an entry with
	// no code_keywords (BM25 will not match it). Validator still exits
	// zero when only warnings are present.
	SeverityWarning Severity = "warn"
)

// Issue is one finding from ValidateProject. The validator returns a
// flat slice rather than aggregating by entry so callers (CLI, future
// CI integration) can decide how to group output.
type Issue struct {
	Severity Severity
	// EntryID is the entry the issue belongs to. Empty for
	// project-level issues (e.g. malformed project.yaml).
	EntryID string
	// File is the absolute path of the file the issue points at,
	// when known. Lets the CLI render clickable paths.
	File string
	// Message is the human-readable description. Keep it short — one
	// line, no trailing period, in the same style as compiler errors.
	Message string
}

// HasErrors reports whether any issue in the slice is a SeverityError.
// CLIs use this to set the process exit code.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// CountByStatus returns the number of entries with each status. The
// keys returned are exactly the four canonical statuses from
// STATUS_LIFECYCLE.md — entries with unrecognized statuses are not
// counted here (the validator reports them as errors elsewhere).
func (p *Project) CountByStatus() map[string]int {
	out := map[string]int{
		"verified":           0,
		"needs_verification": 0,
		"draft":              0,
		"needs_author":       0,
	}
	for _, e := range p.Entries {
		if _, ok := out[e.Status]; ok {
			out[e.Status]++
		}
	}
	return out
}

// CountByKnowledgeType returns the number of entries with each B1..B7
// knowledge type. Returns a fully populated map (zero counts present)
// so render code can emit a stable row order.
func (p *Project) CountByKnowledgeType() map[string]int {
	out := map[string]int{
		"B1": 0, "B2": 0, "B3": 0, "B4": 0, "B5": 0, "B6": 0, "B7": 0,
	}
	for _, e := range p.Entries {
		if _, ok := out[e.KnowledgeType]; ok {
			out[e.KnowledgeType]++
		}
	}
	return out
}

// CountBySubsystem returns the number of entries per subsystem ID,
// split into verified vs total counts. The "Subsystem coverage" table
// in inventory.md renders both columns.
func (p *Project) CountBySubsystem() map[string]struct{ Verified, Total int } {
	out := make(map[string]struct{ Verified, Total int }, len(p.Subsystems))
	for id := range p.Subsystems {
		out[id] = struct{ Verified, Total int }{}
	}
	for _, e := range p.Entries {
		cur, ok := out[e.Subsystem]
		if !ok {
			// Entry references an unknown subsystem; validator catches
			// this separately. Skip here to keep counts clean.
			continue
		}
		cur.Total++
		if e.Status == "verified" {
			cur.Verified++
		}
		out[e.Subsystem] = cur
	}
	return out
}

// EntryIDsSorted returns every entry ID in deterministic lexical order.
// The renderer uses this for the "Current entries" table; the
// validator uses it so issues come out grouped by ID.
func (p *Project) EntryIDsSorted() []string {
	ids := make([]string, 0, len(p.Entries))
	for id := range p.Entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
