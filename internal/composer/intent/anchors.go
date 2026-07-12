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
		"X를 변경한 뒤에도 이전 값으로 동작하고, 새 값이 적용되지 않는다 — 근본 원인을 찾아 수정해줘",
		"A transaction that should be included stays stuck in the pending pool after a parameter change; find the root cause and fix it",
		"특정 조건에서 내부 상태가 갱신되지 않아 이후 검증이 계속 실패한다. 증상 재현 후 원인 위치를 확정해줘",
		"the setting was reverted but the system keeps enforcing the old threshold, blocking valid requests",
		"이 함수가 panic 발생",
		"X 에서 잘못된 값 반환",
		"intermittent crash on input Y",
		"function returns incorrect output",
		"memory leak in handler",
		"버그 수정해줘",
	},
	contract.IntentFeatureAdd: {
		"운영자가 X 값을 재시작 없이 설정으로 바꿀 수 있도록 옵션을 추가해줘",
		"Expose a new RPC endpoint that returns the current Y so monitoring can scrape it",
		"add support for X",
		"implement new endpoint for Y",
		"X 기능 추가",
		"새로운 retry policy 구현",
		"build a CLI flag for Z",
	},
	contract.IntentRefactor: {
		"중복 로직을 헬퍼 함수로 추출하고 가독성을 개선해줘",
		"이 패키지는 책임이 섞여 있어 테스트가 어렵다 — 구조를 분리하고 의존성을 주입 가능하게 정리해줘",
		"clean up the duplication in X",
		"extract helper from Y",
		"X 정리",
		"improve performance of Z",
		"성능 개선",
		"이 부분 리팩토링",
	},
	contract.IntentArchExplain: {
		"이 함수는 어떻게 동작하고 어디서 호출되는지 알려줘",
		"특정 심볼의 동작 원리와 호출 흐름 설명",
		"값 X가 어디서 생성되어 어떤 경로로 저장·소비되는지 전체 흐름을 설명해줘",
		"Explain how the fee value flows from the governance contract to the txpool admission check",
		"how does X work",
		"explain the flow of Y",
		"X 어떻게 동작",
		"what calls this function",
		"Y 의 구조 설명",
		"이 모듈의 역할",
	},
	contract.IntentTestAdd: {
		"이번 수정이 회귀하지 않도록 증상을 재현하는 테스트를 추가해줘",
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
		"이 diff가 기존 불변식을 깨지 않는지, 영향 범위와 함께 검토해줘",
		"review this PR",
		"check code standards on X",
		"PR 리뷰",
		"pre-merge review",
		"코드 리뷰",
		"리뷰 가이드라인 따라 검토",
	},
}
