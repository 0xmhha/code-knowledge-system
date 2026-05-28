// Package vocab maps natural-language phrases (Korean or ambiguous English)
// to canonical code keywords sourced from a curated glossary.
//
// Code RAG against go-stablenet hits a vocabulary gap: a user's prompt
// talks about "0번 블록" but the codebase calls it "GenesisBlock". Without
// a bridge the embedder has to invent the connection from training data,
// and BM25 misses entirely because the surface strings never overlap.
// vocab.Resolver loads a glossary that names every bridge explicitly and
// expands a prompt with the canonical code keywords before retrieval runs.
//
// The package is glossary-source-agnostic: integrators ship their own
// YAML — go-stablenet glossary curation is tracked separately as V-4 /
// D-1 in the integrated workplan. An empty glossary is a valid configuration;
// Resolve just returns the original prompt unchanged in that case.
package vocab

// Entry is one curated mapping in the glossary YAML.
type Entry struct {
	// Aliases are the surface phrases a user might type. Lowercased for
	// matching. Phrases may span multiple tokens ("0번 블록").
	Aliases []string `yaml:"aliases"`
	// Canonical is the preferred natural-language phrasing for this
	// concept. Surfaced in ResolveResult for telemetry and audit.
	Canonical string `yaml:"canonical"`
	// CodeKeywords are the identifiers Stage 1 should feed to ckg's
	// BM25Search and ckv's SemanticSearch. Function names, type names,
	// package paths — whatever the codebase actually uses.
	CodeKeywords []string `yaml:"code_keywords"`
}

// Glossary is the on-disk schema. version is reserved for schema
// migrations; the loader rejects unknown versions.
type Glossary struct {
	Version int     `yaml:"version"`
	Entries []Entry `yaml:"entries"`
}

// ResolveResult is the structured output of one Resolve call.
//
// Expanded is the canonical query string to feed retrieval: the original
// prompt followed by space-separated MatchedKeywords. When no entry
// matches, Expanded equals Original and MatchedKeywords / MatchedEntries
// are empty.
type ResolveResult struct {
	Original        string   `json:"original"`
	Expanded        string   `json:"expanded"`
	MatchedEntries  []Entry  `json:"matched_entries,omitempty"`
	MatchedKeywords []string `json:"matched_keywords,omitempty"`
}
