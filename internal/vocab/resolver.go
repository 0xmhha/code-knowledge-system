package vocab

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// SupportedSchemaVersion is the only Glossary.Version this package
// accepts. Bumped only when the on-disk schema changes shape.
const SupportedSchemaVersion = 1

// Resolver expands user prompts with canonical code keywords drawn from
// a glossary. Safe for concurrent reads; the glossary is loaded once
// at construction and not mutated afterwards.
type Resolver struct {
	glossary Glossary
	// entries holds Aliases lowercased for case-insensitive matching;
	// CodeKeywords are kept verbatim because Go identifiers are
	// case-sensitive.
	entries []normalizedEntry
}

type normalizedEntry struct {
	loweredAliases []string
	original       Entry
}

// New constructs a Resolver from an already-parsed Glossary. Validates
// the schema version and rejects entries with no aliases (those would
// never match anything and likely indicate a typo).
//
// An empty glossary (Entries == nil) is permitted; Resolve becomes a
// pass-through. Stage 1 can wire a Resolver unconditionally and only
// observe expansion once the glossary is populated.
func New(g Glossary) (*Resolver, error) {
	if g.Version != 0 && g.Version != SupportedSchemaVersion {
		return nil, fmt.Errorf("vocab: unsupported schema version %d (want %d)", g.Version, SupportedSchemaVersion)
	}
	entries := make([]normalizedEntry, 0, len(g.Entries))
	for i, e := range g.Entries {
		if len(e.Aliases) == 0 {
			return nil, fmt.Errorf("vocab: entry %d has no aliases", i)
		}
		lowered := make([]string, 0, len(e.Aliases))
		for _, a := range e.Aliases {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			lowered = append(lowered, strings.ToLower(a))
		}
		if len(lowered) == 0 {
			return nil, fmt.Errorf("vocab: entry %d has only blank aliases", i)
		}
		entries = append(entries, normalizedEntry{
			loweredAliases: lowered,
			original:       e,
		})
	}
	return &Resolver{glossary: g, entries: entries}, nil
}

// Load reads a glossary YAML from path and constructs a Resolver.
// Returns an error when the file is missing, malformed, or carries an
// unsupported schema version.
func Load(path string) (*Resolver, error) {
	if path == "" {
		return nil, errors.New("vocab: empty glossary path")
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vocab: read %q: %w", path, err)
	}
	var g Glossary
	if err := yaml.Unmarshal(buf, &g); err != nil {
		return nil, fmt.Errorf("vocab: parse %q: %w", path, err)
	}
	return New(g)
}

// Resolve expands query with every entry whose alias appears in query
// (case-insensitive substring match). Returns the query untouched when
// no entry matches or when the resolver is nil.
//
// Matching policy: an alias matches when it appears as a substring in
// the lowercased query. Substring is intentional — alias phrases like
// "0번 블록" span tokens that a tokenizer would split, so token-based
// matching would miss them. A bare alias like "Foo" matches any prompt
// containing "foo", which is the desired behavior for glossary entries
// targeting common identifiers.
//
// MatchedKeywords is deduplicated across entries to keep the expanded
// query compact. Order is glossary order: entries appear in the YAML
// order, keywords appear in CodeKeywords order within each entry.
func (r *Resolver) Resolve(query string) ResolveResult {
	res := ResolveResult{Original: query, Expanded: query}
	if r == nil || query == "" || len(r.entries) == 0 {
		return res
	}
	lowered := strings.ToLower(query)
	seenKeyword := make(map[string]struct{})
	for _, ne := range r.entries {
		matched := false
		for _, a := range ne.loweredAliases {
			if strings.Contains(lowered, a) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		res.MatchedEntries = append(res.MatchedEntries, ne.original)
		for _, kw := range ne.original.CodeKeywords {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			if _, ok := seenKeyword[kw]; ok {
				continue
			}
			seenKeyword[kw] = struct{}{}
			res.MatchedKeywords = append(res.MatchedKeywords, kw)
		}
	}
	if len(res.MatchedKeywords) > 0 {
		res.Expanded = query + " " + strings.Join(res.MatchedKeywords, " ")
	}
	return res
}

// EntryCount reports how many entries are loaded. Useful for health
// checks and footprint logging.
func (r *Resolver) EntryCount() int {
	if r == nil {
		return 0
	}
	return len(r.entries)
}
