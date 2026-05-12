package intent

import "github.com/0xmhha/code-knowledge-system/pkg/contract"

// defaultAnchors lists representative prompt examples for each Intent.
//
// At Classifier construction time each anchor is embedded once and cached.
// At classify time, the input prompt is embedded and compared by cosine
// similarity to every anchor; the Intent of the closest anchor wins
// (subject to the unknown threshold).
//
// Curation guidance:
//   - Aim for 5-7 anchors per Intent.
//   - Mix Korean and English freely — a multilingual embedder spans both,
//     so adding one Korean and one English anchor is cheaper than writing
//     two rule sets.
//   - Cover the surface forms that real users actually type. The Phase-0
//     set below is a starter; Phase E (evaluation) measures miscluster
//     rates and feeds the data back here.
var defaultAnchors = map[contract.Intent][]string{
	contract.IntentBugFix: {
		"이 함수가 panic 발생",
		"X 에서 잘못된 값 반환",
		"intermittent crash on input Y",
		"function returns incorrect output",
		"memory leak in handler",
		"버그 수정해줘",
	},
	contract.IntentFeatureAdd: {
		"add support for X",
		"implement new endpoint for Y",
		"X 기능 추가",
		"새로운 retry policy 구현",
		"build a CLI flag for Z",
	},
	contract.IntentRefactor: {
		"clean up the duplication in X",
		"extract helper from Y",
		"X 정리",
		"improve performance of Z",
		"성능 개선",
		"이 부분 리팩토링",
	},
	contract.IntentArchExplain: {
		"how does X work",
		"explain the flow of Y",
		"X 어떻게 동작",
		"what calls this function",
		"Y 의 구조 설명",
		"이 모듈의 역할",
	},
	contract.IntentTestAdd: {
		"add tests for X",
		"cover Y with unit tests",
		"X 에 테스트 추가",
		"table-driven tests for Z",
		"테스트 커버리지 보완",
	},
	contract.IntentConcurrencySafety: {
		"is X safe under concurrent calls",
		"race condition in Y",
		"X 동시성 점검",
		"deadlock check on Z",
		"data race detection",
		"goroutine leak 의심",
	},
	contract.IntentSecurity: {
		"check X for vulnerabilities",
		"audit Y for security issues",
		"X 취약점 점검",
		"auth flow audit",
		"SQL injection 가능성",
		"input validation 검토",
	},
	contract.IntentDocsUpdate: {
		"update docs for X",
		"explain Y in README",
		"X 문서 갱신",
		"godoc 추가",
		"documentation update",
	},
	contract.IntentQAReview: {
		"review this PR",
		"check code standards on X",
		"PR 리뷰",
		"pre-merge review",
		"코드 리뷰",
		"리뷰 가이드라인 따라 검토",
	},
}
