package ckvclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Dummy is a Client that, instead of calling a real ckv backend, records
// each invocation on the InstructionCollector attached to ctx and returns
// a single placeholder Hit so the Composer pipeline keeps flowing. The
// collected instructions surface in EvidencePack.Instructions so the
// upstream LLM (coding-agent) can execute the corresponding skill against
// the go-stablenet source tree and provide the response the real ckv
// would have returned.
//
// Once ckv is ready, callers swap Dummy out for Real. The Composer and
// every other CKS module remain unchanged — they speak Client either way.
type Dummy struct {
	// SkillPath is the absolute path to the skill directory the upstream
	// LLM will consult. Defaults to DefaultSkillPath when empty.
	SkillPath string
	// SourcePath is the absolute path to the go-stablenet source tree.
	// Defaults to DefaultSourcePath when empty.
	SourcePath string

	// Degraded marks this Dummy as a fallback for an UNAVAILABLE real ckv
	// (e.g. Ollama down). When set, Health reports Reachable=false so the
	// cks.ops.health rollup yields "degraded" (S5) instead of falsely "ok".
	// DegradedReason is surfaced in the health StatsHash for operators.
	Degraded       bool
	DegradedReason string
}

// DefaultSkillPath is the on-disk skill directory used when a Dummy is
// constructed without an explicit override. Hard-coded per the
// integrated workplan to keep wiring trivial during the dummy phase.
const DefaultSkillPath = "/Users/wm-it-22-00661/Work/github/stable-net/go-stablenet-latest/.claude"

// DefaultSourcePath is the on-disk go-stablenet source root used when a
// Dummy is constructed without an explicit override.
const DefaultSourcePath = "/Users/wm-it-22-00661/Work/github/stable-net/go-stablenet-latest"

// NewDummy returns a Dummy with the default skill + source paths.
func NewDummy() *Dummy {
	return &Dummy{SkillPath: DefaultSkillPath, SourcePath: DefaultSourcePath}
}

// NewDegradedDummy returns a Dummy that additionally reports the ckv backend
// as unreachable via Health, so cks.ops.health rolls up to "degraded" (S5).
// Used when a real ckv index is configured but the embedder (Ollama) is
// unavailable: the pipeline still flows via recorded instructions, but the
// operator sees the degraded signal instead of a false "ok".
func NewDegradedDummy(reason string) *Dummy {
	return &Dummy{
		SkillPath:      DefaultSkillPath,
		SourcePath:     DefaultSourcePath,
		Degraded:       true,
		DegradedReason: reason,
	}
}

// Compile-time assertion that Dummy satisfies Client.
var _ Client = (*Dummy)(nil)

func (d *Dummy) skill() string {
	if d.SkillPath == "" {
		return DefaultSkillPath
	}
	return d.SkillPath
}

func (d *Dummy) source() string {
	if d.SourcePath == "" {
		return DefaultSourcePath
	}
	return d.SourcePath
}

// SemanticSearch records a ckv.SemanticSearch instruction on the
// context-bound collector and returns a single placeholder Hit so the
// downstream pipeline keeps advancing.
func (d *Dummy) SemanticSearch(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	args := map[string]string{
		"k":        fmt.Sprintf("%d", opts.K),
		"language": opts.Filter.Language,
		"path":     opts.Filter.PathGlob,
		"kinds":    strings.Join(opts.Filter.SymbolKinds, ","),
		"commit":   opts.Filter.CommitHash,
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to read go-stablenet source at %s and return up to %d code chunks "+
			"that are semantically related to the query %q. Treat the search as a vector-similarity "+
			"retrieval (ckv.SemanticSearch). Respect filters in Args (language, path glob, kinds, commit). "+
			"Respond with a JSON array of contract.Hit, each containing Citation{File, StartLine, EndLine}, "+
			"Rank (1-based), Score (0..1), and Source=\"ckv\".",
		d.skill(), d.source(), opts.K, query,
	)
	if c := contract.CollectorFrom(ctx); c != nil {
		c.Add(contract.DummyInstruction{
			Backend:    "ckv",
			Operation:  "SemanticSearch",
			SkillPath:  d.skill(),
			SourcePath: d.source(),
			Query:      query,
			Args:       args,
			Expected:   "[]contract.Hit",
			Directive:  directive,
		})
	}
	return []contract.Hit{placeholderHit(contract.HitSourceCKV)}, nil
}

// Freshness records a ckv.Freshness instruction and returns a stub
// FreshnessReport that claims the index is fresh.
func (d *Dummy) Freshness(ctx context.Context) (FreshnessReport, error) {
	directive := fmt.Sprintf(
		"Use the skills under %s to report whether the go-stablenet source at %s is fresh "+
			"relative to the last indexed commit. Respond with a JSON object matching "+
			"ckvclient.FreshnessReport {Fresh, IndexedHead, CurrentHead, ChangedFiles}.",
		d.skill(), d.source(),
	)
	if c := contract.CollectorFrom(ctx); c != nil {
		c.Add(contract.DummyInstruction{
			Backend:    "ckv",
			Operation:  "Freshness",
			SkillPath:  d.skill(),
			SourcePath: d.source(),
			Query:      "",
			Expected:   "ckvclient.FreshnessReport",
			Directive:  directive,
		})
	}
	return FreshnessReport{Fresh: true}, nil
}

// Health reports the dummy as not model-reachable without recording an
// instruction; health checks are part of the CKS bootstrap, not part of the
// retrieval pipeline the upstream LLM needs to fulfil.
//
// Under the ckv-required policy a Smart Dummy never represents a serviceable
// ckv: with no embedder there is no semantic retrieval, so ModelReachable is
// always false and the reason is carried in the dedicated Reason field (not
// smuggled through StatsHash, which is reserved for cross-run identity).
func (d *Dummy) Health(ctx context.Context) (Health, error) {
	if d.Degraded {
		reason := "ckv index configured but embedder (Ollama) unavailable"
		if d.DegradedReason != "" {
			reason = d.DegradedReason
		}
		// Reachable=false: the configured index could not be opened/served.
		return Health{Reachable: false, ModelReachable: false, Reason: reason}, nil
	}
	// Plain dummy = ckv not configured. The index "responds" (Reachable) but
	// there is no model, so it is not ready to serve design-grade context.
	return Health{
		Reachable:      true,
		ModelReachable: false,
		Reason:         "ckv not configured (Smart Dummy) — semantic retrieval unavailable",
		LastIndexAt:    time.Time{},
	}, nil
}

// Close is a no-op; the dummy holds no resources.
func (d *Dummy) Close() error { return nil }

// placeholderHit returns a Hit with a sentinel Citation. The Citation is
// IsValid()-passing so downstream stages (stage1 keyword extraction,
// stage2 search, …) operate on a well-formed shape; the "DUMMY" file
// name plus zero StartLine signals dummy provenance to anything that
// cares to inspect it.
func placeholderHit(src contract.HitSource) contract.Hit {
	return contract.Hit{
		Citation: contract.Citation{
			File:      "DUMMY",
			StartLine: 1,
			EndLine:   1,
		},
		Rank:   1,
		Score:  1.0,
		Source: src,
	}
}
