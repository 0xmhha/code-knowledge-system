# 기술 분석 — 유저 시나리오 구현을 위한 기술 평가와 베스트 프랙티스 (2026-05-27)

> **문서 성격(2026-05-27 시점 스냅샷)**: 본 문서는 2026-05-27 기준 point-in-time 기술
> 분석이며 지식 기준은 2025년 1월까지다. 설계 근거/베스트 프랙티스 레퍼런스로 보존하며,
> 여기 적힌 구현 상태(✅/⚠️/❌)는 그 시점의 것이다 — 현재 상태는 `docs/remaining-work.md`가
> 정본이다.

> **목적**: 유저 시나리오(Jira ticket → 코드 구현 → PR → Merge)를 구현하기 위한
> 기술적 접근법이 올바른지 평가하고, 더 나은 방법이 있다면 제시하며,
> CKV/CKG 내부 시스템의 베스트 프랙티스를 설명한다.
>
> **지식 기준**: 본 문서의 기술 설명은 **2025년 1월까지의 논문·기술·제품**을 기준으로 한다.
> 2025년 2월 이후 발표된 기술은 누락될 수 있으며, 각 항목에 출처와 시점을 명기한다.
>
> **관련 문서**: `docs/archive/system-review-2026-05-27.md` (현재 코드 상태 점검)

---

## 1. 유저 시나리오의 기술적 분해

유저 시나리오는 두 가지 기술 영역의 결합이다:

```
[A] Code RAG (정보 수집)        → CKV + CKG + CKS
[B] Agentic Coding (작업 수행)   → Plugin + Agent + Skill + Hook
```

### 1.1 Code RAG 란

RAG(Retrieval-Augmented Generation)는 LLM이 답변할 때, 외부 지식 저장소에서
관련 정보를 먼저 검색(Retrieve)해서 프롬프트에 포함(Augment)시킨 뒤
생성(Generate)하는 패턴이다 (Lewis et al., 2020).

LLM의 학습 데이터에 없는 비공개 코드(go-stablenet)를 다루려면 RAG가 필수이며,
유저 시나리오에서 CKS/CKV/CKG가 담당하는 역할이 바로 이 Code RAG이다.

### 1.2 Agentic Coding 이란

LLM이 단순 질답이 아닌, 도구를 사용하며 자율적으로 작업을 수행하는 패턴이다.
유저 시나리오에서 Planning/Implementation/Evaluation Agent가 이 역할을 맡는다.

대표 연구:
- ReAct: Reasoning + Acting (Yao et al., 2022.10)
- Plan-and-Execute: 계획과 실행 분리 (Wang et al., 2023)
- Reflexion: 실패로부터 학습 (Shinn et al., 2023.03)
- SWE-Agent: GitHub 이슈를 AI가 해결 (Princeton, 2024.02)

---

## 2. 현재 접근법 평가

### 2.1 올바른 설계 결정

| 설계 결정 | 평가 | 근거 |
|----------|------|------|
| 3-repo 분리 (cks/ckv/ckg) | ✅ 올바름 | 각 모듈 독립 발전 가능. 책임 경계 명확 |
| CKS = orchestrator (LLM 호출 금지) | ✅ 올바름 | RAG 품질을 LLM 변동성과 분리. 결정론적 평가 가능 |
| CKV = vector, CKG = graph+BM25 | ✅ 올바름 | Hybrid Search의 정석적 역할 분리 |
| 순차 파이프라인 (Stage 1→5) | ✅ 올바름 | 디버깅·footprint 추적 용이 |
| EvidencePack + integrity hash | ✅ 올바름 | 검증 가능한 출력. 기업 환경 감사 추적에 핵심 |
| Intent 기반 검색 라우팅 (10개 intent) | ✅ 올바름 | 작업 유형마다 다른 코드를 찾아야 함 |
| Agent 역할 분리 (Planning/Impl/Eval) | ✅ 올바름 | Context window 효율 + 특화 프롬프트 가능 |
| 전용 워크스페이스 (폴더 + 브랜치) | ✅ 올바름 | 추적성 + 복원력. 실패해도 중간 산출물 보존 |
| 분할 커밋 | ✅ 올바름 | 인간 리뷰어 고려. 대부분의 AI 코딩 도구가 놓치는 부분 |
| Chainbench 통합 테스트 | ✅ 올바름 | Execution-based verification은 가장 신뢰성 높은 검증 |
| PR 기반 평가 | ✅ 올바름 | SWE-bench와 동일 발상. 코딩 에이전트 평가의 사실상 표준 |

### 2.2 개선이 필요한 부분

#### 2.2.1 Static RAG → Agentic RAG 전환 필요

**현재 CKS 설계 (Static RAG)**:

```
Query → [Intent 분류] → [CKV 의미 검색] → [CKG 키워드 검색]
      → [Graph 확장] → [Budget 할당] → [Sanitize] → EvidencePack (1회)
```

이 방식은 1회 검색(one-shot retrieval)이다. 쿼리가 들어오면 한 번의 파이프라인을
거쳐 결과를 반환한다.

**연구에서 밝혀진 한계**:
- 복잡한 작업은 한 번의 검색으로 필요한 모든 코드를 찾을 수 없음
- 첫 검색 결과를 보고 "이 함수가 호출하는 다른 함수도 봐야겠다"는 판단은 LLM이 해야 함
- Static RAG는 이런 반복 탐색이 불가능

**더 발전된 패턴 (Agentic RAG)**:

```
Planning Agent가 CKS/CKG tool을 필요할 때마다 반복 호출:

Agent: "Jira 내용 분석... genesis block 관련 코드를 찾아보자"
→ CKS.get_for_task("genesis block creation logic")
→ 결과 확인: "core/genesis.go 찾았는데, initStaking도 봐야겠다"
→ CKG.find_callees("core.InitGenesis")
→ 결과 확인: "staking 초기화도 관련. 최근 수정된 적 있나?"
→ CKG.evidence_for_intent("staking initialization bug in genesis")
→ 충분한 정보 수집 → 계획 수립
```

핵심 차이: CKS를 "한 번 호출하면 모든 답을 주는 시스템"이 아니라
"Agent가 반복 호출하는 tool 모음"으로 바라보는 것이다.

좋은 소식: 현재 설계가 이 전환을 막지 않는다. CKS MCP의 `get_for_task`뿐 아니라
CKG의 8개 개별 tool(`find_symbol`, `find_callers`, `impact_of_change` 등)을
Agent가 직접 호출할 수 있다. 코드 변경 없이 Agent 레이어에서 전환 가능하다.

**관련 연구**:
- Self-RAG (Asai et al., 2023.10): 검색 결과의 관련성을 LLM이 판단하여 추가 검색 결정
- FLARE (Jiang et al., 2023): LLM이 생성 중 확신이 낮으면 자동으로 검색 트리거
- Adaptive RAG (Jeong et al., 2024): 쿼리 복잡도에 따라 검색 전략 자동 선택

#### 2.2.2 Contextual Retrieval 적용 (Anthropic, 2024.09)

코드 청크를 임베딩하기 전에, 해당 청크의 컨텍스트 정보를 텍스트로 추가하는 기법이다.

```
[현재 CKV 청크]
"func (v *Vault) Deposit(amount *big.Int) error {"

[Contextual Retrieval 적용 후]
"Package: vault | File: contracts/Vault.sol |
 Called by: Router.execute() |
 Recently modified in PR #67 (fix: deposit overflow check).
 func (v *Vault) Deposit(amount *big.Int) error {"
```

Anthropic의 측정 결과 (2024.09):
- Contextual Retrieval만 적용: retrieval failure 35% 감소
- Contextual Retrieval + BM25 hybrid: retrieval failure 49% 감소
- Contextual Retrieval + BM25 + reranker: retrieval failure 67% 감소

현재 CKV는 `CKV_DISABLE_CONTEXTUAL_PREFIX` 환경변수로 토글되는 "symbol: X.Y"
prefix만 구현되어 있다. CKG의 호출 관계, PR 히스토리, 패키지 설명 등을 빌드 시점에
청크에 주입하면 검색 품질이 크게 향상될 수 있다.

CKV와 CKG가 분리되어 있으므로, CKV 빌드 파이프라인에서 CKG 데이터를 참조하는
단계를 추가하는 형태로 구현 가능하다.

#### 2.2.3 Cross-encoder Reranking

초기 retrieval(vector + BM25) 결과 top-30을 더 정밀한 모델로 재정렬하는 기법이다.

- Bi-encoder (vector search): 쿼리와 문서를 독립적으로 임베딩 → 빠르지만 부정확
- Cross-encoder (reranker): 쿼리+문서 쌍을 함께 입력 → 느리지만 정확

Bi-encoder로 후보를 빠르게 좁히고, Cross-encoder로 top-K를 정밀 선별하는
2-stage 패턴이 표준이다.

대표 모델: BGE Reranker M3 (multilingual), Cohere Rerank, ms-marco-MiniLM

현재 CKS에서 Tier 2로 계획되어 있으나 미구현 상태이다.

#### 2.2.4 실패 복귀 시 미푸시 코드 처리

Phase 5에서 테스트 실패 → Phase 2 복귀 시, 아직 push되지 않은 코드는
CKS 인덱스에 반영되어 있지 않다. 세 가지 해결 옵션이 있다:

| 옵션 | 방법 | 장점 | 단점 |
|------|------|------|------|
| **A: CKS 우회** | Agent가 git diff를 LLM 컨텍스트에 직접 포함. CKS는 "변경 전 코드" 검색에만 사용 | CKS 수정 불필요 | diff가 클 경우 토큰 압박 |
| **B: diff overlay** | CKS에 "이 파일들은 이 내용으로 대체" 옵션 추가 | 일관된 검색 파이프라인 | CKS 구현 수정 필요 |
| **C: 즉석 reindex** | ckv reindex + ckg rebuild를 로컬 변경 포함하여 실행 | 완전한 검색 정합성 | 인덱싱 시간 소요 |

권장: 초기(Phase α~β)에는 **옵션 A**로 시작, 인덱싱 속도 확보 후 **옵션 C** 전환.

#### 2.2.5 기타 보완점

| 보완점 | 이유 | 제안 |
|--------|------|------|
| 실패 분류 | "왜 실패했는가"에 따라 복귀 경로 달라야 함 | 에러 taxonomy 정의 (build fail / test fail / logic error / performance) |
| 점진적 검증 | 전체 구현 후 테스트하면 디버깅 비용 큼 | 각 커밋 단위 micro-test (TDD 패턴) |
| Agent 간 context 전달 | Planning→Implementation 전환 시 context 손실 위험 | 구조화된 handoff 문서 (이미 설계에 포함됨) |

---

## 3. Code RAG 분야의 주요 연구 패턴

유저 시나리오에 적용 가능한 Code RAG 기법들을 정리한다.

### 3.1 검색 패턴 비교

| 패턴 | 핵심 아이디어 | 출처 | 적용 위치 | 현재 상태 |
|------|------------|------|----------|----------|
| **Hybrid Search** | vector + BM25를 RRF로 결합 | Cormack et al., 2009 | CKS fusion | ⚠️ 부분 적용 |
| **Domain Glossary** | 사람이 관리하는 용어 매핑표 | 실무 패턴 | CKS vocab | ❌ 미구현 |
| **Query Expansion** | glossary 기반 alias 추가 | 실무 패턴 | CKS Stage 1 | ❌ 미구현 |
| **HyDE** | LLM이 가상 답변 생성 → 그걸로 검색 | Gao et al., 2022.12 | Agent layer | ❌ Tier 3 |
| **MultiQuery** | LLM이 1 query → N개 다양 표현 | 실무 패턴 | Agent layer | ❌ Tier 3 |
| **Cross-encoder Reranker** | top-30 → cross-encoder 재정렬 | Nogueira & Cho, 2019 | CKS fusion | ❌ Tier 2 |
| **Contextual Retrieval** | 청크에 문서 컨텍스트 주입 | Anthropic, 2024.09 | CKV build | ⚠️ 최소 적용 |
| **Entity Linking** | surface form → canonical qname | 실무 패턴 | CKS vocab | ❌ 미구현 |
| **GraphRAG** | 그래프 community 요약 → 검색 | Microsoft, 2024.04 | CKG | ❌ Tier 3 |
| **Symbol-aware Chunking** | AST/symbol 경계로 chunk 분할 | 실무 패턴 | CKV chunk | ✅ 구현됨 |
| **Agentic RAG** | Agent가 검색 tool을 반복 호출 | ReAct, Self-RAG | Agent layer | ❌ 미적용 |
| **Embedder Fine-tune** | 도메인 데이터로 임베딩 모델 재학습 | 실무 패턴 | CKV embedder | ❌ Tier 3 |

### 3.2 검색 품질 향상 순서 (예상 영향력 기준)

| 순위 | 기법 | 예상 recall 향상 | 근거 (출처) |
|------|------|----------------|------------|
| 1 | **Real Embedder** | +40~60% | FakeEmbedder → Real은 기반 자체가 바뀜 |
| 2 | **Hybrid Search (RRF)** | +10~25% | Anthropic (2024.09): BM25 추가로 failure 49% 감소 |
| 3 | **Contextual Enrichment** | +5~15% | Anthropic (2024.09): contextual chunks로 failure 67% 감소 |
| 4 | **Cross-encoder Reranker** | +5~10% | 초기 retrieval 잡음 제거, precision 향상 |
| 5 | **Query Expansion / Glossary** | +5~15% (한국어) | 언어 장벽 해소. 영어 쿼리에는 효과 적음 |
| 6 | **Agentic RAG** | 정량화 어려움 | 복잡한 작업에서 coverage 향상 |

---

## 4. CKV 내부 시스템 — 베스트 프랙티스 플로우

### 4.1 인덱싱 파이프라인 (Build Time)

```
Source Code
  │
  ├─ [1] Discovery (파일 탐색)
  │     - .gitignore + 시크릿 패턴 필터링
  │     - 바이너리/대용량 파일 제외
  │     ✅ CKV 구현됨
  │
  ├─ [2] Parsing (AST 파싱)
  │     - tree-sitter가 사실상 업계 표준 (40+ 언어, incremental parsing)
  │     - Go는 types.Info로 타입 해석까지 (tree-sitter만으로는 부족)
  │     - Symbol 추출: function, type, method, interface, ...
  │     ✅ CKV 구현됨 (Go/TS/Sol/MD)
  │
  ├─ [3] Chunking (의미 단위 분할)
  │     ┌─────────────────────────────────────────────────────┐
  │     │ 코드 특화 chunking 전략 (베스트 프랙티스)              │
  │     ├─────────────────────────────────────────────────────┤
  │     │ - Symbol-level: 함수/타입/메서드 단위 (가장 효과적)     │
  │     │ - File header: import/package 컨텍스트 보존            │
  │     │ - 긴 함수: sliding window split (signature 유지)       │
  │     │ - 테스트 파일: 별도 분리 (검색 시 선택적 포함)           │
  │     │ - 피해야 할 것: 고정 토큰 수 기반 분할 (의미 파괴)       │
  │     └─────────────────────────────────────────────────────┘
  │     ✅ CKV 구현됨
  │
  ├─ [4] Contextual Enrichment ← ★ 가장 큰 개선 기회
  │     ┌─────────────────────────────────────────────────────┐
  │     │ Anthropic Contextual Retrieval (2024.09) 적용 방법    │
  │     ├─────────────────────────────────────────────────────┤
  │     │ 각 chunk에 다음 컨텍스트를 텍스트로 추가:              │
  │     │                                                     │
  │     │ 1. 파일 경로와 패키지명                                │
  │     │ 2. 이 심볼을 호출하는 주요 caller (CKG 데이터 참조)     │
  │     │ 3. 최근 수정된 PR 정보 (CKG PR breadcrumb 참조)        │
  │     │ 4. 이 심볼이 속한 모듈의 역할 설명 (doc comment)        │
  │     │                                                     │
  │     │ 예시:                                                │
  │     │ [원본] func (v *Vault) Deposit(amount *big.Int) {    │
  │     │ [보강] Package: vault | File: contracts/Vault.sol    │
  │     │        Called by: Router.execute()                   │
  │     │        Modified in PR #67 (deposit overflow fix)     │
  │     │        func (v *Vault) Deposit(amount *big.Int) {    │
  │     └─────────────────────────────────────────────────────┘
  │     ⚠️ CKV: "symbol: X.Y" prefix만 구현. 호출 관계·PR 정보 미포함
  │
  ├─ [5] Embedding (벡터 변환)
  │     ┌─────────────────────────────────────────────────────┐
  │     │ 임베딩 모델 선택 기준 (코드 검색용)                     │
  │     ├─────────────────────────────────────────────────────┤
  │     │ 1. 코드 특화 학습 여부 (범용 < 코드 특화)              │
  │     │ 2. 다국어 지원 (한국어 쿼리 처리)                      │
  │     │ 3. 차원 수 (512~2048 권장. 너무 작으면 정보 손실)       │
  │     │ 4. 컨텍스트 길이 (512 < 8K < 32K)                    │
  │     │ 5. 처리량 (인덱싱 시간에 직결)                         │
  │     │ 6. 로컬 실행 가능 여부 (보안/오프라인)                  │
  │     │                                                     │
  │     │ 현재 후보: qwen3-embedding:0.6b (Ollama)             │
  │     │ - MTEB Code: 75.41, 1024d, 32K context              │
  │     │ - Ollama로 로컬 실행, Apple Silicon 가속              │
  │     └─────────────────────────────────────────────────────┘
  │     ❌ CKV: FakeEmbedder만 사용 중 (P0 블로커)
  │
  └─ [6] Storage (벡터 저장)
        - sqlite-vec: 임베디드, 단일 파일, cosine distance search
        - Manifest: 인덱스 ID 관리 (model + commit hash)
        - 실무 규모 천장: ~1M chunks (단일 writer 설계)
        ✅ CKV 구현됨
```

### 4.2 검색 파이프라인 (Query Time)

```
User Query (자연어, 한국어 가능)
  │
  ├─ [1] Query Understanding
  │     ┌─────────────────────────────────────────────────────┐
  │     │ 3단계 쿼리 이해 (베스트 프랙티스)                      │
  │     ├─────────────────────────────────────────────────────┤
  │     │ A. Intent Classification                            │
  │     │    - 어떤 종류의 작업? (bug_fix, feature_add, ...)   │
  │     │    - 임베딩 cosine similarity 기반 분류              │
  │     │    ✅ CKS: 10 intents, 양방향 앵커 (한/영)          │
  │     │                                                     │
  │     │ B. Vocabulary Resolution                            │
  │     │    - 한국어/모호 → 코드 정확 키워드                    │
  │     │    - glossary YAML 기반 rule 매핑                    │
  │     │    ❌ CKS: 미구현                                    │
  │     │                                                     │
  │     │ C. Query Expansion                                  │
  │     │    - 동의어/관련어 추가                               │
  │     │    - "genesis" → "genesis, GenesisBlock, InitGenesis"│
  │     │    ❌ CKS: 미구현                                    │
  │     └─────────────────────────────────────────────────────┘
  │
  ├─ [2] Initial Retrieval (후보 수집)
  │     ┌─────────────────────────────────────────────────────┐
  │     │ Hybrid Search (베스트 프랙티스)                       │
  │     ├─────────────────────────────────────────────────────┤
  │     │ 두 가지 검색을 병렬 실행:                              │
  │     │                                                     │
  │     │ A. Vector Search (의미적 유사도)                      │
  │     │    - 쿼리 임베딩 → cosine distance → top-K            │
  │     │    - 장점: 동의어, 유사 개념 매칭 가능                  │
  │     │    - 단점: 정확한 함수명 매칭에 약함                    │
  │     │    ✅ CKV: sqlite-vec cosine search                 │
  │     │                                                     │
  │     │ B. BM25 Search (키워드 정확 매칭)                     │
  │     │    - 토큰 분리 → IDF 가중치 → 점수 계산                │
  │     │    - 장점: 정확한 이름/키워드 매칭                      │
  │     │    - 단점: 의미적 유사도 없음                          │
  │     │    ✅ CKG: FTS5 + Okapi BM25                        │
  │     └─────────────────────────────────────────────────────┘
  │
  ├─ [3] Fusion (결과 합침)
  │     ┌─────────────────────────────────────────────────────┐
  │     │ RRF (Reciprocal Rank Fusion)                        │
  │     ├─────────────────────────────────────────────────────┤
  │     │ 공식: score = Σ 1/(k + rank_i)  (k=60이 표준)       │
  │     │                                                     │
  │     │ 예시:                                                │
  │     │ Vector search: [A:1위, B:3위, C:5위]                 │
  │     │ BM25 search:   [B:1위, A:4위, D:2위]                 │
  │     │                                                     │
  │     │ RRF scores:                                          │
  │     │ A = 1/(60+1) + 1/(60+4) = 0.0164 + 0.0156 = 0.0320│
  │     │ B = 1/(60+3) + 1/(60+1) = 0.0159 + 0.0164 = 0.0323│
  │     │ → B가 1위 (두 검색에서 모두 상위였으므로)               │
  │     │                                                     │
  │     │ RRF의 장점: 점수 정규화 불필요 (rank만 사용)            │
  │     │ Cormack et al. (2009) 이후 사실상 표준                │
  │     └─────────────────────────────────────────────────────┘
  │     ⚠️ CKV: optional RRF 구현. CKS: BM25+Symbol 합산 (정식 RRF 아님)
  │
  ├─ [4] Reranking (정밀 재정렬) ← ★ 품질 크게 향상
  │     ┌─────────────────────────────────────────────────────┐
  │     │ 2-stage Retrieval (베스트 프랙티스)                   │
  │     ├─────────────────────────────────────────────────────┤
  │     │ Stage 1: Bi-encoder (빠른 후보 수집)                  │
  │     │ - 쿼리와 문서를 독립적으로 임베딩                       │
  │     │ - 빠르지만 쿼리-문서 상호작용 없음                      │
  │     │ - top-30 후보 수집                                   │
  │     │                                                     │
  │     │ Stage 2: Cross-encoder (정밀 재정렬)                  │
  │     │ - 쿼리+문서 쌍을 함께 모델에 입력                       │
  │     │ - 느리지만 쿼리-문서 관계를 직접 평가                    │
  │     │ - top-30 → top-5 정밀 선별                           │
  │     │                                                     │
  │     │ 대표 모델: BGE Reranker M3 (multilingual)             │
  │     └─────────────────────────────────────────────────────┘
  │     ❌ CKS: 미구현 (Tier 2 계획)
  │
  ├─ [5] Graph Expansion (관련 코드 확장)
  │     - 검색된 코드의 caller/callee/implementor 탐색
  │     - Intent에 따라 다른 관계 유형 사용:
  │       BugFix → calls + called_by
  │       ArchExplain → imports + implements + embeds
  │       ConcurrencySafety → acquires_lock + spawns
  │     ✅ CKS Stage 3 구현됨
  │
  └─ [6] Budget Packing (토큰 예산 내 선별)
        - 점수 높은 순으로 greedy fit
        - 토큰 예산 초과 시 skip (truncation 아님)
        ✅ CKS Stage 4 구현됨
```

---

## 5. CKG 내부 시스템 — 베스트 프랙티스 플로우

### 5.1 그래프 구축 파이프라인 (Build Time)

```
Source Code + Git History
  │
  ├─ [1] Multi-language AST Parsing
  │     - tree-sitter가 사실상 표준 (incremental, 40+ 언어)
  │     - Go는 types.Info 필요 (interface 만족 관계 = 구조적 타이핑)
  │     ✅ CKG: Go + TS + Solidity + Proto 파서
  │
  ├─ [2] Symbol & Relation Extraction
  │     - Node: 함수, 타입, 변수, 패키지, 파일
  │     - Edge: calls, imports, implements, extends
  │     - 동시성 특화: acquires_lock, spawns, sends_to, recvs_from
  │     ✅ CKG: 35 node types, 40 edge types
  │
  ├─ [3] Cross-file Reference Resolution
  │     - import 경로 → 실제 파일 → symbol 연결
  │     - Go의 interface 만족 관계 추론
  │     - 2-pass: 수집(collect) → 해결(resolve)
  │     ✅ CKG: 구현됨
  │
  ├─ [4] Temporal Layer (Git History) ← ★ 유저 시나리오 핵심
  │     ┌─────────────────────────────────────────────────────┐
  │     │ 코드 히스토리 그래프 (Code Temporal Graph)             │
  │     ├─────────────────────────────────────────────────────┤
  │     │ Commit 노드: git commit 메타데이터                    │
  │     │ Hunk 노드: diff의 각 변경 블록                        │
  │     │ PR 메타데이터: 번호, 제목, base/head SHA              │
  │     │                                                     │
  │     │ 관계:                                                │
  │     │ - Function ──changed_in──▶ Commit                   │
  │     │ - Commit ──has_hunk──▶ Hunk                         │
  │     │ - Hunk ──modifies──▶ Function                       │
  │     │ - Hunk ──adjacent──▶ Hunk (같은 commit 내)           │
  │     │                                                     │
  │     │ 활용: "이 함수가 왜 이렇게 바뀌었는가?"                  │
  │     │ → Function의 changed_in Commits + PR 정보 추적        │
  │     └─────────────────────────────────────────────────────┘
  │     ✅ CKG: schema 1.12, PR breadcrumb (ckg-NEW-2/3/4)
  │
  ├─ [5] Scoring & Centrality
  │     - PageRank: "중요한 심볼" 식별
  │       (많이 호출되는 함수 = 높은 점수 = 검색에서 우선)
  │     - Betweenness centrality: "브릿지 심볼" 식별
  │       (모듈 간 연결 지점 = 변경 시 영향 범위 큼)
  │     ✅ CKG: 구현됨
  │
  └─ [6] Full-Text Index (BM25)
        ┌─────────────────────────────────────────────────────┐
        │ Code-aware BM25 (베스트 프랙티스)                     │
        ├─────────────────────────────────────────────────────┤
        │ 인덱싱 필드:                                         │
        │ - name: 짧은 심볼명 (Deposit)                        │
        │ - qualified_name: 전체 경로 (vault.Vault.Deposit)     │
        │ - signature: 함수 시그니처                             │
        │ - doc_comment: 문서 주석                              │
        │                                                     │
        │ 토크나이저:                                           │
        │ - camelCase 분리: parseFile → [parse, file]          │
        │ - snake_case 분리: init_genesis → [init, genesis]    │
        │ - dot/slash 분리: pkg.Type → [pkg, type]             │
        │ - 소문자화, 2자 이상 필터                               │
        │                                                     │
        │ 스코어링: Okapi BM25                                  │
        │ - K1=1.5 (term-frequency 포화)                       │
        │ - B=0.75 (문서 길이 정규화)                            │
        │ - IDF = log(1 + (N-df+0.5)/(df+0.5))                │
        └─────────────────────────────────────────────────────┘
        ✅ CKG: FTS5 + 커스텀 tokenizer
```

### 5.2 검색/분석 파이프라인 (Query Time)

```
Query (키워드 또는 intent)
  │
  ├─ [1] BM25 Text Search
  │     - FTS5 OR mode: 넓은 후보 수집
  │     - FTS5 AND mode: 정밀 검색 (모든 토큰 포함 필수)
  │     - CJK(한국어 등) fallback: substring LIKE 검색
  │     ✅ CKG: search_text tool
  │
  ├─ [2] Symbol Lookup
  │     - qname exact match: 정확한 심볼 찾기
  │     - suffix match: 부분 이름으로 검색
  │     - Kind 필터: function, type, interface 등
  │     ✅ CKG: find_symbol tool
  │
  ├─ [3] Graph Traversal
  │     ┌─────────────────────────────────────────────────────┐
  │     │ Intent-aware Graph Traversal                         │
  │     ├─────────────────────────────────────────────────────┤
  │     │ BFS(Breadth-First Search) 기반 이웃 탐색              │
  │     │                                                     │
  │     │ Intent별 탐색 관계:                                   │
  │     │ - BugFix: calls + called_by (영향 범위 추적)          │
  │     │ - FeatureAdd: imports + uses_type (의존 모듈)         │
  │     │ - ArchExplain: implements + embeds (구조)             │
  │     │ - ConcurrencySafety: acquires_lock + spawns (경쟁)   │
  │     │ - Security: reads_field + writes_field (데이터 흐름)  │
  │     │                                                     │
  │     │ depth=2가 기본 (latency vs coverage 균형)             │
  │     │ depth=3+는 비용 급증 (exponential edge explosion)     │
  │     └─────────────────────────────────────────────────────┘
  │     ✅ CKG: NeighborhoodByQname
  │
  ├─ [4] Impact Analysis (역방향 의존성 분석)
  │     ┌─────────────────────────────────────────────────────┐
  │     │ "이 함수를 바꾸면 뭐가 깨지나?" — 6개 카테고리          │
  │     ├─────────────────────────────────────────────────────┤
  │     │ 1. Callers: 이 함수를 직접 호출하는 코드               │
  │     │ 2. Interface impact: 인터페이스 구현체 영향             │
  │     │ 3. Type users: 이 타입을 사용하는 코드                  │
  │     │ 4. Distributed: RPC/HTTP/gRPC 호출 영향               │
  │     │ 5. Concurrent: lock/channel 공유하는 코드              │
  │     │ 6. Other: 기타 관계                                   │
  │     └─────────────────────────────────────────────────────┘
  │     ✅ CKG: impact_of_change tool
  │
  └─ [5] Evidence Assembly (증거 조립)
        - 관련 Hunk 수집 (git diff 단위)
        - BM25로 intent와 가장 관련된 hunk 순위 매기기
        - 패치 코드 포함: "이전에 어떻게 수정했는가"
        ✅ CKG: evidence_for_intent tool (H3)
```

### 5.3 CKG에서 가장 영향력 큰 개선 순서

| 순위 | 개선 | 근거 |
|------|------|------|
| 1 | Real BM25 Score 노출 (F-1) | CKS가 합성 점수를 쓰는 병목 제거 |
| 2 | CamelCase FTS5 분리 | "parseFile" → "parse"+"file" 매칭. 현재 unicode61 한계 |
| 3 | Community Summary (GraphRAG 패턴) | 모듈 수준 "이 패키지는 뭘 하는 곳" 요약 |
| 4 | Cross-snapshot search (ckg-NEW-7) | PR merge 전 시점 코드 검색 (평가에 필수) |

---

## 6. 업계 참조 시스템과의 비교

### 6.1 유사 시스템 대조표

| 시스템 | 접근법 | 유저 시나리오와의 차이 | 출처/시점 |
|--------|--------|---------------------|----------|
| **SWE-Agent** | ReAct + file editing tools | Agent 구조 유사. RAG 없이 파일 직접 탐색 | Princeton, 2024.02 |
| **Aider** | repomap (AST graph) + sparse retrieval | CKG와 유사. 벡터 검색 없음 | Paul Gauthier, 2024 |
| **Cursor** | codebase-wide embeddings + context window | CKV와 유사. Graph 없음 | Cursor Inc, 2024 |
| **Sourcegraph Cody** | LSIF graph + dense embeddings + symbol search | CKG+CKV 합친 것과 가장 유사 | Sourcegraph, 2023-2024 |
| **Amazon Q Developer** | BM25 + vector + IDE context | Hybrid search. Graph traversal 없음 | AWS, 2024 |
| **Claude Code** | Tool use + file system + git integration | Agent 패턴 참조. MCP tool 기반 | Anthropic, 2025.01 |
| **Devin** | 자율 Agent + 브라우저 + 터미널 + 에디터 | 풀스택 자율 에이전트 | Cognition, 2024.03 |

### 6.2 유저 시나리오의 독특한 강점

| 강점 | 설명 | 타 시스템 대비 |
|------|------|-------------|
| CKG 동시성 분석 | lock/channel/mutex 관계 추적 | 대부분의 도구에 없음 |
| PR breadcrumb | 코드의 "변경 이유"까지 검색 | 대부분 현재 코드만 봄 |
| EvidencePack integrity | 디버깅·감사 추적 | 기업 환경에 중요 |
| Intent-based routing | 작업 유형별 검색 최적화 | 범용 도구에 없음 |
| 3-repo 분리 | 독립 발전·교체 가능 | 대부분 monolithic |

---

## 7. 지식 기준 날짜

| 기술/논문 | 발표 시점 | 설명 정확도 |
|----------|----------|-----------|
| RAG 기본 패턴 (Lewis et al.) | 2020 | ✅ 안정적 |
| Okapi BM25 | 1994 (Robertson et al.) | ✅ 안정적 |
| RRF (Cormack et al.) | 2009 | ✅ 안정적 |
| Cross-encoder reranking | 2019 (Nogueira & Cho) | ✅ 안정적 |
| HyDE (Gao et al.) | 2022.12 | ✅ 안정적 |
| ReAct (Yao et al.) | 2022.10 | ✅ 안정적 |
| Reflexion (Shinn et al.) | 2023.03 | ✅ 안정적 |
| Self-RAG (Asai et al.) | 2023.10 | ✅ 안정적 |
| SWE-bench (Jimenez et al.) | 2023.10 | ✅ 안정적 |
| SWE-Agent (Yang et al.) | 2024.02 | ✅ 안정적 |
| Microsoft GraphRAG | 2024.04 | ✅ 안정적 |
| **Anthropic Contextual Retrieval** | **2024.09** | ✅ 안정적 |
| Qwen3-Embedding 벤치마크 | 2025.01 이후 | ⚠️ 웹 검색 기반 |
| Ollama MLX backend | 2025.03 | ⚠️ 웹 검색 기반 |
| Agentic RAG 최신 동향 | 2024.Q4~2025.Q1 | ⚠️ 빠르게 변화 중 |

### 7.1 Gap이 클 수 있는 영역 (최신 확인 권장)

- 2025년 2월 이후 코딩 에이전트 벤치마크 결과 (SWE-bench Lite 통과율)
- Qwen3-Embedding 상세 벤치마크 (실측 미확인)
- 2025년 신규 코드 임베딩 모델 (CodeSage, StarCoder-Embed 후속)
- Claude Code Agent SDK 최신 패턴
- Agentic RAG 프레임워크 진화 (LangGraph, CrewAI 등)

---

## 8. 요약

### 8.1 전체 평가

| 영역 | 평가 | 핵심 근거 |
|------|------|----------|
| 아키텍처 방향 | ✅ 올바름 | 3-repo 분리, LLM 호출 금지, 역할 분리 |
| Agent 설계 | ✅ 올바름 | Plan-and-Execute 패턴, SWE-Agent/Devin과 일치 |
| 검색 기반 (Code RAG) | ⚠️ 개선 필요 | Static RAG → Agentic RAG 전환, Real Embedder, Contextual Retrieval |
| 평가 전략 | ✅ 올바름 | PR 기반 회귀 테스트 = SWE-bench 표준과 동일 |
| 실행 순서 | ⚠️ 수정 필요 | 기반(embedder) 먼저, 상위 레이어(glossary, reranker)는 측정 후 |

### 8.2 즉시 적용할 개선 (Phase α~β)

1. Real Embedder (Ollama + qwen3-embedding) — 모든 것의 기반
2. go-stablenet 인덱스 구축 — 실제 대상으로 측정 가능
3. Hybrid Search RRF 정식 적용 — 측정된 +10~25% recall 향상

### 8.3 측정 후 적용할 개선 (Phase γ~)

4. Contextual Retrieval — CKV 빌드 시 CKG 데이터로 청크 보강
5. Cross-encoder Reranker — precision 향상
6. Glossary + Query Expansion — 한국어 쿼리 지원
7. Agentic RAG — Agent가 CKS tool을 반복 호출하는 패턴 전환
