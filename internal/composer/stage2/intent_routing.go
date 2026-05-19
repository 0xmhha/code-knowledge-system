package stage2

import "github.com/0xmhha/code-knowledge-system/pkg/contract"

// intentPathGlob returns a path glob for an intent-driven supplemental
// BM25 search, or "" when the intent has no such routing.
//
// The intent doc in pkg/contract/intent.go promises Stage 2 routing
// hints per intent ("Stage 2 (ckg): ..."). This is where we wire those
// hints in. Each non-empty entry triggers one extra BM25Search per
// keyword with the listed PathGlob, and the results merge into the
// existing aggregator. Path-filtered matches that also appear in the
// unfiltered pass double-count — that overlap is the boost the intent
// promises ("target symbol plus same-package *_test.go files").
//
// Currently wired:
//   - IntentTestAdd -> "*_test.go" (same-package test files)
//
// Deferred:
//   - IntentDocsUpdate -> README/godoc paths. ckg does not yet index
//     doc files, so a glob here would always return zero hits.
//   - IntentSecurity   -> handler/validator paths. No stable repo-wide
//     convention to glob on; routing here would over-fit.
//   - IntentQAReview   -> project-skill docs (.claude/docs/...). The
//     same external-doc dependency as DocsUpdate.
func intentPathGlob(intent contract.Intent) string {
	switch intent {
	case contract.IntentTestAdd:
		return "*_test.go"
	}
	return ""
}
