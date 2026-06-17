package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// RedactionAction names the disposition of a sanitize-rule hit.
//
// Data-flow note: sanitization runs in the composer's final stage, just
// before the EvidencePack is released to MCP/HTTP callers. Anything that
// reaches the coding agent / LLM has already been processed by this stage.
// The action a rule selects is the *only* knob that decides whether the
// matched data crosses the cks → caller → LLM boundary.
type RedactionAction string

const (
	// RedactionMask replaces matched text with a placeholder (e.g. "***")
	// before release. The Citation remains intact and the Body is sent on
	// to the caller with the substitution applied.
	//
	// LLM EXPOSURE: yes — the masked text reaches the LLM. The substitution
	// destroys the secret value but may leak structure (length, prefix,
	// surrounding context). Use only when partial exposure is acceptable.
	//
	// The Phase-0 base ruleset (policies/sanitization_rules.yaml) does NOT
	// use mask; it is defined here so operators can opt-in per project
	// policy (e.g., dev-only environments) without code changes.
	RedactionMask RedactionAction = "mask"

	// RedactionDrop removes the offending Body entirely. The Citation
	// remains in EvidencePack.Citations so the caller knows a citation
	// existed at that location, but the text never crosses the boundary.
	//
	// LLM EXPOSURE: no body text — only the file:line location. Default
	// choice for the base ruleset.
	RedactionDrop RedactionAction = "drop"

	// RedactionFailClosed refuses to release the pack at all. The composer
	// returns an error; cks emits no response. No partial content is ever
	// exposed.
	//
	// LLM EXPOSURE: none — the LLM call cannot proceed. Use for rules that
	// represent a hard policy boundary (e.g., private keys, prod credentials).
	RedactionFailClosed RedactionAction = "fail_closed"
)

// Redaction records one sanitize-rule hit. Used in EvidencePack.SanitizeReport.
//
// Excerpt is a *safe* (non-secret) summary of what was redacted — e.g.,
// "API key matched on line 42 (sk-* form)". It must never contain the raw
// matched bytes; the sanitizer is responsible for redaction discipline.
type Redaction struct {
	RuleID  string          `json:"rule_id"`
	Path    string          `json:"path,omitempty"` // JSON-pointer-ish location within the pack
	Action  RedactionAction `json:"action"`
	Excerpt string          `json:"excerpt,omitempty"`
}

// Body is the code text for a Citation. Held separately from Hits so that
// many Hits can reference the same Body without duplication.
type Body struct {
	Citation Citation `json:"citation"`
	Text     string   `json:"text"`
	// TokenEstimate is the composer's approximate token cost of Text;
	// used by the budget manager. Zero when not yet computed.
	TokenEstimate int `json:"token_estimate,omitempty"`
}

// Relation names a graph edge between two Citations. The composer's
// graph-neighbor expander populates EvidencePack.GraphNeighbors with edges
// of these types.
type Relation string

const (
	// RelationCalls — Source's function body calls Target.
	RelationCalls Relation = "calls"
	// RelationCalledBy — Source is called by Target. Inverse of RelationCalls.
	RelationCalledBy Relation = "called_by"
	// RelationImplements — Source (a concrete type) implements Target
	// (an interface).
	RelationImplements Relation = "implements"
	// RelationImports — Source (a file or package) imports Target.
	RelationImports Relation = "imports"
	// RelationReferences — Source references Target as a symbol (variable
	// use, type instantiation, constant read). Coarser than RelationCalls.
	RelationReferences Relation = "references"
	// RelationTestedBy — Source (production code) is tested by Target
	// (a *_test.go function or table-case).
	RelationTestedBy Relation = "tested_by"
	// RelationEmbeds — Source (a struct or interface) embeds Target. In Go,
	// this includes both struct embedding and interface composition.
	RelationEmbeds Relation = "embeds"
	// RelationDefines — Source (a file) defines Target (a top-level
	// symbol: type, function, var, or const).
	RelationDefines Relation = "defines"
)

// Neighbor is one graph edge in an EvidencePack. Distance is the hop count
// from the originating citation (1 = direct neighbor, 2 = neighbor's
// neighbor); the composer's expander caps this for budget reasons.
type Neighbor struct {
	Source   Citation `json:"source"`
	Target   Citation `json:"target"`
	Relation Relation `json:"relation"`
	Distance int      `json:"distance"`
}

// IsValid reports whether n is structurally sound: valid endpoints, a known
// Relation, and a positive Distance.
func (n Neighbor) IsValid() bool {
	if !n.Source.IsValid() || !n.Target.IsValid() {
		return false
	}
	if n.Distance < 1 {
		return false
	}
	switch n.Relation {
	case RelationCalls, RelationCalledBy, RelationImplements, RelationImports,
		RelationReferences, RelationTestedBy, RelationEmbeds, RelationDefines:
		return true
	default:
		return false
	}
}

// IntegrityHashAlgo identifies which hash function the IntegrityHash uses.
// Phase-0 fixes this to SHA-256; the field exists so future migrations
// (e.g., to HMAC for adversarial integrity) can be announced without
// breaking older consumers.
type IntegrityHashAlgo string

const (
	IntegrityHashAlgoSHA256 IntegrityHashAlgo = "sha256"
)

// PackMetadata carries provenance, budgeting, and integrity state for an
// EvidencePack.
type PackMetadata struct {
	BudgetTokens     int       `json:"budget_tokens"`
	UsedTokens       int       `json:"used_tokens"`
	UtilizationRatio float64   `json:"utilization_ratio,omitempty"`
	BuiltAt          time.Time `json:"built_at"`
	BuilderVersion   string    `json:"builder_version,omitempty"`
	// CKGSchemaVersion and CKVStatsHash are opaque pin values that an
	// evaluation harness can compare across runs to confirm the same
	// index snapshot was used. Empty when the backend did not supply them.
	CKGSchemaVersion string `json:"ckg_schema_version,omitempty"`
	CKVStatsHash     string `json:"ckv_stats_hash,omitempty"`
	// IntegrityHash is a hex-encoded hash of the canonical serialization
	// of the entire EvidencePack with IntegrityHash itself blanked. The
	// receiver recomputes the same value to confirm the pack was not
	// tampered in transit or storage. Populated by ComputeIntegrityHash
	// when the composer finishes pack assembly; verified by
	// VerifyIntegrity at the consumer.
	IntegrityHash     string            `json:"integrity_hash,omitempty"`
	IntegrityHashAlgo IntegrityHashAlgo `json:"integrity_hash_algo,omitempty"`
}

// EvidencePack is the cks output unit: an intent-classified, token-budgeted,
// sanitized, hash-integrity-stamped bundle of citations and code bodies
// that an upper-layer LLM client can consume directly.
//
// Phase-0 GraphNeighbors are populated by the Phase-B.5 expander; the field
// is present in the contract from P0.3.1 so the type does not change shape
// when B.5 ships.
type EvidencePack struct {
	Intent         Intent      `json:"intent,omitempty"`
	Query          string      `json:"query"`
	Citations      []Citation  `json:"citations"`
	Bodies         []Body      `json:"bodies,omitempty"`
	GraphNeighbors []Neighbor  `json:"graph_neighbors,omitempty"`
	SanitizeReport []Redaction `json:"sanitize_report,omitempty"`
	// Instructions is populated by Compose runs that used dummy ckv/ckg
	// backends in place of real ones. Each entry directs the upstream
	// LLM (coding-agent) to execute a skill against go-stablenet source
	// to produce the response the real backend would have returned.
	// Always empty once ckv/ckg are wired in.
	Instructions []DummyInstruction `json:"instructions,omitempty"`
	Metadata     PackMetadata       `json:"metadata"`
}

// IsValid reports whether p is structurally sound:
//   - non-empty Query
//   - every Citation valid
//   - every Body's Citation valid and present in Citations
//   - every Neighbor valid; both endpoints present in Citations
//   - SanitizeReport actions are recognized
//
// Token-budget validity (UsedTokens <= BudgetTokens) is checked separately
// by the composer; IsValid is intentionally permissive on budgeting because
// over-budget packs are still useful for debugging.
//
// IsValid does NOT verify IntegrityHash; use VerifyIntegrity for that.
// Structural validity and integrity are separate concerns: a structurally
// valid pack can still be tampered, and an integrity-clean pack can still
// be structurally malformed (if a tampered version slips past Verify due
// to a hash collision, which is cryptographically negligible for SHA-256).
func (p EvidencePack) IsValid() bool {
	if p.Query == "" {
		return false
	}
	if !p.Intent.IsValid() {
		return false
	}

	citationKeys := make(map[string]struct{}, len(p.Citations))
	for _, c := range p.Citations {
		if !c.IsValid() {
			return false
		}
		citationKeys[c.Key()] = struct{}{}
	}
	for _, b := range p.Bodies {
		if !b.Citation.IsValid() {
			return false
		}
		if _, ok := citationKeys[b.Citation.Key()]; !ok {
			return false
		}
	}
	for _, n := range p.GraphNeighbors {
		if !n.IsValid() {
			return false
		}
		if _, ok := citationKeys[n.Source.Key()]; !ok {
			return false
		}
		if _, ok := citationKeys[n.Target.Key()]; !ok {
			return false
		}
	}
	for _, r := range p.SanitizeReport {
		switch r.Action {
		case RedactionMask, RedactionDrop, RedactionFailClosed:
		default:
			return false
		}
	}
	return true
}

// ComputeIntegrityHash returns the hex-encoded SHA-256 hash of the canonical
// JSON serialization of p with Metadata.IntegrityHash and
// Metadata.IntegrityHashAlgo blanked. Pure function: it does not mutate p.
//
// To stamp a pack, callers should assign the returned hash and the algo
// constant to p.Metadata before releasing the pack. VerifyIntegrity reverses
// the process and reports any mismatch.
//
// Canonicalization relies on encoding/json's behavior: struct fields in
// declaration order, map keys sorted lexicographically. As long as the
// struct definitions in this package remain stable, two recipients computing
// the same hash will agree.
func ComputeIntegrityHash(p EvidencePack) (string, error) {
	p.Metadata.IntegrityHash = ""
	p.Metadata.IntegrityHashAlgo = ""
	buf, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("contract: marshal pack for hash: %w", err)
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}

// StampIntegrity sets p.Metadata.IntegrityHash and IntegrityHashAlgo using
// ComputeIntegrityHash. Convenience for callers that always want the
// SHA-256 default.
func StampIntegrity(p *EvidencePack) error {
	if p == nil {
		return fmt.Errorf("contract: StampIntegrity called with nil pack")
	}
	h, err := ComputeIntegrityHash(*p)
	if err != nil {
		return err
	}
	p.Metadata.IntegrityHash = h
	p.Metadata.IntegrityHashAlgo = IntegrityHashAlgoSHA256
	return nil
}

// VerifyIntegrity recomputes the expected hash of p and compares it to the
// hash carried in p.Metadata.IntegrityHash. Returns (true, nil) on match.
// Returns (false, nil) when the hash mismatches; returns an error when the
// pack carries no hash, an unsupported algo, or marshal fails.
func VerifyIntegrity(p EvidencePack) (bool, error) {
	if p.Metadata.IntegrityHash == "" {
		return false, fmt.Errorf("contract: pack has no integrity_hash")
	}
	switch p.Metadata.IntegrityHashAlgo {
	case IntegrityHashAlgoSHA256, "":
		// Accept empty algo as SHA-256 for backward compat with stamping
		// code that omits the algo field.
	default:
		return false, fmt.Errorf("contract: unsupported integrity_hash_algo %q", p.Metadata.IntegrityHashAlgo)
	}
	expected, err := ComputeIntegrityHash(p)
	if err != nil {
		return false, err
	}
	return expected == p.Metadata.IntegrityHash, nil
}
