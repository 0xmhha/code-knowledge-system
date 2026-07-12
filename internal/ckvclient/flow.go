package ckvclient

// Phase D flow-aware surface (D-4, agreed Phase 2 deliverable). These types and
// the FlowClient interface are the cks-side translation of CKV's flow tools,
// shipped in pkg/ckv as Engine.GetFlow / ExpandFlow / FindBranches /
// GetInvariantEnforcement (CKV b8e9622, MCP 15→19). They run over the flow
// corpus (flow_step / flow_spine / curated-invariant + flow_meta) persisted in
// the ckv index (CKV Phase B).
//
// Like SemanticSearch, Real translates the backend-native ckv types into these
// cks-owned types so backend changes do not leak through the MCP surface.
//
// Alignment with the shipped API (coordination §9.2-R review): CKV did NOT
// adopt the two CKS-requested adjustments to the §9.1 draft —
//   (1) budget caps: the engine methods take no max/limit, so cks applies the
//       caps (MaxSteps / Limit / max) AFTER fetching, here in this package;
//   (2) canonical_id per step: the engine returns Symbol + Citation only, so the
//       cks types carry no CanonicalID. Callers join to ckg via Symbol
//       (ckg.FindByCanonicalID resolves a qname too). Enriching steps with a
//       resolved canonical_id is a possible follow-up, not done here.
// Field shapes also follow the shipped types: Reads/Writes/Emits are single
// strings (not lists), and ExpandResult carries Direction + OriginBranches.
//
// FlowClient is kept separate from Client (interface segregation): the composer
// never needs flow methods, only the direct-call MCP tools do. The MCP layer
// type-asserts Deps.CKV to FlowClient.

import (
	"context"
	"errors"

	"github.com/0xmhha/code-knowledge-vector/pkg/ckv"
	ckvtypes "github.com/0xmhha/code-knowledge-vector/pkg/types"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// ErrFlowUnsupported is returned by a Client whose backend does not expose the
// flow corpus (the Smart Dummy / an unconfigured backend). Callers surface it
// as "flow retrieval not available on this instance" rather than a hard failure.
var ErrFlowUnsupported = errors.New("ckvclient: flow surface not supported by backend")

// FlowClient is the optional flow-aware surface. A Client implementation may
// also implement FlowClient when its backend exposes the flow corpus.
type FlowClient interface {
	// GetFlow returns a whole flow as a step sequence in call order. Exactly
	// one of FlowQuery.{FlowID,EntryPoint,InvariantID} selects the flow.
	GetFlow(ctx context.Context, q FlowQuery) (Flow, error)

	// ExpandFlow returns the steps adjacent to a step in the given direction
	// ("up" = callers, "down" = callees), bounded by Hops and (cks-side) Limit.
	ExpandFlow(ctx context.Context, q ExpandFlowQuery) (FlowExpansion, error)

	// FindBranches maps a free-text symptom to ranked when→then@at failure-
	// condition candidates (semantic search over the flow corpus).
	FindBranches(ctx context.Context, symptom string, k int) ([]BranchMatch, error)

	// GetInvariantEnforcement enumerates every site that enforces an invariant
	// (the coding-agent H-guardrail enabler). max caps EnforcedAt cks-side
	// (0 = no cap); the backend method itself takes no cap.
	GetInvariantEnforcement(ctx context.Context, invID string, max int) (InvariantEnforcement, error)

	// FindInvariants returns curated invariants matching the filter: file
	// ("" = any) scopes to one source file; category ("" = any) filters by
	// policy category; tierMin (1|2|3, 0 = default) drops lower-confidence
	// tiers. The coding-agent diagnose path uses it to check a planned change
	// against domain rules.
	FindInvariants(ctx context.Context, file, category string, tierMin int) ([]InvariantHit, error)

	// GetConventions returns per-package AST-convention summaries under the
	// package prefix ("" = all packages) — the deterministic idiom digest.
	GetConventions(ctx context.Context, packagePrefix string) ([]ConventionHit, error)
}

// FlowQuery selects a flow for GetFlow. Exactly one of FlowID / EntryPoint /
// InvariantID must be set.
type FlowQuery struct {
	FlowID      string
	EntryPoint  string
	InvariantID string
	// MaxSteps caps the returned step count, applied cks-side (0 = no cap).
	MaxSteps int
}

// Flow is a whole flow as a step sequence (call order).
type Flow struct {
	FlowID     string     `json:"flow_id"`
	EntryPoint string     `json:"entry_point,omitempty"`
	Trigger    string     `json:"trigger,omitempty"`
	RootSymbol string     `json:"root_symbol,omitempty"`
	Links      []string   `json:"links,omitempty"`
	CalledBy   []string   `json:"called_by,omitempty"`
	Steps      []FlowStep `json:"steps"`
}

// FlowStep is one node in a flow, in call order. Join to ckg via Symbol.
type FlowStep struct {
	StepID     string            `json:"step_id"`
	Symbol     string            `json:"symbol,omitempty"`
	Citation   contract.Citation `json:"citation"`
	Kind       string            `json:"kind,omitempty"`
	Calls      []string          `json:"calls,omitempty"`
	Reads      string            `json:"reads,omitempty"`
	Writes     string            `json:"writes,omitempty"`
	Emits      string            `json:"emits,omitempty"`
	Branches   []Branch          `json:"branches,omitempty"`
	Invariants []string          `json:"invariants,omitempty"`
}

// Branch is a flow branch condition: when <cond> then <effect> at <loc>.
type Branch struct {
	When string `json:"when"`
	Then string `json:"then"`
	At   string `json:"at"`
}

// ExpandFlowQuery shapes an ExpandFlow call.
type ExpandFlowQuery struct {
	StepID    string
	Direction string // "up" | "down"
	Hops      int    // default 1
	// Limit caps the returned neighbor count, applied cks-side (0 = no cap).
	Limit int
}

// FlowExpansion is the result of ExpandFlow: the neighbors of one step plus the
// origin's own failure branches.
type FlowExpansion struct {
	Origin         string         `json:"origin"`
	Direction      string         `json:"direction"`
	OriginBranches []Branch       `json:"origin_branches,omitempty"`
	Neighbors      []FlowNeighbor `json:"neighbors"`
}

// FlowNeighbor is one adjacent step returned by ExpandFlow.
type FlowNeighbor struct {
	StepID   string            `json:"step_id"`
	Symbol   string            `json:"symbol,omitempty"`
	Citation contract.Citation `json:"citation"`
	Relation string            `json:"relation"` // "calls" (downstream) | "called_by" (upstream)
}

// BranchMatch is one ranked symptom→cause candidate from FindBranches.
type BranchMatch struct {
	When     string            `json:"when"`
	Then     string            `json:"then"`
	At       string            `json:"at"`
	StepID   string            `json:"step_id"`
	FlowID   string            `json:"flow_id"`
	Symbol   string            `json:"symbol,omitempty"`
	Citation contract.Citation `json:"citation"`
	Score    float64           `json:"score"`
}

// InvariantEnforcement enumerates every site enforcing an invariant.
type InvariantEnforcement struct {
	InvID      string            `json:"inv_id"`
	Statement  string            `json:"statement,omitempty"`
	EnforcedAt []EnforcementSite `json:"enforced_at"`
}

// EnforcementSite is one (flow, step, loc) where an invariant is enforced.
type EnforcementSite struct {
	Flow string `json:"flow,omitempty"`
	Step string `json:"step,omitempty"`
	Loc  string `json:"loc,omitempty"`
}

// InvariantHit is one curated invariant returned by FindInvariants.
type InvariantHit struct {
	ChunkID     string               `json:"chunk_id"`
	File        string               `json:"file"`
	StartLine   int                  `json:"start_line"`
	EndLine     int                  `json:"end_line"`
	Marker      string               `json:"marker,omitempty"` // e.g. "CRITICAL", "panic"
	Tier        int                  `json:"tier"`             // 1, 2, or 3 (flattened from ckv InvariantTier)
	Text        string               `json:"text"`
	Category    string               `json:"category,omitempty"`
	Guidance    ModificationGuidance `json:"guidance,omitempty"` // flattened from ckv's *ModificationGuidance (zero value when absent)
	SourceChunk string               `json:"source_chunk_id,omitempty"`
}

// ModificationGuidance is the flattened (value, not pointer) form of ckv's
// per-invariant change guidance.
type ModificationGuidance struct {
	AlsoReview    []string `json:"also_review,omitempty"`
	RequiredTests []string `json:"required_tests,omitempty"`
	WatchOut      []string `json:"watch_out,omitempty"`
}

// ConventionHit is one per-package AST-convention summary from GetConventions.
type ConventionHit struct {
	ChunkID string         `json:"chunk_id"`
	File    string         `json:"file"`
	Package string         `json:"package,omitempty"`
	Summary string         `json:"summary"`
	Stats   map[string]any `json:"stats,omitempty"`
}

// --- translation: backend ckv types → cks types ----------------------------

func flowCitation(c ckvtypes.Citation) contract.Citation {
	return contract.Citation{
		File:       c.File,
		StartLine:  c.StartLine,
		EndLine:    c.EndLine,
		CommitHash: c.CommitHash,
	}
}

func flowBranches(in []ckvtypes.Branch) []Branch {
	if len(in) == 0 {
		return nil
	}
	out := make([]Branch, len(in))
	for i, b := range in {
		out[i] = Branch{When: b.When, Then: b.Then, At: b.At}
	}
	return out
}

func translateFlowView(v *ckv.FlowView) Flow {
	if v == nil {
		return Flow{}
	}
	f := Flow{
		FlowID:     v.FlowID,
		EntryPoint: v.EntryPoint,
		Trigger:    v.Trigger,
		RootSymbol: v.RootSymbol,
		Links:      v.Links,
		CalledBy:   v.CalledBy,
	}
	for _, s := range v.Steps {
		f.Steps = append(f.Steps, FlowStep{
			StepID:     s.StepID,
			Symbol:     s.Symbol,
			Citation:   flowCitation(s.Citation),
			Kind:       s.Kind,
			Calls:      s.Calls,
			Reads:      s.Reads,
			Writes:     s.Writes,
			Emits:      s.Emits,
			Branches:   flowBranches(s.Branches),
			Invariants: s.Invariants,
		})
	}
	return f
}

func translateExpand(r *ckv.ExpandResult) FlowExpansion {
	if r == nil {
		return FlowExpansion{}
	}
	exp := FlowExpansion{
		Origin:         r.Origin,
		Direction:      r.Direction,
		OriginBranches: flowBranches(r.OriginBranches),
	}
	for _, n := range r.Neighbors {
		exp.Neighbors = append(exp.Neighbors, FlowNeighbor{
			StepID:   n.StepID,
			Symbol:   n.Symbol,
			Citation: flowCitation(n.Citation),
			Relation: n.Relation,
		})
	}
	return exp
}

func translateBranchMatches(in []ckv.BranchMatch) []BranchMatch {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchMatch, len(in))
	for i, m := range in {
		out[i] = BranchMatch{
			When:     m.When,
			Then:     m.Then,
			At:       m.At,
			StepID:   m.StepID,
			FlowID:   m.FlowID,
			Symbol:   m.Symbol,
			Citation: flowCitation(m.Citation),
			Score:    m.Score,
		}
	}
	return out
}

func translateInvariant(v *ckv.InvariantEnforcement) InvariantEnforcement {
	if v == nil {
		return InvariantEnforcement{}
	}
	inv := InvariantEnforcement{InvID: v.InvID, Statement: v.Statement}
	for _, p := range v.EnforcedAt {
		inv.EnforcedAt = append(inv.EnforcedAt, EnforcementSite{Flow: p.Flow, Step: p.Step, Loc: p.Loc})
	}
	return inv
}

func translateInvariantHits(in []ckv.InvariantHit) []InvariantHit {
	if len(in) == 0 {
		return nil
	}
	out := make([]InvariantHit, len(in))
	for i, h := range in {
		hit := InvariantHit{
			ChunkID:     h.ChunkID,
			File:        h.File,
			StartLine:   h.StartLine,
			EndLine:     h.EndLine,
			Marker:      h.Marker,
			Tier:        int(h.Tier),
			Text:        h.Text,
			Category:    h.Category,
			SourceChunk: h.SourceChunk,
		}
		if h.Guidance != nil {
			hit.Guidance = ModificationGuidance{
				AlsoReview:    h.Guidance.AlsoReview,
				RequiredTests: h.Guidance.RequiredTests,
				WatchOut:      h.Guidance.WatchOut,
			}
		}
		out[i] = hit
	}
	return out
}

func translateConventionHits(in []ckv.ConventionHit) []ConventionHit {
	if len(in) == 0 {
		return nil
	}
	out := make([]ConventionHit, len(in))
	for i, c := range in {
		out[i] = ConventionHit{
			ChunkID: c.ChunkID,
			File:    c.File,
			Package: c.Package,
			Summary: c.Summary,
			Stats:   c.Stats,
		}
	}
	return out
}

// --- Real: in-process pkg/ckv.Engine flow methods (T4 wired) ---------------

// Compile-time assertion that Real satisfies FlowClient.
var _ FlowClient = (*Real)(nil)

func (r *Real) GetFlow(ctx context.Context, q FlowQuery) (Flow, error) {
	view, err := r.eng.GetFlow(ctx, ckv.FlowSelector{
		FlowID:      q.FlowID,
		EntryPoint:  q.EntryPoint,
		InvariantID: q.InvariantID,
	})
	if err != nil {
		return Flow{}, err
	}
	f := translateFlowView(view)
	if q.MaxSteps > 0 && len(f.Steps) > q.MaxSteps {
		f.Steps = f.Steps[:q.MaxSteps]
	}
	return f, nil
}

func (r *Real) ExpandFlow(ctx context.Context, q ExpandFlowQuery) (FlowExpansion, error) {
	res, err := r.eng.ExpandFlow(ctx, q.StepID, q.Direction, q.Hops)
	if err != nil {
		return FlowExpansion{}, err
	}
	exp := translateExpand(res)
	if q.Limit > 0 && len(exp.Neighbors) > q.Limit {
		exp.Neighbors = exp.Neighbors[:q.Limit]
	}
	return exp, nil
}

func (r *Real) FindBranches(ctx context.Context, symptom string, k int) ([]BranchMatch, error) {
	matches, err := r.eng.FindBranches(ctx, symptom, k)
	if err != nil {
		return nil, err
	}
	return translateBranchMatches(matches), nil
}

func (r *Real) GetInvariantEnforcement(ctx context.Context, invID string, max int) (InvariantEnforcement, error) {
	v, err := r.eng.GetInvariantEnforcement(ctx, invID)
	if err != nil {
		return InvariantEnforcement{}, err
	}
	inv := translateInvariant(v)
	if max > 0 && len(inv.EnforcedAt) > max {
		inv.EnforcedAt = inv.EnforcedAt[:max]
	}
	return inv, nil
}

func (r *Real) FindInvariants(ctx context.Context, file, category string, tierMin int) ([]InvariantHit, error) {
	hits, err := r.eng.FindInvariants(ctx, file, category, tierMin)
	if err != nil {
		return nil, err
	}
	return translateInvariantHits(hits), nil
}

func (r *Real) GetConventions(ctx context.Context, packagePrefix string) ([]ConventionHit, error) {
	hits, err := r.eng.GetConventions(ctx, packagePrefix)
	if err != nil {
		return nil, err
	}
	return translateConventionHits(hits), nil
}

// --- Dummy: flow is a Real-backend feature; the Smart Dummy predates it -----

// Compile-time assertion that Dummy satisfies FlowClient.
var _ FlowClient = (*Dummy)(nil)

func (d *Dummy) GetFlow(ctx context.Context, q FlowQuery) (Flow, error) {
	return Flow{}, ErrFlowUnsupported
}

func (d *Dummy) ExpandFlow(ctx context.Context, q ExpandFlowQuery) (FlowExpansion, error) {
	return FlowExpansion{}, ErrFlowUnsupported
}

func (d *Dummy) FindBranches(ctx context.Context, symptom string, k int) ([]BranchMatch, error) {
	return nil, ErrFlowUnsupported
}

func (d *Dummy) GetInvariantEnforcement(ctx context.Context, invID string, max int) (InvariantEnforcement, error) {
	return InvariantEnforcement{}, ErrFlowUnsupported
}

// FindInvariants / GetConventions degrade to an empty result (not
// ErrFlowUnsupported like the flow-navigation methods above): these are
// knowledge lookups on the diagnose path, where "no ckv backend configured"
// should read as "no invariants/conventions found" so the caller proceeds
// without a hard error (task DoD: degraded → empty, no error).
func (d *Dummy) FindInvariants(ctx context.Context, file, category string, tierMin int) ([]InvariantHit, error) {
	return nil, nil
}

func (d *Dummy) GetConventions(ctx context.Context, packagePrefix string) ([]ConventionHit, error) {
	return nil, nil
}

// --- Fake: canned flow responses for MCP-layer and composer tests ----------

// Compile-time assertion that Fake satisfies FlowClient.
var _ FlowClient = (*Fake)(nil)

func (f *Fake) GetFlow(ctx context.Context, q FlowQuery) (Flow, error) {
	f.Calls.GetFlow = append(f.Calls.GetFlow, q)
	if f.GetFlowErr != nil {
		return Flow{}, f.GetFlowErr
	}
	out := f.FlowVal
	if q.MaxSteps > 0 && len(out.Steps) > q.MaxSteps {
		out.Steps = out.Steps[:q.MaxSteps]
	}
	return out, nil
}

func (f *Fake) ExpandFlow(ctx context.Context, q ExpandFlowQuery) (FlowExpansion, error) {
	f.Calls.ExpandFlow = append(f.Calls.ExpandFlow, q)
	if f.ExpandFlowErr != nil {
		return FlowExpansion{}, f.ExpandFlowErr
	}
	out := f.ExpandVal
	if q.Limit > 0 && len(out.Neighbors) > q.Limit {
		out.Neighbors = out.Neighbors[:q.Limit]
	}
	return out, nil
}

func (f *Fake) FindBranches(ctx context.Context, symptom string, k int) ([]BranchMatch, error) {
	f.Calls.FindBranches = append(f.Calls.FindBranches, FindBranchesCall{Symptom: symptom, K: k})
	if f.FindBranchesErr != nil {
		return nil, f.FindBranchesErr
	}
	out := f.BranchMatches
	if k > 0 && len(out) > k {
		out = out[:k]
	}
	return out, nil
}

func (f *Fake) GetInvariantEnforcement(ctx context.Context, invID string, max int) (InvariantEnforcement, error) {
	f.Calls.GetInvariantEnforcement = append(f.Calls.GetInvariantEnforcement, GetInvariantEnforcementCall{InvID: invID, Max: max})
	if f.GetInvariantEnforcementErr != nil {
		return InvariantEnforcement{}, f.GetInvariantEnforcementErr
	}
	out := f.EnforcementVal
	if max > 0 && len(out.EnforcedAt) > max {
		out.EnforcedAt = out.EnforcedAt[:max]
	}
	return out, nil
}

func (f *Fake) FindInvariants(ctx context.Context, file, category string, tierMin int) ([]InvariantHit, error) {
	f.Calls.FindInvariants = append(f.Calls.FindInvariants, FindInvariantsCall{File: file, Category: category, TierMin: tierMin})
	if f.FindInvariantsErr != nil {
		return nil, f.FindInvariantsErr
	}
	return f.InvariantHits, nil
}

func (f *Fake) GetConventions(ctx context.Context, packagePrefix string) ([]ConventionHit, error) {
	f.Calls.GetConventions = append(f.Calls.GetConventions, GetConventionsCall{PackagePrefix: packagePrefix})
	if f.GetConventionsErr != nil {
		return nil, f.GetConventionsErr
	}
	return f.Conventions, nil
}
