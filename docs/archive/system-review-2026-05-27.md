# CKS 시스템 리뷰 — 유저 시나리오 기준 점검 (2026-05-27)

> **ARCHIVED 2026-07-19.** Point-in-time snapshot; live status is [`remaining-work.md`](../remaining-work.md), durable architecture map is [`system-context-map.md`](../system-context-map.md). Kept for provenance.

> **목적**: 유저 시나리오(Jira ticket → 코드 구현 → PR → Merge)를 만족시키기 위해
> 필요한 기술·모듈·기능을 정리하고, 현재 코드 수준을 점검하며, 모듈별 필요 작업을 도출한다.

---

## 1. 유저 시나리오 (End-to-End Workflow)

```
┌─────────────────────────────────────────────────────────────────┐
│  Phase 1: 작업 인식 (Jira → Agent)                               │
├─────────────────────────────────────────────────────────────────┤
│  1. 담당자가 Claude Code plugin 커맨드 실행 (Jira ticket 번호)      │
│  2. Jira MCP로 ticket 내용 읽기                                   │
│  3. 민감정보 검토 (skill 기반 1차 + Sonnet 모델 2차)                │
│  4. go-stablenet 프로젝트 관련 내용임을 인식                        │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│  Phase 2: 정보 수집 (CKS MCP → CKV + CKG)                       │
├─────────────────────────────────────────────────────────────────┤
│  5. CKS MCP에 ticket 내용을 query로 전달                          │
│  6. CKV: 의미 검색으로 "어떤 작업인지" 파악 (intent + keywords)     │
│  7. Rerank 과정으로 accuracy 향상                                 │
│  8. CKG: 키워드 관련 코드 검색 (symbol, graph traversal)           │
│  9. CKG: 코드 수정 히스토리 (PR breadcrumb, hunk evidence)         │
│ 10. CKG: 동시성 영향 분석 (lock, channel, mutex 관련 코드)          │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│  Phase 3: 계획 수립 (Agent + Claude Model)                       │
├─────────────────────────────────────────────────────────────────┤
│ 11. 수집 정보 종합 → 작업 단계 수립                                │
│ 12. 수정 코드 + 테스트 정의 + 우선순위 배정                         │
│ 13. 전용 폴더 생성 ({jira-ticket}-{timestamp}/)                   │
│ 14. 플랜 문서 + 디버깅 로그 경로 설정                               │
│ 15. 정밀 설계 문서 작성 + 반복 검토 (버전 관리)                      │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│  Phase 4: 코드 구현 (Implementation Agent)                       │
├─────────────────────────────────────────────────────────────────┤
│ 16. 구현 전문 agent가 설계 문서 기반 코드 작업                       │
│ 17. 분할 커밋 (리뷰 편의), 별도 브랜치                              │
│ 18. Hook 활용한 무중단 iteration                                  │
│ 19. 구현 완료 → 이전 agent에 완료 통보                              │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│  Phase 5: 검증 (Evaluation Agent)                                │
├─────────────────────────────────────────────────────────────────┤
│ 20. Unit test, lint, fmt 실행                                    │
│ 21. 보안 취약점 검토                                              │
│ 22. Chainbench MCP로 로컬 네트워크 구성 (수정 바이너리)               │
│ 23. 블록 생성·TX 테스트 수행                                       │
│ 24. 테스트 결과 리포트 생성                                        │
│ 25. 실패 시 → Phase 2로 복귀 (단, 미푸시 코드 반영 필요)             │
└───────────────────────────┬─────────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────────┐
│  Phase 6: PR 생성 + 코드 리뷰 + Merge                            │
├─────────────────────────────────────────────────────────────────┤
│ 26. PR 생성 → Jira ticket에 PR URL 댓글 업데이트                   │
│ 27. 인간 개발자 코드 리뷰                                         │
│ 28. 수정 요청 시 → plugin 커맨드로 리뷰 코멘트 읽고 수정 작업         │
│ 29. 반복 수정 → 모든 리뷰어 승인                                   │
│ 30. Squash merge → Jira ticket Complete 상태 변경                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. 시나리오 단계별 필요 모듈과 현재 상태

### 2.1 필요 모듈 매핑

| Phase | 단계 | 필요 모듈 | 유형 |
|-------|------|----------|------|
| 1 | Jira 읽기 | Jira MCP (Atlassian) | MCP |
| 1 | 민감정보 검토 | Sanitization skill | Claude Code Skill |
| 1 | Sonnet 2차 검토 | Claude Code model routing | Agent |
| 1 | 프로젝트 인식 | Project routing skill | Claude Code Skill |
| 2 | CKS query | CKS MCP (get_for_task) | MCP |
| 2 | 의미 검색 | CKV (semantic_search) | Backend |
| 2 | Rerank | CKV BM25 rerank + CKS Stage 2 | Backend |
| 2 | 코드 검색 | CKG (find_symbol, search_text) | Backend |
| 2 | 히스토리 | CKG (evidence_for_intent, PR breadcrumb) | Backend |
| 2 | 동시성 분석 | CKG (impact_of_change) | Backend |
| 3 | 계획 수립 | Planning agent | Agent |
| 3 | 폴더·문서 관리 | Workspace manager skill | Skill |
| 3 | 설계 문서 반복 검토 | Design review agent | Agent |
| 4 | 코드 구현 | Implementation agent | Agent |
| 4 | 분할 커밋 | Git operation skill | Skill |
| 4 | 무중단 iteration | Claude Code hook | Hook |
| 5 | Unit test/lint/fmt | Go toolchain integration | Skill |
| 5 | 보안 검토 | Security review skill | Skill |
| 5 | 네트워크 테스트 | Chainbench MCP | MCP |
| 5 | 결과 리포트 | Report generation skill | Skill |
| 5 | 실패 복귀 (미푸시 코드) | Local diff + CKS hybrid | Agent + Backend |
| 6 | PR 생성 | GitHub MCP / gh CLI | MCP |
| 6 | Jira 댓글 업데이트 | Jira MCP | MCP |
| 6 | 리뷰 코멘트 읽기 | GitHub MCP | MCP |
| 6 | 리뷰 반영 수정 | Review handler agent | Agent |
| 6 | Squash merge | Git operation skill | Skill |
| 6 | Jira 상태 변경 | Jira MCP | MCP |

### 2.2 현재 상태 요약

| 모듈 | 상태 | 비고 |
|------|------|------|
| **CKS MCP** | ⚠️ 동작하지만 실효성 부족 | FakeEmbedder 사용 중 → 의미 검색 신호 없음 |
| **CKV** | ⚠️ 코드 완성도 높지만 실전 미검증 | mock embedder로만 테스트됨. 실제 semantic signal 없음 |
| **CKG** | ✅ 가장 성숙 | 8 MCP tools, BM25, graph, PR breadcrumb 모두 동작 |
| **Claude Code Plugin** | ❌ 미존재 | 전체 워크플로우 진입점이 없음 |
| **Agent 아키텍처** | ❌ 미존재 | Planning/Implementation/Evaluation agent 미설계 |
| **Skill 세트** | ❌ 미존재 | 민감정보 검토, 워크스페이스 관리, 리포트 생성 등 |
| **Hook 설정** | ❌ 미존재 | 무중단 iteration을 위한 hook 미설정 |
| **Jira MCP** | ✅ 존재 | Atlassian MCP plugin 설치됨 (인증 필요) |
| **GitHub MCP** | ✅ 존재 | gh CLI 사용 가능 |
| **Chainbench MCP** | ❌ 미존재 | 로컬 네트워크 테스트 인프라 미구축 |
| **go-stablenet 데이터** | ❌ 미구축 | CKV/CKG 인덱스가 go-stablenet 대상으로 빌드되지 않음 |

---

## 3. 핵심 문제 진단

### 3.1 근본 원인: 실제 의미 검색이 동작하지 않음

3주간 작업의 핵심 병목은 **실제 semantic signal이 없다**는 점이다.

```
현재 상태:
  사용자 질문 → [FakeEmbedder (hash 기반)] → 무작위 유사도 → 무의미한 결과
  
필요한 상태:
  사용자 질문 → [Real Embedder (코드 학습됨)] → 의미적 유사도 → 정확한 코드 검색
```

| 컴포넌트 | 현재 | 문제 |
|----------|------|------|
| CKV embedder | FakeEmbedder (32d hash) | 의미 신호 없음. 동일 토큰 공유 시에만 유사도 발생 |
| CKS intent classifier | FakeEmbedder 사용 | intent 분류 정확도 보장 불가 |
| CKS Stage 1 (keyword extraction) | CKV semantic search 의존 | CKV가 무의미한 결과 → Stage 1도 무의미 |
| 평가 (dogfood-eval) | cks가 자기 자신을 인덱싱 | go-stablenet 대상 평가가 아님 |

**결론**: CKV/CKG 코드 자체는 상당히 완성되어 있지만, **실제 의미 검색 엔진이 없어서
전체 파이프라인의 품질을 측정할 수조차 없는 상태**이다.

### 3.2 평가가 어려운 이유

| 원인 | 영향 |
|------|------|
| FakeEmbedder 사용 중 | 의미 검색 결과가 무작위 → recall/precision 무의미 |
| self-indexing (cks→cks) | go-stablenet 도메인 특성 반영 안 됨 |
| 실제 사용자 쿼리 부재 | 시나리오가 가상. 실제 Jira ticket 기반 테스트 없음 |
| 한국어 쿼리 미지원 | embedder가 한국어 처리 못 함 (FakeEmbedder는 언어 무관, 실제 모델은 영어 중심) |
| E2E 검증 경로 부재 | Jira → CKS → 코드 수정 → 테스트 → PR 전체 경로 테스트 불가 |

### 3.3 방향은 올바른가?

**아키텍처 방향은 올바르다.** 구체적으로:

| 설계 결정 | 판단 | 근거 |
|----------|------|------|
| 3-repo 분리 (cks/ckv/ckg) | ✅ 올바름 | 각 모듈이 독립적으로 발전 가능 |
| CKS = orchestrator (LLM 호출 금지) | ✅ 올바름 | 결정론적 평가 가능 |
| CKV = vector, CKG = graph+BM25 | ✅ 올바름 | 명확한 책임 분리 |
| 순차 파이프라인 (Stage 1→2→3→4→5) | ✅ 올바름 | 디버깅·footprint 추적 용이 |
| EvidencePack + integrity hash | ✅ 올바름 | 검증 가능한 출력 |
| Intent 기반 라우팅 (10개 intent) | ✅ 올바름 | 사용자 시나리오의 다양한 작업 유형 커버 |

**문제는 방향이 아니라 실행 순서이다.** 기반(embedder)이 없는 상태에서
상위 레이어(glossary, query expansion, reranker)를 설계하고 있었다.

---

## 4. 모듈별 기술 요구사항과 현재 구현 수준

### 4.1 CKV (Vector/Semantic Search)

| 기능 | 유저 시나리오에서의 역할 | 필요 기술 | 현재 상태 | Gap |
|------|----------------------|----------|----------|-----|
| **Real embedder** | 의미 검색의 기반 | Ollama + qwen3-embedding | ❌ FakeEmbedder만 사용 | **P0 블로커** |
| Semantic search | "어떤 작업인지" 파악 | vector cosine search | ✅ 구현됨 (sqlite-vec) | embedder에 의존 |
| Symbol-level chunking | 코드를 의미 단위로 분할 | AST parser | ✅ Go/TS/Sol/MD 지원 | — |
| BM25 rerank | accuracy 향상 | Okapi BM25 + RRF | ✅ 구현됨 (optional) | — |
| Glossary/Alias | 한국어→코드 키워드 변환 | 용어 사전 YAML | ✅ CLI 존재 | go-stablenet 사전 미작성 |
| Incremental reindex | 코드 변경 반영 | git diff 기반 | ✅ 구현됨 | — |
| Citation enforcement | 결과의 실재 검증 | file existence + line check | ✅ 구현됨 | — |
| go-stablenet 인덱스 | 실제 대상 코드 인덱싱 | ckv build on go-stablenet | ❌ 미구축 | **P0 블로커** |

### 4.2 CKG (Graph + BM25)

| 기능 | 유저 시나리오에서의 역할 | 필요 기술 | 현재 상태 | Gap |
|------|----------------------|----------|----------|-----|
| BM25 search | 키워드 기반 코드 검색 | FTS5 + Okapi | ✅ 완전 동작 | — |
| Symbol resolution | 정확한 심볼 찾기 | qname exact/suffix | ✅ 완전 동작 | — |
| Graph traversal | 관련 코드 확장 | BFS + edge type filter | ✅ depth=2 기본 | — |
| PR breadcrumb | 코드 수정 히스토리 | git log --merges 파싱 | ✅ schema 1.12 | — |
| Impact analysis | 동시성 영향 분석 | reverse-dep closure | ✅ 6개 카테고리 | — |
| Evidence pack | hunk 기반 증거 조립 | BM25-rank hunks | ✅ H3 구현 | — |
| Lock propagation | cross-function lock 분석 | SSA-ish 분석 | ⚠️ Stage A만 | Go 고급 동시성 분석 제한적 |
| go-stablenet 인덱스 | 실제 대상 코드 그래프 | ckg build on go-stablenet | ⚠️ eval/stablenet에 존재 | 최신 상태 확인 필요 |

### 4.3 CKS (Orchestrator)

| 기능 | 유저 시나리오에서의 역할 | 필요 기술 | 현재 상태 | Gap |
|------|----------------------|----------|----------|-----|
| MCP server | Agent가 호출하는 인터페이스 | mcp-go stdio | ✅ 2 tools | 시나리오에 필요한 추가 tools |
| Intent classification | 작업 유형 자동 분류 | embedding cosine | ✅ 10 intents | Real embedder 의존 |
| Stage 1 (keyword extraction) | 핵심 키워드 도출 | CKV + iterative refine | ✅ 구현됨 | Real embedder 의존 |
| Stage 2 (citation search) | 정확한 코드 위치 | CKG BM25 + FindSymbol | ✅ 구현됨 | — |
| Stage 3 (graph expansion) | 관련 코드 확장 | CKG Neighbors | ✅ 구현됨 | — |
| Stage 4 (budget allocation) | 토큰 예산 내 선별 | greedy fit | ✅ 구현됨 | — |
| Stage 5 (sanitization) | 민감정보 제거 | regex rules | ✅ 14 rules | — |
| Vocabulary resolver | 모호 쿼리→정확 키워드 | glossary + expansion | ❌ 미구현 | T1.B/C 미착수 |
| Config for go-stablenet | 대상 프로젝트 설정 | YAML config | ⚠️ 예시만 존재 | 실제 설정 필요 |

### 4.4 Claude Code Plugin (미존재)

| 기능 | 유저 시나리오에서의 역할 | 필요 기술 | 현재 상태 |
|------|----------------------|----------|----------|
| Plugin manifest | 진입점, 커맨드 등록 | plugin.json | ❌ |
| Jira ticket 커맨드 | `/stablenet-task JIRA-123` | Command + Jira MCP | ❌ |
| PR review 커맨드 | `/stablenet-review PR-URL` | Command + GitHub MCP | ❌ |
| Planning agent | 정보 수집 → 계획 수립 | Agent definition | ❌ |
| Implementation agent | 설계 기반 코드 구현 | Agent definition | ❌ |
| Evaluation agent | 테스트·검증 수행 | Agent definition | ❌ |
| Review handler agent | PR 리뷰 코멘트 반영 | Agent definition | ❌ |
| Workspace skill | 폴더·문서·로그 관리 | Skill definition | ❌ |
| Security skill | 민감정보 1차 검토 | Skill definition | ❌ |
| Report skill | 결과 리포트 생성 | Skill definition | ❌ |
| Iteration hook | 무중단 agent 실행 | Hook definition | ❌ |

### 4.5 외부 의존 (MCP / Infrastructure)

| 모듈 | 역할 | 현재 상태 | 필요 작업 |
|------|------|----------|----------|
| Jira MCP (Atlassian) | ticket 읽기/쓰기 | ✅ plugin 설치됨 | 인증 설정 |
| GitHub MCP / gh CLI | PR 생성/리뷰 읽기 | ✅ 사용 가능 | — |
| Chainbench MCP | 로컬 체인 네트워크 테스트 | ❌ 미존재 | 설계·구현 필요 |
| Ollama | 임베딩 모델 추론 서버 | ❌ 미설정 | 설치 + 모델 pull |

---

## 5. 검증 전략

### 5.1 가장 현실적인 검증 방법: 기존 PR 기반 회귀 테스트

go-stablenet에 이미 merge된 PR들을 활용한다:

```
1. PR merge 직전 코드로 checkout
2. 해당 시점 기준으로 CKV/CKG 인덱스 구축
3. PR 내용(버그 수정 설명 또는 신규 기능 요구)을 쿼리로 입력
4. 시스템의 응답을 실제 PR 코드 변경과 비교
```

### 5.2 검증 관점 (각 Phase별)

| 검증 관점 | 측정 항목 | 방법 |
|----------|----------|------|
| **문제 인식** (Phase 2-①) | "어떤 작업인가" 정확 분류 | intent classification 결과 vs PR label |
| **코드 위치** (Phase 2-②) | "어디를 수정해야 하나" | CKS 반환 citations vs PR changed files |
| **관련 코드** (Phase 2-③) | "연관 코드는 무엇인가" | graph expansion 결과 vs PR에서 함께 수정된 파일 |
| **히스토리** (Phase 2-④) | "과거에 어떤 수정이 있었나" | PR breadcrumb 결과 vs 실제 관련 PR |
| **계획 수립** (Phase 3) | "수정 계획이 합리적인가" | LLM judge (E3 PlanStepsScore) |
| **구현** (Phase 4) | "코드가 올바르게 수정되었나" | diff similarity + test pass |
| **검증** (Phase 5) | "수정이 사이드이펙트 없는가" | unit test + chainbench |

### 5.3 검증을 위해 필요한 기능

| 기능 | 위치 | 현재 상태 | 필요 작업 |
|------|------|----------|----------|
| PR fixture (YAML) | CKV eval/prregress | ✅ 4개 존재 | 12개로 확장 (ckv-NEW-5) |
| PR regression runner | CKV internal/eval/prregress | ✅ 구현됨 | Real embedder 연결 |
| CKS eval scenarios | CKS eval/scenarios | ✅ 9개 (self-indexing) | go-stablenet 시나리오 추가 |
| Intent accuracy metric | CKV E1 (IntentScore) | ✅ token-F1 | — |
| File location accuracy | CKV E2 (SymbolF1) | ✅ file/symbol P/R | — |
| Plan quality metric | CKV E3 (PlanStepsScore) | ⚠️ LLM judge 의존 | LLM 연동 필요 |
| Per-stage footprint | CKS internal/footprint | ✅ JSONL events | — |
| End-to-end latency | CKS footprint | ✅ compose_complete event | — |
| Chainbench 통합 테스트 | Chainbench MCP | ❌ 미존재 | 설계·구현 필요 |

### 5.4 Footprint 설정 (성능 추적용)

이미 CKS에 footprint 시스템이 구현되어 있으며, 다음 이벤트를 추적한다:

| Event | 추적 내용 | 활용 |
|-------|----------|------|
| `composer.intent_classified` | intent, confidence, best_anchor | intent 분류 정확도 모니터링 |
| `composer.stage1_extracted` | keywords, rounds, confidence | keyword 추출 품질 추적 |
| `composer.stage2_searched` | hit count, coverage, failed keywords | code search 범위 추적 |
| `composer.compose_complete` | total latency, utilization | E2E 성능 추적 |

**Gap**: Phase 3-6 (planning, implementation, testing, PR)에 대한 footprint가 없다.
이 부분은 Claude Code plugin/agent 레벨에서 별도로 구현해야 한다.

---

## 6. 기술 스택 정리

### 6.1 핵심 기술 (시나리오 동작에 필수)

| 기술 | 적용 위치 | 역할 | 현재 | 필요 작업 |
|------|----------|------|------|----------|
| **Ollama + qwen3-embedding** | CKV embedder | 코드 의미 검색의 기반 | ❌ | Ollama 어댑터 구현 |
| **sqlite-vec** | CKV storage | 벡터 저장 + cosine search | ✅ | — |
| **Okapi BM25** | CKG search, CKV rerank | 키워드 기반 정확 검색 | ✅ | — |
| **FTS5** | CKG full-text index | symbol/signature/doc 검색 | ✅ | — |
| **mcp-go** | CKS/CKV/CKG server | MCP stdio 통신 | ✅ | — |
| **tree-sitter** | CKV/CKG parser | Go/TS/Sol AST 파싱 | ✅ | — |
| **Claude Code Plugin SDK** | Plugin 진입점 | 커맨드·agent·skill·hook 등록 | ❌ | 전체 설계·구현 |
| **Atlassian MCP** | Jira 연동 | ticket 읽기/쓰기/상태 변경 | ✅ (설치됨) | 인증 설정 |

### 6.2 보조 기술 (품질 향상)

| 기술 | 적용 위치 | 역할 | Tier |
|------|----------|------|------|
| Vocabulary glossary | CKS vocab resolver | 한국어→코드 키워드 | T1 |
| Query expansion | CKS Stage 1 전처리 | 쿼리 다양화 | T1 |
| Cross-encoder reranker | CKS fusion 직전 | top-30 → top-5 정밀 선별 | T2 |
| PR corpus indexing | CKV | PR description 의미 검색 | T2 |
| HyDE | Coding agent layer | 가상 답변 생성 → 검색 | T3 |

---

## 7. 모듈별 필요 작업 정리

### 7.1 CKV — 즉시 필요 작업

| 순서 | 작업 | 설명 | 의존 | 예상 규모 |
|------|------|------|------|----------|
| **V-1** | Ollama embedder 어댑터 | `internal/embed/ollama/` 신규. Embedder 인터페이스 구현, `localhost:11434/api/embed` 호출 | Ollama 설치 | ~150 LOC |
| **V-2** | qwen3-embedding:0.6b 연동 테스트 | `ollama pull` + 기존 eval fixture로 recall 측정 | V-1 | 테스트 |
| **V-3** | go-stablenet 인덱스 구축 | `ckv build --src=<go-stablenet> --out=<path> --embedder=ollama` | V-1 | 빌드 실행 |
| **V-4** | go-stablenet glossary 작성 | 한국어/도메인 용어 → 코드 키워드 YAML | 도메인 지식 | YAML |
| **V-5** | PR fixture 12개 확장 | go-stablenet의 실제 fix/feature PR 기반 | V-3 | YAML |
| **V-6** | Real embedder 기준 eval 실행 | recall@5, MRR, hallucination rate 측정 | V-2, V-3, V-5 | 테스트 |

### 7.2 CKG — 확인/보완 작업

| 순서 | 작업 | 설명 | 의존 | 예상 규모 |
|------|------|------|------|----------|
| **G-1** | go-stablenet 인덱스 최신화 | `ckg build --src=<go-stablenet>` 실행, 현재 인덱스 상태 확인 | — | 빌드 실행 |
| **G-2** | CKS에서 사용하는 store.Reader API 검증 | BM25Search, FindSymbol, Neighbors가 go-stablenet에서 정확한 결과 반환하는지 | G-1 | 테스트 |
| **G-3** | PR breadcrumb 검증 | go-stablenet PR 히스토리가 올바르게 파싱되었는지 | G-1 | 테스트 |
| **G-4** | Score 필드 노출 (F-1 해결) | BM25 raw score를 store.Reader API에 노출 (CKS 합성 점수 제거) | — | ~50 LOC |

### 7.3 CKS — 통합 및 보완 작업

| 순서 | 작업 | 설명 | 의존 | 예상 규모 |
|------|------|------|------|----------|
| **S-1** | Ollama embedder를 intent classifier에 연결 | FakeEmbedder → Ollama embedder 교체 | V-1 | ~30 LOC (wiring) |
| **S-2** | go-stablenet 대상 config 작성 | CKG path, CKV path, real embedder 설정 | V-3, G-1 | YAML |
| **S-3** | go-stablenet 대상 eval 시나리오 작성 | 실제 Jira ticket 유사 쿼리 기반 시나리오 | S-2 | YAML |
| **S-4** | Vocabulary resolver 구현 | `internal/vocab/resolver.go` — glossary 로드 + query expansion | V-4 | ~200 LOC |
| **S-5** | E2E dogfood eval (go-stablenet) | `make dogfood-eval`을 go-stablenet 대상으로 실행 | S-2, S-3 | 테스트 |
| **S-6** | 미푸시 코드 처리 | Phase 5 실패 복귀 시, local diff를 CKS 컨텍스트에 포함하는 방법 | — | 설계 필요 |

### 7.4 Claude Code Plugin — 신규 개발

| 순서 | 작업 | 설명 | 의존 | 예상 규모 |
|------|------|------|------|----------|
| **P-1** | Plugin 구조 설계 | plugin.json, 커맨드, agent, skill, hook 정의 | — | 설계 문서 |
| **P-2** | `/stablenet-task` 커맨드 | Jira ticket 번호 입력 → Phase 1~2 실행 | S-2 | ~100 LOC |
| **P-3** | Planning agent | Phase 2 결과 → Phase 3 (계획·설계 문서 생성) | P-2 | agent 정의 |
| **P-4** | Implementation agent | Phase 3 결과 → Phase 4 (코드 구현) | P-3 | agent 정의 |
| **P-5** | Evaluation agent | Phase 4 결과 → Phase 5 (테스트·검증) | P-4 | agent 정의 |
| **P-6** | `/stablenet-review` 커맨드 | PR URL → 리뷰 코멘트 읽기 → 수정 작업 | — | ~80 LOC |
| **P-7** | Workspace manager skill | 폴더 생성, 문서 버전 관리, 로그 경로 설정 | — | skill 정의 |
| **P-8** | Iteration hook | agent 간 전환, 무중단 실행 | — | hook 정의 |

### 7.5 외부 인프라 — 구축 필요

| 순서 | 작업 | 설명 | 의존 | 예상 규모 |
|------|------|------|------|----------|
| **I-1** | Ollama 설치 + qwen3-embedding pull | `brew install ollama && ollama pull qwen3-embedding:0.6b` | — | 설치 |
| **I-2** | Jira MCP 인증 설정 | Atlassian MCP plugin 인증 완료 | — | 설정 |
| **I-3** | Chainbench MCP 설계 | 로컬 go-stablenet 네트워크 테스트 MCP | — | 설계 문서 |

---

## 8. 우선순위 및 의존 관계

### 8.1 Critical Path (이것 없이는 아무것도 검증 불가)

```
I-1 (Ollama 설치)
  → V-1 (Ollama embedder 어댑터)
    → V-2 (qwen3-embedding 연동 테스트)
      → V-3 (go-stablenet CKV 인덱스 구축)
      → G-1 (go-stablenet CKG 인덱스 확인)
        → S-1 (CKS intent classifier 연결)
        → S-2 (go-stablenet config)
          → S-5 (E2E dogfood eval)
```

**이 경로가 해소되어야 "시스템이 실제로 동작하는가"를 판단할 수 있다.**

### 8.2 Phase 순서 제안

| Phase | 목표 | 작업 | 완료 기준 |
|-------|------|------|----------|
| **Phase α** | Real embedder 동작 | I-1, V-1, V-2 | qwen3-embedding으로 recall 측정 가능 |
| **Phase β** | go-stablenet 검증 | V-3, G-1, S-1, S-2, S-5 | go-stablenet 대상 recall > 0.6 |
| **Phase γ** | 품질 향상 | V-4, S-4, G-4, V-5, V-6, S-3 | go-stablenet 대상 recall > 0.8 |
| **Phase δ** | Plugin MVP | P-1, P-2, P-3 | Jira ticket → 계획 문서 생성 자동화 |
| **Phase ε** | E2E 자동화 | P-4~P-8, I-2, I-3 | 전체 시나리오 자동 실행 |

### 8.3 3주간 진행 결과 요약

```
수행한 것:
  ✅ CKS 6-stage composer pipeline 완전 구현
  ✅ CKV MCP 4 tools + chunking + storage 완전 구현
  ✅ CKG 8 MCP tools + graph + BM25 + PR breadcrumb 완전 구현
  ✅ 평가 프레임워크 (cks-eval 9 scenarios, ckv eval + prregress)
  ✅ Footprint/AuditLog 시스템
  ✅ Sanitization 14 rules
  ✅ Intent classification 10 intents

아직 안 된 것:
  ❌ Real embedder 연동 (전체 파이프라인 품질의 기반)
  ❌ go-stablenet 대상 인덱스 구축 및 평가
  ❌ Claude Code Plugin (전체 워크플로우 진입점)
  ❌ Agent 아키텍처 (Planning/Implementation/Evaluation)
  ❌ Chainbench 통합 테스트
  ❌ Vocabulary resolver (한국어→코드 매핑)
```

---

## 9. 결론

### 현재 위치

3개 레포의 **코드 인프라는 80% 이상 완성**되어 있다. CKS의 6-stage pipeline,
CKV의 vector storage + chunking, CKG의 graph + BM25 + MCP 모두 구현되어 동작한다.

그러나 **실제 의미 검색 엔진(Real Embedder)이 없어서**, 전체 파이프라인이
의미 있는 결과를 생성하지 못하고 있으며, 이 때문에 평가도 실효성이 없다.

### 즉시 필요한 조치

1. **Ollama + qwen3-embedding:0.6b 연동** — 이것 하나로 CKV의 semantic search,
   CKS의 intent classification, Stage 1 keyword extraction이 모두 실제 동작 시작
2. **go-stablenet 인덱스 구축** — 실제 대상 코드로 검증 가능한 상태 만들기
3. **E2E 평가 실행** — 실제 데이터로 recall/precision 측정하여 현 수준 파악

이 3가지가 완료되면, 그 다음 작업(glossary, query expansion, plugin, agent)의
우선순위를 **데이터 기반으로** 결정할 수 있다.
