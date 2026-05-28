package contract

import (
	"context"
	"sync"
)

// DummyInstruction is a directive emitted by a dummy ckv/ckg client in
// place of a real backend call. The Composer collects these during a
// Compose run and attaches them to EvidencePack.Instructions so the
// upstream LLM (coding-agent) can execute the corresponding skill
// against go-stablenet source and produce the response the real
// backend would have returned.
//
// When ckv/ckg are wired in, dummy clients are swapped for real ones
// and EvidencePack.Instructions stays empty.
type DummyInstruction struct {
	// Backend identifies which backend would have served this call.
	// Values: "ckv", "ckg".
	Backend string `json:"backend"`
	// Operation names the interface method that was invoked.
	// Examples: "SemanticSearch", "BM25Search", "FindSymbol",
	// "Neighbors", "ImpactOfChange", "EvidenceForIntent",
	// "GetNodePRs", "GetSubgraph".
	Operation string `json:"operation"`
	// SkillPath is the absolute path to the skill directory the LLM
	// should consult when fulfilling this instruction.
	SkillPath string `json:"skill_path"`
	// SourcePath is the absolute path to the source tree the LLM
	// should search/analyse.
	SourcePath string `json:"source_path"`
	// Query is the primary input passed to the backend method
	// (search string, symbol name, seed qname, etc.).
	Query string `json:"query"`
	// Args carries the remaining backend-method options serialised
	// as strings (k, language, path glob, kinds, depth, …). Strings
	// keep the wire stable across method shapes.
	Args map[string]string `json:"args,omitempty"`
	// Expected describes the shape of the response the LLM must
	// produce so the calling pipeline (or coding-agent) can splice
	// it back in as if the real backend had answered.
	Expected string `json:"expected"`
	// Directive is the natural-language instruction the LLM should
	// follow. Self-contained so the LLM does not need additional
	// context to act on it.
	Directive string `json:"directive"`
}

// InstructionCollector accumulates DummyInstructions emitted during a
// single Compose call. It is safe for concurrent use.
type InstructionCollector struct {
	mu    sync.Mutex
	items []DummyInstruction
}

// NewInstructionCollector returns an empty collector.
func NewInstructionCollector() *InstructionCollector {
	return &InstructionCollector{}
}

// Add records one instruction.
func (c *InstructionCollector) Add(i DummyInstruction) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = append(c.items, i)
}

// All returns a copy of every recorded instruction in invocation order.
func (c *InstructionCollector) All() []DummyInstruction {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]DummyInstruction, len(c.items))
	copy(out, c.items)
	return out
}

// Len reports the number of recorded instructions.
func (c *InstructionCollector) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

type collectorKey struct{}

// WithCollector returns a context carrying c. Dummy clients use
// CollectorFrom to attach instructions to the in-flight Compose call.
func WithCollector(ctx context.Context, c *InstructionCollector) context.Context {
	return context.WithValue(ctx, collectorKey{}, c)
}

// CollectorFrom returns the collector attached to ctx, or nil when
// none is present. Real clients ignore it; dummy clients use it to
// record their would-have-been calls.
func CollectorFrom(ctx context.Context) *InstructionCollector {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(collectorKey{})
	if v == nil {
		return nil
	}
	c, _ := v.(*InstructionCollector)
	return c
}
