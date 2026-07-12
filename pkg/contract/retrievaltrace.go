package contract

// RetrievalStepKind classifies one retrieval action inside a RetrievalTrace.
//
// The set names the distinct backend operations the ckv→ckg funnel performs,
// so a trace produced by the deterministic Composer and one produced by the
// LLM agent describe their work in the same vocabulary. Values are persisted
// to evaluation reports and audit logs — renaming one is a breaking change
// (see the same note on Intent). New kinds may be appended as new operations
// are exercised (e.g. ckg.find_callers); existing values must not change.
type RetrievalStepKind string

const (
	// StepCKVRecall is a ckv semantic search — the meaning stage. Input is a
	// natural-language (possibly vocab-augmented) query; output is a set of
	// semantically related hits whose symbols seed the ckg stage.
	StepCKVRecall RetrievalStepKind = "ckv.recall"

	// StepCKGBM25 is a ckg BM25 keyword search/rerank — scores candidate
	// keywords by how strongly they hit the lexical index.
	StepCKGBM25 RetrievalStepKind = "ckg.bm25"

	// StepCKGFindSymbol is a ckg exact-symbol lookup by name.
	StepCKGFindSymbol RetrievalStepKind = "ckg.find_symbol"

	// StepCKGSubgraph is a ckg graph expansion (1-hop or deeper) from seed
	// citations — the "give me the related code as a graph" operation.
	StepCKGSubgraph RetrievalStepKind = "ckg.subgraph"

	// StepCKGImpact is a ckg impact-of-change query from a seed symbol.
	StepCKGImpact RetrievalStepKind = "ckg.impact"
)

// IsValid reports whether k is one of the known retrieval step kinds.
func (k RetrievalStepKind) IsValid() bool {
	switch k {
	case StepCKVRecall, StepCKGBM25, StepCKGFindSymbol, StepCKGSubgraph, StepCKGImpact:
		return true
	default:
		return false
	}
}

// RetrievalStep is one action in the ckv→ckg funnel.
//
// For the deterministic Composer (RetrievalTrace.Producer == "composer") a
// step is a Stage-1 recall-rerank round or a Stage-2/3 ckg lookup. For the
// LLM agent (Producer == "agent") a step is one cks tool call the model
// chose to make. The shared shape is what lets the two algorithms be scored
// against each other — the benchmark compares the process, not the format.
type RetrievalStep struct {
	// N is the 1-based position of this step within RetrievalTrace.Steps.
	N int `json:"n"`

	// Kind names the backend operation. See RetrievalStepKind.
	Kind RetrievalStepKind `json:"kind"`

	// Query is the query string or seed symbol this step acted on (the
	// augmented prompt for ckv.recall, a keyword/qname for the ckg kinds).
	Query string `json:"query"`

	// Source attributes the step to a backend (ckg or ckv). Derivable from
	// Kind but stored explicitly so consumers need not special-case the enum.
	Source HitSource `json:"source,omitempty"`

	// TopHits are the (truncated) results this step surfaced. Reusing Hit
	// keeps Symbol/CanonicalID — the ckv→ckg bridge fields — on each result.
	TopHits []Hit `json:"top_hits,omitempty"`

	// Keywords are the terms this step produced or consumed (e.g. the
	// BM25-reranked keyword list after a ckv recall round).
	Keywords []string `json:"keywords,omitempty"`

	// Confidence is the Stage-1 rerank concentration for this round
	// (top1_score / total_score); zero for steps that do not rerank.
	Confidence float64 `json:"confidence,omitempty"`

	// Decision records what the funnel did after this step: "accept" (seeds
	// are good enough), "augment" (loop with an expanded query), "expand"
	// (hand seeds to the graph stage), or "stop". Free-form but small.
	Decision string `json:"decision,omitempty"`
}

// RetrievalTrace is the standardized, JSON-serializable record of how a prompt
// was turned into code evidence by the ckv→ckg funnel.
//
// It is producer-agnostic by design: both the in-process deterministic
// Composer and the LLM-driven agent emit this exact shape, so an evaluation
// harness can compare the two retrieval algorithms on equal footing —
// measuring final-seed quality, backend call counts, rounds, and token cost
// without the format itself biasing the comparison. The assembled
// EvidencePack is returned separately; this trace explains how that pack's
// seeds were selected, and doubles as a "why was this picked" debug record.
type RetrievalTrace struct {
	// Producer attributes the trace to the algorithm that built it:
	// "composer" (deterministic, in-process) or "agent" (LLM tool use).
	Producer string `json:"producer"`

	// Intent is the coarse classification that routed the pipeline.
	Intent Intent `json:"intent"`

	// Prompt is the original user prompt (pre-expansion).
	Prompt string `json:"prompt"`

	// VocabExpanded / VocabKeywords record whether the glossary resolver
	// rewrote the prompt and which code keywords it spliced in.
	VocabExpanded bool     `json:"vocab_expanded,omitempty"`
	VocabKeywords []string `json:"vocab_keywords,omitempty"`

	// Steps is the ordered sequence of retrieval actions. Always >= 1 for a
	// valid trace.
	Steps []RetrievalStep `json:"steps"`

	// FinalSeeds are the citations the graph stage explored — the funnel's
	// distilled answer to "where is the relevant code".
	FinalSeeds []Citation `json:"final_seeds"`

	// FailedKeywords lists keywords that matched neither BM25 nor
	// find_symbol — a debugging signal for vocabulary gaps.
	FailedKeywords []string `json:"failed_keywords,omitempty"`

	// Rounds is the number of Stage-1 recall-rerank iterations (Composer) or
	// agent hops (agent).
	Rounds int `json:"rounds"`

	// CKVCalls / CKGCalls count backend round-trips, the headline cost
	// metric for comparing algorithms.
	CKVCalls int `json:"ckv_calls"`
	CKGCalls int `json:"ckg_calls"`

	// TokensIn is the size of context the retrieval consumed/emitted, when
	// the producer tracks it (the agent variant; zero for the Composer).
	TokensIn int `json:"tokens_in,omitempty"`
}

// IsValid reports whether t carries the minimum fields to be a meaningful
// trace: a non-empty prompt, an attributed producer, and at least one step.
func (t RetrievalTrace) IsValid() bool {
	if t.Prompt == "" || t.Producer == "" || len(t.Steps) == 0 {
		return false
	}
	for _, s := range t.Steps {
		if !s.Kind.IsValid() {
			return false
		}
	}
	return true
}
