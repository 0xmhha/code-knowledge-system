// Package contract defines the public types that cks exposes through MCP
// and to in-process callers (the coding agent, evaluation harness).
//
// # Pipeline model
//
// The cks composer runs a two-stage funnel rather than a parallel fan-out:
//
//	vibe prompt
//	   ↓
//	[Stage 1: ckv + BM25]    semantic stage — derive concrete keywords/
//	                         symbols from the natural-language prompt
//	   ↓ keywords
//	[Stage 2: ckg + BM25]    precise stage — look up exact code locations
//	                         and traverse graph relations for those keywords
//	   ↓ citations + bodies + neighbors
//	[Sanitize stage]         apply policies/sanitization_rules.yaml; any
//	                         data crossing this gate has been redacted per
//	                         the matched rule's RedactionAction
//	   ↓
//	EvidencePack             (stamped with PackMetadata.IntegrityHash)
//	   ↓ MCP / HTTP
//	caller (coding agent / external LLM client)
//
// The sanitize stage is the *only* boundary that decides whether matched
// data crosses into the LLM-facing world. RedactionMask, RedactionDrop,
// and RedactionFailClosed differ in what they let through; see RedactionAction.
//
// # Types
//
//   - Citation     — canonical reference to a code location, identical in
//     shape to ckg/ckv citations so the three layers can
//     merge results without re-mapping.
//   - Intent       — coarse classification of a vibe prompt; each Intent's
//     doc names its Stage-1 and Stage-2 work.
//   - Hit          — post-fusion search hit with rank/score and source
//     attribution (ckg, ckv, or fused).
//   - Neighbor     — graph edge between two Citations (calls, called_by,
//     implements, imports, references, tested_by, embeds, defines).
//   - EvidencePack — cks output unit: citations + bodies + neighbors +
//     sanitize report + metadata, token-budget-bounded and
//     integrity-hash-stamped before release.
//
// # What lives outside this package
//
//   - Composer pipeline internals (intent classification rules, keyword
//     extraction, graph-expansion strategy) — see internal/composer.
//   - Backend-specific types (ckg graph nodes, ckv chunk records) — see
//     internal/ckgclient and internal/ckvclient adapters; those translate
//     backend shapes into contract.Citation/Hit before they leave the
//     adapter boundary.
//
// # Stability
//
// Types here are part of the cks MCP contract. Adding fields is safe
// (JSON additions are tolerated by older clients); removing or renaming
// fields is a breaking change and requires version coordination with the
// coding agent, evaluation harness, and any external MCP consumers.
package contract
