// Package contract defines the public types that cks exposes through MCP
// and to in-process callers (the coding agent, evaluation harness).
//
// These types are deliberately small and stdlib-only:
//
//   - Citation        — canonical reference to a code location, identical in
//     shape to ckg/ckv citations so the three layers can
//     merge results without re-mapping.
//   - Intent          — coarse classification of a user vibe prompt (bug
//     fix, feature add, refactor, etc.) driving fan-out
//     and sanitization policy in the composer.
//   - Hit             — post-fusion search hit with rank/score and source
//     attribution (ckg, ckv, or fused).
//   - EvidencePack    — the cks output unit: citations + bodies + sanitize
//     report + metadata, token-budget-bounded by the
//     composer before release.
//
// What lives outside this package:
//
//   - Composer pipeline internals (intent classification rules, RRF
//     weights, graph-expansion strategy) — see internal/composer.
//   - Backend-specific types (ckg graph nodes, ckv chunk records) — see
//     internal/ckgclient and internal/ckvclient adapters; those translate
//     backend shapes into contract.Citation/Hit before they leave the
//     adapter boundary.
//
// Stability: types here are part of the cks MCP contract. Adding fields is
// safe (JSON additions tolerated by older clients); removing or renaming
// fields is a breaking change and requires version coordination with the
// coding agent, evaluation harness, and any external MCP consumers.
package contract
