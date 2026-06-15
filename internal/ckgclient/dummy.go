package ckgclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// Dummy is a Client that, instead of calling a real ckg backend, records
// each invocation on the InstructionCollector attached to ctx and returns
// minimal placeholder data so the Composer pipeline keeps flowing. The
// collected instructions surface in EvidencePack.Instructions so the
// upstream LLM (coding-agent) can execute the corresponding skill against
// the go-stablenet source tree and provide the response the real ckg
// would have returned.
//
// Once ckg is ready, callers swap Dummy out for Real. The Composer and
// every other CKS module remain unchanged — they speak Client either way.
type Dummy struct {
	// SkillPath is the skill directory the upstream LLM will consult. When
	// empty it defaults to <SourcePath>/.claude (see skill).
	SkillPath string
	// SourcePath is the go-stablenet source tree. When empty it defaults to
	// the current working directory (see source).
	SourcePath string
}

// NewDummy returns a Dummy. When SkillPath/SourcePath are left unset they
// default to the current working directory; the caller (cmd/cks-mcp) sets
// SourcePath from config when available.
func NewDummy() *Dummy {
	return &Dummy{}
}

// Compile-time assertion that Dummy satisfies Client.
var _ Client = (*Dummy)(nil)

// source returns the configured source tree, falling back to the current
// working directory (cks-mcp runs from the indexed repo root), so dummy
// directives stay valid on any machine instead of a hard-coded path.
func (d *Dummy) source() string {
	if d.SourcePath != "" {
		return d.SourcePath
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// skill returns the configured skill directory, defaulting to <source>/.claude.
func (d *Dummy) skill() string {
	if d.SkillPath != "" {
		return d.SkillPath
	}
	return filepath.Join(d.source(), ".claude")
}

// BM25Search records a ckg.BM25Search instruction and returns a single
// placeholder Hit so downstream stages have non-empty input.
func (d *Dummy) BM25Search(ctx context.Context, query string, opts SearchOpts) ([]contract.Hit, error) {
	args := map[string]string{
		"k":        fmt.Sprintf("%d", opts.K),
		"language": opts.Filter.Language,
		"path":     opts.Filter.PathGlob,
		"commit":   opts.Filter.CommitHash,
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to run a BM25 keyword search over go-stablenet source at %s "+
			"for the query %q. Respect filters in Args. Respond with a JSON array of contract.Hit, "+
			"each containing Citation{File, StartLine, EndLine}, Rank (1-based), Score, and Source=\"ckg\".",
		d.skill(), d.source(), query,
	)
	d.record(ctx, contract.DummyInstruction{
		Backend:    "ckg",
		Operation:  "BM25Search",
		SkillPath:  d.skill(),
		SourcePath: d.source(),
		Query:      query,
		Args:       args,
		Expected:   "[]contract.Hit",
		Directive:  directive,
	})
	return []contract.Hit{placeholderHit(contract.HitSourceCKG)}, nil
}

// FindSymbol records a ckg.FindSymbol instruction and returns a single
// placeholder Citation.
func (d *Dummy) FindSymbol(ctx context.Context, name string, opts SymbolOpts) ([]contract.Citation, error) {
	args := map[string]string{
		"kinds":  strings.Join(opts.Kinds, ","),
		"path":   opts.PathGlob,
		"commit": opts.CommitHash,
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to resolve the symbol %q against go-stablenet source at %s. "+
			"Respect kind/path filters in Args. Respond with a JSON array of contract.Citation, "+
			"one per definition site, each containing File, StartLine, EndLine, CommitHash.",
		d.skill(), name, d.source(),
	)
	d.record(ctx, contract.DummyInstruction{
		Backend:    "ckg",
		Operation:  "FindSymbol",
		SkillPath:  d.skill(),
		SourcePath: d.source(),
		Query:      name,
		Args:       args,
		Expected:   "[]contract.Citation",
		Directive:  directive,
	})
	return []contract.Citation{placeholderCitation()}, nil
}

// Neighbors records a ckg.Neighbors instruction and returns no edges;
// the Composer's stage3 expander tolerates an empty neighbour set so
// returning a synthetic edge would risk feeding garbage to later stages.
func (d *Dummy) Neighbors(ctx context.Context, src contract.Citation, opts NeighborsOpts) ([]contract.Neighbor, error) {
	relations := make([]string, 0, len(opts.Relations))
	for _, r := range opts.Relations {
		relations = append(relations, string(r))
	}
	args := map[string]string{
		"relations": strings.Join(relations, ","),
		"hops":      fmt.Sprintf("%d", opts.Hops),
		"max_total": fmt.Sprintf("%d", opts.MaxTotal),
		"src":       src.String(),
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to walk the call/relationship graph around %s in go-stablenet "+
			"source at %s. Respect relations + hop limits in Args. Respond with a JSON array of "+
			"contract.Neighbor entries, each containing Source/Target citations, Relation, and Distance.",
		d.skill(), src.String(), d.source(),
	)
	d.record(ctx, contract.DummyInstruction{
		Backend:    "ckg",
		Operation:  "Neighbors",
		SkillPath:  d.skill(),
		SourcePath: d.source(),
		Query:      src.String(),
		Args:       args,
		Expected:   "[]contract.Neighbor",
		Directive:  directive,
	})
	return nil, nil
}

// ImpactOfChange records a ckg.ImpactOfChange instruction and returns
// an empty ImpactResult.
func (d *Dummy) ImpactOfChange(ctx context.Context, seedQname string, opts ImpactOpts) (contract.ImpactResult, error) {
	args := map[string]string{
		"depth":     fmt.Sprintf("%d", opts.Depth),
		"max_total": fmt.Sprintf("%d", opts.MaxTotal),
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to compute the reverse-dependency closure of %q in go-stablenet "+
			"source at %s. Group hits by coupling category (callers, interface, type_users, "+
			"distributed, concurrent, other). Respond with a JSON contract.ImpactResult "+
			"{Seed, Groups[{Category, Hits[Citation]}]}.",
		d.skill(), seedQname, d.source(),
	)
	d.record(ctx, contract.DummyInstruction{
		Backend:    "ckg",
		Operation:  "ImpactOfChange",
		SkillPath:  d.skill(),
		SourcePath: d.source(),
		Query:      seedQname,
		Args:       args,
		Expected:   "contract.ImpactResult",
		Directive:  directive,
	})
	return contract.ImpactResult{Seed: seedQname}, nil
}

// ConcurrencyImpact records a ckg.ConcurrencyImpact instruction and returns
// an empty result so the pipeline keeps flowing in degraded mode.
func (d *Dummy) ConcurrencyImpact(ctx context.Context, symbol string, opts ConcurrencyOpts) (contract.ConcurrencyResult, error) {
	args := map[string]string{
		"depth":     fmt.Sprintf("%d", opts.Depth),
		"max_total": fmt.Sprintf("%d", opts.MaxTotal),
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to compute the concurrency blast radius of %q in go-stablenet "+
			"source at %s — goroutines/channels/locks it spawns, sends to, or acquires, and modules "+
			"reached over concurrency edges. Respond with a JSON contract.ConcurrencyResult "+
			"{Seed, Depth, Modules[{Citation, Qname, Name, Kind, Direction}]}.",
		d.skill(), symbol, d.source(),
	)
	d.record(ctx, contract.DummyInstruction{
		Backend:    "ckg",
		Operation:  "ConcurrencyImpact",
		SkillPath:  d.skill(),
		SourcePath: d.source(),
		Query:      symbol,
		Args:       args,
		Expected:   "contract.ConcurrencyResult",
		Directive:  directive,
	})
	return contract.ConcurrencyResult{Seed: symbol}, nil
}

// EvidenceForIntent records a ckg.EvidenceForIntent instruction and
// returns an empty ChangeHistoryResult.
func (d *Dummy) EvidenceForIntent(ctx context.Context, intent string, opts EvidenceOpts) (contract.ChangeHistoryResult, error) {
	args := map[string]string{
		"seed_qname": opts.SeedQname,
		"k":          fmt.Sprintf("%d", opts.K),
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to surface git history hunks relevant to the intent %q in "+
			"go-stablenet source at %s. Rank by BM25 against the intent text. Respond with a "+
			"JSON contract.ChangeHistoryResult {Seed, PRs, Hunks[{File, StartLine, EndLine, Patch, Score}]}.",
		d.skill(), intent, d.source(),
	)
	d.record(ctx, contract.DummyInstruction{
		Backend:    "ckg",
		Operation:  "EvidenceForIntent",
		SkillPath:  d.skill(),
		SourcePath: d.source(),
		Query:      intent,
		Args:       args,
		Expected:   "contract.ChangeHistoryResult",
		Directive:  directive,
	})
	return contract.ChangeHistoryResult{Seed: opts.SeedQname}, nil
}

// GetNodePRs records a ckg.GetNodePRs instruction and returns no PRs.
func (d *Dummy) GetNodePRs(ctx context.Context, qname string, opts PRRefOpts) ([]contract.PRRef, error) {
	args := map[string]string{
		"max_count": fmt.Sprintf("%d", opts.MaxCount),
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to enumerate merge-commits that touched %q in go-stablenet "+
			"source at %s. Respond with a JSON array of contract.PRRef "+
			"{Number, Title, Summary, BaseSHA, HeadSHA, MergedAt, Repo}, newest first.",
		d.skill(), qname, d.source(),
	)
	d.record(ctx, contract.DummyInstruction{
		Backend:    "ckg",
		Operation:  "GetNodePRs",
		SkillPath:  d.skill(),
		SourcePath: d.source(),
		Query:      qname,
		Args:       args,
		Expected:   "[]contract.PRRef",
		Directive:  directive,
	})
	return nil, nil
}

// GetSubgraph records a ckg.GetSubgraph instruction and returns no nodes
// or edges.
func (d *Dummy) GetSubgraph(ctx context.Context, qname string, opts SubgraphOpts) ([]contract.Citation, []contract.Neighbor, error) {
	args := map[string]string{
		"depth":     fmt.Sprintf("%d", opts.Depth),
		"max_total": fmt.Sprintf("%d", opts.MaxTotal),
	}
	directive := fmt.Sprintf(
		"Use the skills under %s to walk every relation type around %q in go-stablenet source "+
			"at %s up to the requested depth. Respond with two JSON arrays: contract.Citation[] "+
			"(node set) and contract.Neighbor[] (edge set).",
		d.skill(), qname, d.source(),
	)
	d.record(ctx, contract.DummyInstruction{
		Backend:    "ckg",
		Operation:  "GetSubgraph",
		SkillPath:  d.skill(),
		SourcePath: d.source(),
		Query:      qname,
		Args:       args,
		Expected:   "([]contract.Citation, []contract.Neighbor)",
		Directive:  directive,
	})
	return nil, nil, nil
}

// Health reports the dummy as reachable without recording an
// instruction; health checks are part of bootstrap, not retrieval.
func (d *Dummy) Health(ctx context.Context) (Health, error) {
	return Health{
		Reachable:     true,
		SchemaVersion: "dummy",
	}, nil
}

// Close is a no-op; the dummy holds no resources.
func (d *Dummy) Close() error { return nil }

func (d *Dummy) record(ctx context.Context, i contract.DummyInstruction) {
	if c := contract.CollectorFrom(ctx); c != nil {
		c.Add(i)
	}
}

// placeholderHit returns a Hit with a sentinel Citation. Symmetric to
// ckvclient's placeholderHit; kept package-local so the two dummies
// stay independent.
func placeholderHit(src contract.HitSource) contract.Hit {
	return contract.Hit{
		Citation: placeholderCitation(),
		Rank:     1,
		Score:    1.0,
		Source:   src,
	}
}

func placeholderCitation() contract.Citation {
	return contract.Citation{
		File:      "DUMMY",
		StartLine: 1,
		EndLine:   1,
	}
}
