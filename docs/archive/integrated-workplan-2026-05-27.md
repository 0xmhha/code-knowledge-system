# 통합 작업 계획 — 4개 프로젝트 교차 점검 (2026-05-27)

> **ARCHIVED 2026-07-19.** Point-in-time snapshot; live status is [`remaining-work.md`](../remaining-work.md), durable architecture map is [`system-context-map.md`](../system-context-map.md). Kept for provenance.

> **목적**: coding-agent(플러그인) + cks(오케스트레이터) + ckv(벡터) + ckg(그래프)
> 4개 프로젝트의 설계 문서와 실제 코드를 교차 점검하여,
> 각 모듈의 역할·동작·현재 상태를 확인하고 모듈별 작업 리스트를 도출한다.
>
> **관련 문서**:
> - `coding-agent/docs/superpowers/specs/2026-05-27-coding-agent-plugin-design.md` (HLD)
> - `coding-agent/docs/superpowers/specs/phase1~7-*.md` (Phase별 상세 설계)
> - `cks/docs/system-review-2026-05-27.md` (코드 상태 점검)
> - `cks/docs/technical-analysis-2026-05-27.md` (기술 분석)

---

## 1. 프로젝트 구조와 역할

```
coding-agent (Claude Code Plugin — 상위 레이어)
  │
  │  Commands: /work, /review, /status
  │  Agents: orchestrator, planner, implementer, evaluator
  │  Skills: template-parse, stablenet-context, state-machine
  │  Hooks: on-agent-complete, on-subcommit
  │
  │  상태 머신: TICKET_INTAKE → ANALYSIS → PLANNING → IMPLEMENTATION → EVALUATION → COMPLETION
  │
  ├──── Jira Gateway MCP (프록시 + Sensitive Filter)
  │         └── Atlassian MCP (기존)
  │
  ├──── CKS MCP (Code Knowledge Search — 오케스트레이터)
  │         ├── CKV Engine (Vector/Semantic Search)
  │         └── CKG Engine (Graph/BM25 Search)
  │
  ├──── Claude Models (Opus 4.7 / Sonnet 4.6)
  │
  └──── ChainBench MCP (로컬 네트워크 테스트)
```

### 1.1 각 프로젝트 역할

| 프로젝트 | 경로 | 역할 | 호출 관계 |
|----------|------|------|----------|
| **coding-agent** | `tools/coding-agent` | Plugin 진입점. 상태 머신 + Agent 파이프라인 | Claude Code에서 설치, CKS/Jira/ChainBench MCP 호출 |
| **cks** | `tools/code-knowledge-system` | 코드 검색 오케스트레이터. CKV+CKG 통합 | coding-agent의 Planner Agent가 MCP로 호출 |
| **ckv** | `tools/code-knowledge-vector` | 의미(semantic) 검색 엔진 | cks가 subprocess MCP로 호출 |
| **ckg** | `tools/code-knowledge-graph` | 구조(graph) + 키워드(BM25) 검색 엔진 | cks가 Go library import로 호출 |

---

## 2. coding-agent 설계 vs 하위 모듈 실제 구현 교차 점검

### 2.1 coding-agent Phase별 의존성과 하위 모듈 상태

| Phase | coding-agent 설계 | 의존하는 하위 모듈 | 하위 모듈 현재 상태 | 격차 |
|-------|-------------------|-------------------|-------------------|------|
| **Phase 1** | Plugin skeleton + State machine | 없음 (자체 구현) | N/A | coding-agent 자체 구현 필요 |
| **Phase 2** | Jira GW MCP + Sensitive Filter | Atlassian MCP (기존) | ✅ Atlassian plugin 설치됨 | Jira GW proxy MCP 신규 구현 필요 |
| **Phase 3** | CKS MCP - CKV | CKV embedder + search | ⚠️ 코드 완성, Real embedder 없음 | **P0: Ollama embedder 연동** |
| **Phase 4** | CKS MCP - CKG | CKG graph + BM25 | ✅ 8 MCP tools, 완전 동작 | go-stablenet 인덱스 최신화 확인 |
| **Phase 5** | Agent pipeline | CKS MCP (Phase 3+4) | ⚠️ CKS 2 tools, 동작하지만 실효성 부족 | Real embedder 해결 후 검증 가능 |
| **Phase 6** | Evaluator + ChainBench | ChainBench MCP | ❌ ChainBench MCP 미존재 | 별도 설계·구현 필요 |
| **Phase 7** | PR + Review cycle | GitHub (gh CLI) + Jira MCP | ✅ gh CLI 사용 가능 | coding-agent 자체 구현 필요 |

### 2.2 coding-agent 설계에서 이미 해결된 사항

이전 `technical-analysis` 문서에서 "미존재" 또는 "개선 필요"로 표기했으나,
coding-agent 설계에 이미 포함되어 있는 항목:

| 항목 | technical-analysis 판단 | coding-agent 설계 위치 | 상태 |
|------|----------------------|----------------------|------|
| Plugin 구조 | "❌ 미존재" | HLD §3 + Phase 1 | ✅ 상세 설계 완료 |
| Agent 아키텍처 | "❌ 미존재" | HLD §4 + Phase 5 | ✅ 상세 설계 완료 |
| 상태 머신 | 언급 안됨 | HLD §8 (state.json 전체 스키마) | ✅ 상세 설계 완료 |
| Sensitive Filter | "❌ 미존재" | HLD §6 + Phase 2 | ✅ 상세 설계 완료 |
| 미푸시 코드 처리 | "개선 필요 (옵션 A/B/C)" | HLD §4.5: git diff 직접 읽기 | ✅ 옵션 A로 결정됨 |
| 실패 분류 | "에러 taxonomy 필요" | HLD §8.1 failure_log (구조화된 에러 기록) | ✅ 설계됨 |
| 워크스페이스 관리 | "Skill 미존재" | HLD §8.3 아티팩트 폴더 구조 | ✅ 설계됨 |
| Hook | "❌ 미존재" | HLD §3.3 hooks.json | ✅ 설계됨 |
| Jira 템플릿 | 언급 안됨 | HLD §7 (Feature/BugFix/Review/Release) | ✅ 설계됨 |
| 모델 라우팅 | 언급 안됨 | HLD §1.4 (Planner: Opus, Impl/Eval: Sonnet) | ✅ 설계됨 |

### 2.3 coding-agent 설계에서 미결정 사항 (HLD §10)

| 항목 | 후보 | 현재 진행 상황 | 권장 결정 |
|------|------|--------------|----------|
| CKV Vector Store | SQLite+vector, ChromaDB, Qdrant | ✅ **sqlite-vec로 결정** (ADR-001) | 확정 |
| CKV Embedding Model | CodeBERT, StarCoder, Claude | ⚠️ **qwen3-embedding:0.6b (Ollama)** 으로 방향 전환 | 확정 필요 |
| CKG Graph Store | embedded Neo4j, SQLite, in-memory | ✅ **SQLite (pure Go)** 로 결정 | 확정 |
| Jira GW MCP 구현 언어 | TS, Python, Go | 미결정 | TS 권장 (MCP SDK 생태계) |
| CKS MCP 구현 언어 | Go | ✅ **Go로 구현됨** | 확정 |
| 커밋 크기 상한 | 파일 수 N / diff M줄 | 미결정 | Phase 5에서 결정 |
| ChainBench 타임아웃 | 분 단위 | 미결정 | Phase 6에서 결정 |

---

## 3. 모듈별 현재 상태와 작업 리스트

### 3.1 CKV (Code Knowledge Vector)

**역할**: 의미(semantic) 기반 코드 검색. Jira 티켓의 자연어 설명으로 관련 코드를 찾는다.

**coding-agent Phase 3에서 기대하는 CKV 기능**:

| 기대 기능 (Phase 3 설계) | CKV 실제 구현 | 상태 | 격차 |
|------------------------|-------------|------|------|
| Go AST 기반 Code Chunker | Symbol-level chunking (Go/TS/Sol/MD) | ✅ 더 넓은 범위 구현 | — |
| Embedding Model (코드 특화) | FakeEmbedder (mock) | ❌ **P0 블로커** | Ollama 어댑터 필요 |
| Vector Store (로컬) | sqlite-vec | ✅ 구현됨 | — |
| Reranker | BM25 optional rerank (RRF) | ✅ 구현됨 | Cross-encoder는 Tier 2 |
| ckv_search MCP tool | `cks.context.semantic_search` (4 tools) | ✅ 더 풍부 | 인터페이스 매핑 필요 |
| Sensitive Filter (내장) | Secret filtering (28 patterns) | ✅ 구현됨 | — |
| Incremental Index | `ckv reindex` (git diff 기반) | ✅ 구현됨 | — |
| Glossary/Alias | `ckv glossary` CLI | ✅ 기능 존재 | go-stablenet 사전 미작성 |

**CKV 작업 리스트**:

| ID | 작업 | 설명 | 우선순위 | 의존 | 상태 |
|----|------|------|---------|------|------|
| **V-1** | Ollama embedder 어댑터 | `internal/embed/ollama/` 신규. Embedder 인터페이스 구현 | **P0** | Ollama 설치 | 미착수 |
| **V-2** | qwen3-embedding:0.6b 연동 테스트 | 기존 eval fixture로 recall 측정 | **P0** | V-1 | 미착수 |
| **V-3** | go-stablenet 인덱스 구축 | `ckv build --src=<go-stablenet> --embedder=ollama` | **P0** | V-1 | 미착수 |
| **V-4** | go-stablenet glossary 작성 | 한국어/도메인 용어 → 코드 키워드 YAML | P1 | 도메인 지식 | 미착수 |
| **V-5** | PR fixture 12개 확장 | go-stablenet 실제 fix/feature PR 기반 | P1 | V-3 | 4개 존재 |
| **V-6** | Real embedder 기준 eval 실행 | recall@5, MRR, hallucination rate | P1 | V-2, V-3, V-5 | 미착수 |
| **V-7** | Contextual Enrichment 확장 | CKG 데이터 (caller, PR info) 를 chunk에 주입 | P2 | V-3, G-1 | prefix만 구현 |
| **V-8** | coding-agent MCP tool 인터페이스 확인 | Phase 3 `ckv_search` 스펙과 현재 MCP tool 매핑 | P1 | — | 미착수 |

### 3.2 CKG (Code Knowledge Graph)

**역할**: 구조(graph) 기반 코드 검색. 심볼의 의존성, 호출 관계, 동시성 영향, 변경 히스토리 제공.

**coding-agent Phase 4에서 기대하는 CKG 기능**:

| 기대 기능 (Phase 4 설계) | CKG 실제 구현 | 상태 | 격차 |
|------------------------|-------------|------|------|
| AST 기반 관계 추출 | 35 node types, 40 edge types | ✅ 설계보다 풍부 | — |
| Git History Analyzer | Commit/Hunk nodes + PR breadcrumb | ✅ schema 1.12 | — |
| Concurrency Analyzer | acquires_lock, spawns, sends_to 등 | ✅ 구현됨 | lock propagation 부분 미완 |
| Graph Store | SQLite (pure Go) + PostgreSQL | ✅ 구현됨 | — |
| ckg_query MCP tool | 8개 MCP tools (find_symbol, find_callers, impact_of_change 등) | ✅ 설계보다 풍부 | — |
| BFS/DFS + depth control | NeighborhoodByQname, depth=1~2 | ✅ 구현됨 | — |

**CKG 작업 리스트**:

| ID | 작업 | 설명 | 우선순위 | 의존 | 상태 |
|----|------|------|---------|------|------|
| **G-1** | go-stablenet 인덱스 최신화 확인 | `ckg build --src=<go-stablenet>`, eval/stablenet 상태 점검 | **P0** | — | eval/stablenet 존재, 최신 여부 미확인 |
| **G-2** | CKS store.Reader API 검증 | go-stablenet에서 BM25Search, FindSymbol, Neighbors 정확성 | P1 | G-1 | 미착수 |
| **G-3** | PR breadcrumb 검증 | go-stablenet PR 히스토리 파싱 정확성 | P1 | G-1 | 미착수 |
| **G-4** | Real BM25 Score 노출 (F-1) | store.Reader에 raw score 필드 추가 → CKS 합성 점수 제거 | P1 | — | 미착수 |
| **G-5** | coding-agent MCP tool 인터페이스 확인 | Phase 4 `ckg_query` 스펙과 현재 8개 MCP tool 매핑 | P1 | — | 미착수 |
| **G-6** | CamelCase FTS5 토크나이저 | unicode61 한계 해결 | P2 | — | 미착수 |

### 3.3 CKS (Code Knowledge Search — Orchestrator)

**역할**: CKV + CKG를 통합하여 코드 검색 결과를 EvidencePack으로 조립.

**coding-agent에서 기대하는 CKS 기능**:

| 기대 기능 | CKS 실제 구현 | 상태 | 격차 |
|----------|-------------|------|------|
| 의미 검색 (CKV 경유) | Stage 1: CKV semantic search + iterative refine | ✅ 구현됨 | Real embedder 의존 |
| 키워드 검색 (CKG 경유) | Stage 2: CKG BM25 + FindSymbol | ✅ 구현됨 | — |
| Graph 확장 | Stage 3: CKG Neighbors (intent-aware) | ✅ 구현됨 | — |
| Budget packing | Stage 4: greedy fit | ✅ 구현됨 | — |
| Sensitive Filter | Stage 5: 14 regex rules | ✅ 구현됨 | — |
| Intent 분류 | 10 intents, bilingual anchors | ✅ 구현됨 | Real embedder 의존 |
| MCP tools | get_for_task, health | ✅ 2 tools | 추가 tools 필요 여부 확인 |
| Evaluation | 9 scenarios, P/R/F1 | ✅ 구현됨 | go-stablenet 시나리오 추가 필요 |

**CKS 작업 리스트**:

| ID | 작업 | 설명 | 우선순위 | 의존 | 상태 |
|----|------|------|---------|------|------|
| **S-1** | Ollama embedder → intent classifier 연결 | FakeEmbedder 교체 | **P0** | V-1 | 미착수 |
| **S-2** | go-stablenet config 작성 | CKG path, CKV path, real embedder 설정 | **P0** | V-3, G-1 | 예시만 존재 |
| **S-3** | go-stablenet eval 시나리오 작성 | 실제 Jira 유사 쿼리 기반 | P1 | S-2 | 미착수 |
| **S-4** | E2E dogfood eval (go-stablenet) | recall/precision 실측 | P1 | S-2, S-3 | 미착수 |
| **S-5** | Vocabulary resolver 구현 | `internal/vocab/resolver.go` — glossary + query expansion | P1 | V-4 | 미구현 |
| **S-6** | CKS MCP tool 확장: 검색 | `semantic_search`, `search_text` 추가 | P1 | — | 미구현 |
| **S-7** | CKS MCP tool 확장: 그래프 탐색 | `find_symbol`, `find_callers`, `find_callees`, `get_subgraph` 추가 | P1 | S-10 | 미구현 |
| **S-8** | CKS MCP tool 확장: 분석 | `impact_analysis`, `change_history` 추가 | P1 | S-10 | 미구현 |
| **S-9** | CKS MCP tool 확장: 운영 | `freshness` 추가 | P1 | S-11 | 미구현 |
| **S-10** | CKG client 인터페이스 확장 | ImpactOfChange, EvidenceForIntent, GetNodePRs, GetSubgraph 추가 | P1 | — | 미구현 |
| **S-11** | CKV client 인터페이스 확장 | Freshness 추가 | P1 | — | 미구현 |
| **S-12** | Hybrid Search RRF 정식 적용 | Stage 2의 BM25+Symbol 합산 → 정식 RRF | P1 | — | 부분 적용 |
| **S-13** | Cross-encoder Reranker | Stage 2~3 사이 reranking 단계 추가 | P2 | — | Tier 2 계획 |
| **S-14** | Smart dummy 구현 | CKV/CKG 완료 전 개발용. skill 기반으로 go-stablenet 코드 검색 | P1 | — | 미착수 |

### 3.4 coding-agent (Claude Code Plugin)

**역할**: 전체 워크플로우 진입점. 상태 머신 + Agent 파이프라인.

**현재 상태**: **설계 문서 완료, 코드 구현 미착수.**

| ID | 작업 (Phase 순서) | 설명 | 우선순위 | 의존 | 상태 |
|----|------------------|------|---------|------|------|
| **P-1** | Phase 1: Plugin skeleton | plugin.json, commands, state.json 관리 | P1 | — | 설계 완료, 구현 미착수 |
| **P-2** | Phase 2: Jira GW MCP | 프록시 MCP + Sensitive Filter | P1 | Atlassian MCP 인증 | 설계 완료, 구현 미착수 |
| **P-3** | Phase 3: CKS-CKV 연동 | ckv_search 호출 흐름 + 응답 처리 | **P0** | V-1, V-3 | 설계 완료, CKV에 의존 |
| **P-4** | Phase 4: CKS-CKG 연동 | ckg_query 호출 흐름 + 응답 처리 | P1 | G-1 | 설계 완료, CKG 준비됨 |
| **P-5** | Phase 5: Agent pipeline | Orchestrator/Planner/Implementer + hooks | P1 | P-3, P-4 | 설계 완료, 구현 미착수 |
| **P-6** | Phase 6: Evaluator + ChainBench | 테스트 파이프라인 + ChainBench MCP | P2 | ChainBench MCP | 설계 완료, ChainBench 미존재 |
| **P-7** | Phase 7: PR + Review cycle | PR 생성/리뷰 반영/Jira 업데이트 | P2 | P-5 | 설계 완료, 구현 미착수 |

### 3.5 외부 인프라

| ID | 작업 | 설명 | 우선순위 | 상태 |
|----|------|------|---------|------|
| **I-1** | Ollama 설치 + qwen3-embedding pull | `brew install ollama && ollama pull qwen3-embedding:0.6b` | **P0** | 미설치 |
| **I-2** | Atlassian MCP 인증 | Jira 접근 인증 완료 | P1 | 미설정 |
| **I-3** | ChainBench MCP 설계 | 로컬 go-stablenet 네트워크 테스트 MCP | P2 | 미착수 |

---

## 4. 작업 순서 (Critical Path)

### 4.1 Phase α: Real Embedder (모든 것의 기반)

```
I-1 (Ollama 설치)
  → V-1 (Ollama embedder 어댑터 구현)
    → V-2 (qwen3-embedding 연동 테스트)
```

**완료 기준**: CKV가 real semantic signal로 검색 결과 반환. eval fixture에서 recall 측정 가능.

### 4.2 Phase β: go-stablenet 검증

```
V-3 (go-stablenet CKV 인덱스 구축)    G-1 (go-stablenet CKG 인덱스 확인)
            │                                    │
            └──────────┬─────────────────────────┘
                       │
                 S-1 (CKS embedder 연결)
                 S-2 (go-stablenet config)
                       │
                 S-4 (E2E dogfood eval)
```

**완료 기준**: go-stablenet 대상 CKS get_for_task 호출 시 의미 있는 EvidencePack 반환. recall > 0.6.

### 4.3 Phase γ: 품질 향상 + 인터페이스 정합

```
V-4 (glossary)    V-5 (PR fixture 확장)    G-4 (real BM25 score)
      │                  │                        │
      └──── S-5 (vocab resolver) ────────── S-7 (RRF 정식 적용)
                                                  │
V-8 + G-5 + S-6 (coding-agent MCP 인터페이스 매핑)
```

**완료 기준**: go-stablenet 대상 recall > 0.8. coding-agent 설계의 MCP 인터페이스와 실제 CKS/CKG tool 매핑 완료.

### 4.4 Phase δ: Plugin MVP

```
P-1 (Plugin skeleton)
  → P-2 (Jira GW MCP)  → I-2 (Jira 인증)
  → P-3 (CKS-CKV 연동) → P-4 (CKS-CKG 연동)
    → P-5 (Agent pipeline)
```

**완료 기준**: `/work JIRA-123` 으로 Jira ticket → 분석 → 계획 → 설계 문서 생성까지 동작.

### 4.5 Phase ε: E2E 자동화

```
P-6 (Evaluator + ChainBench) → I-3 (ChainBench MCP)
P-7 (PR + Review cycle)
```

**완료 기준**: 전체 시나리오 (Jira → 코드 구현 → 테스트 → PR → 리뷰 반영 → Merge) 자동 실행.

---

## 5. CKS MCP 인터페이스 설계

### 5.1 아키텍처 원칙

**coding-agent는 CKS MCP만 호출한다. CKV/CKG를 직접 호출하지 않는다.**

```
coding-agent → CKS MCP tools (유일한 인터페이스)
                  │
                  └── CKS 내부에서 CKV, CKG, 자체 기능 조합하여 응답
```

### 5.2 CKS MCP Tool 전체 설계

coding-agent의 워크플로우 단계별 정보 요구를 분석하여 도출한 CKS MCP tool 세트:

**검색 (Search)**:

| tool | 역할 | 내부 사용 | coding-agent 사용 시점 | 현재 상태 |
|------|------|----------|---------------------|----------|
| `cks.context.get_for_task` | 종합 코드 검색. 자연어 입력 → EvidencePack | CKV semantic + CKG BM25 + graph + sanitize 전체 파이프라인 | ANALYSIS: 최초 정보 수집 | ✅ 구현됨 |
| `cks.context.semantic_search` | 의미 유사도 기반 코드 검색 | CKV 벡터 검색 | ANALYSIS: 추가 의미 검색이 필요할 때 | ❌ 미구현 |
| `cks.context.search_text` | 키워드 기반 텍스트 검색 (BM25) | CKG FTS5 + Okapi BM25 | ANALYSIS: 정확한 심볼명/키워드로 검색할 때 | ❌ 미구현 |

**그래프 탐색 (Graph Traversal)**:

| tool | 역할 | 내부 사용 | coding-agent 사용 시점 | 현재 상태 |
|------|------|----------|---------------------|----------|
| `cks.context.find_symbol` | 심볼 정의 위치 검색 (exact/suffix) | CKG FindSymbol | ANALYSIS: "이 함수 정의가 어디에 있나" | ❌ 미구현 |
| `cks.context.find_callers` | 특정 심볼을 호출하는 코드 탐색 | CKG Neighbors (reverse, calls/invokes) | ANALYSIS: "이 함수를 누가 호출하나" | ❌ 미구현 |
| `cks.context.find_callees` | 특정 심볼이 호출하는 코드 탐색 | CKG Neighbors (forward, calls/invokes) | ANALYSIS: "이 함수가 뭘 호출하나" | ❌ 미구현 |
| `cks.context.get_subgraph` | 심볼 주변 전체 관계 탐색 (모든 edge type) | CKG SubgraphByQname | ANALYSIS: 아키텍처 파악, 모듈 구조 이해 | ❌ 미구현 |

**분석 (Analysis)**:

| tool | 역할 | 내부 사용 | coding-agent 사용 시점 | 현재 상태 |
|------|------|----------|---------------------|----------|
| `cks.context.impact_analysis` | 코드 변경 시 영향 범위 분석 (6개 카테고리: callers, interface, type users, distributed, concurrent, other) | CKG impact_of_change | PLANNING: "이걸 수정하면 뭐가 깨지나" | ❌ 미구현 |
| `cks.context.change_history` | 심볼/파일의 수정 히스토리 + PR 정보 | CKG evidence_for_intent + PR breadcrumb (GetNodePRs) | ANALYSIS: "이 코드가 왜 이렇게 되었나" | ❌ 미구현 |

**운영 (Ops)**:

| tool | 역할 | 내부 사용 | coding-agent 사용 시점 | 현재 상태 |
|------|------|----------|---------------------|----------|
| `cks.ops.health` | CKV/CKG 백엔드 건강 상태 | CKV Health + CKG Health | 작업 시작 전 시스템 점검 | ✅ 구현됨 |
| `cks.ops.freshness` | 인덱스 신선도 (최신 코드 반영 여부) | CKV Freshness | 작업 시작 전 인덱스 상태 확인 | ❌ 미구현 |

**요약**: 현재 2개 → 11개 tool로 확장 필요.

### 5.3 CKG Client 인터페이스 업데이트

현재 CKS의 `ckgclient.Client` 인터페이스에 추가 필요한 메서드:

```
현재 (4개 메서드):
  BM25Search, FindSymbol, Neighbors, Health, Close

추가 필요 (4개 메서드):
  ImpactOfChange    — cks.context.impact_analysis 지원
  EvidenceForIntent — cks.context.change_history 지원
  GetNodePRs        — cks.context.change_history 지원 (PR breadcrumb)
  GetSubgraph       — cks.context.get_subgraph 지원
```

| 메서드 | 시그니처 (예상) | CKG 내부 대응 |
|--------|---------------|-------------|
| `ImpactOfChange` | `(ctx, seed_qname, opts) → ImpactResult` | `pkg/impact.Compute()` |
| `EvidenceForIntent` | `(ctx, intent, opts) → EvidencePack` | `pkg/evidence.BuildPack()` |
| `GetNodePRs` | `(ctx, nodeID, cutoff) → []PRRef` | `store.Reader.GetNodePRs()` |
| `GetSubgraph` | `(ctx, qname, depth) → ([]Node, []Edge)` | `store.Reader.SubgraphByQname()` |

### 5.4 CKV Client 인터페이스 업데이트

현재 CKS의 `ckvclient.Client` 인터페이스에 추가 필요한 메서드:

```
현재 (3개 메서드):
  SemanticSearch, Health, Close

추가 필요 (1개 메서드):
  Freshness — cks.ops.freshness 지원
```

| 메서드 | 시그니처 (예상) | CKV 내부 대응 |
|--------|---------------|-------------|
| `Freshness` | `(ctx) → FreshnessReport` | CKV MCP `cks.ops.get_freshness` |

### 5.5 워크플로우 커버리지 검증

coding-agent의 TICKET_INTAKE → COMPLETION 전체 상태를 11개 CKS MCP tool로 대조한 결과.

#### TICKET_INTAKE

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| Jira ticket 읽기 | Jira GW MCP | — | ✅ |
| Sensitive filter | Jira GW MCP 내장 | — | ✅ |
| workspace 폴더 생성 | coding-agent 자체 | — | ✅ |
| state.json 초기화 | coding-agent 자체 | — | ✅ |

CKS tool 사용 없음.

#### ANALYSIS (Planner Agent - Opus)

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| 티켓 내용으로 관련 코드 종합 검색 | `get_for_task` | ✅ |
| 추가 의미 검색 (첫 결과 부족 시) | `semantic_search` | ✅ |
| 특정 키워드로 정확 검색 | `search_text` | ✅ |
| 도메인 분류 + 작업 유형 결정 | LLM 추론 (CKS 아님) | ✅ |
| 관련 심볼 정의 위치 | `find_symbol` | ✅ |
| 심볼 호출자 (누가 호출하나) | `find_callers` | ✅ |
| 심볼 피호출자 (뭘 호출하나) | `find_callees` | ✅ |
| 동시성 영향 분석 | `impact_analysis` (concurrent 카테고리) | ✅ |
| 수정 히스토리 + PR 정보 | `change_history` | ✅ |
| 모듈 구조 파악 | `get_subgraph` | ✅ |
| 인터페이스 구현체 찾기 | `get_subgraph` (implements edges 포함) | ✅ |
| 테스트 코드 존재 여부 | `get_subgraph` (tested_by edges 포함) | ✅ |
| 인덱스 최신 상태 확인 | `freshness` | ✅ |

#### PLANNING (Planner Agent - Opus)

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| 수정 대상 함수의 callee 확인 | `find_callees` | ✅ |
| 수정이 다른 패키지에 영향 주는지 | `impact_analysis` | ✅ |
| 비슷한 수정이 과거에 있었는지 | `change_history` + `semantic_search` | ✅ |
| 단계별 검증 테스트 정의 | ANALYSIS 결과 + LLM 추론 | ✅ |

#### DESIGN (Planner Agent - Opus)

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| struct 필드 목록 확인 | `find_symbol` | ✅ |
| 함수 시그니처 확인 | `find_symbol` | ✅ |
| 실제 코드 본문 읽기 | Read tool (직접 파일 읽기) | ✅ |
| Self-review | LLM 추론 | ✅ |

#### IMPLEMENTATION (Implementer Agent - Sonnet)

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| plan + design 로드 | 로컬 파일 | ✅ |
| 브랜치 생성 | git (Bash) | ✅ |
| 코드 수정 수행 | Write/Edit tool | ✅ |
| 구현 중 참조할 코드 검색 | `find_symbol`, `search_text` | ✅ |
| 타입/인터페이스 확인 | `find_symbol` | ✅ |
| 분할 커밋 | git (Bash) | ✅ |

#### EVALUATION (Evaluator Agent - Sonnet)

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| Unit test | `go test` (Bash) | ✅ |
| Lint & Format | `golangci-lint` (Bash) | ✅ |
| Security scan | `go vet` (Bash) | ✅ |
| ChainBench | ChainBench MCP | ✅ |

CKS tool 사용 없음.

#### BUG/REVIEW CYCLE (EVALUATION_FAIL → ANALYSIS 재진입)

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| 이슈 내용 + git diff 수집 | git diff (Bash) | ✅ |
| 관련 원본 코드 검색 | `get_for_task` | ✅ |
| 실패한 함수의 의존성 | `find_callers`, `find_callees` | ✅ |
| 수정 코드의 영향 범위 | `impact_analysis` | ✅ |
| 과거에 비슷한 에러가 있었는지 | `change_history` + `semantic_search` | ✅ |
| 종합 추론 (unpushed + 원본 + 이슈) | LLM 추론 (git diff는 직접 컨텍스트) | ✅ |

미푸시 코드는 CKS 인덱스에 미반영. Agent가 git diff로 직접 읽어 LLM 컨텍스트에 포함 (HLD §4.5 옵션 A).

#### COMPLETION

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| PR 생성 | gh CLI | ✅ |
| Jira 댓글 업데이트 | Jira GW MCP | ✅ |
| Jira 상태 변경 | Jira GW MCP | ✅ |

CKS tool 사용 없음.

#### /review <PR-URL> 흐름

| 단계 | 동작 | CKS tool | 충족 |
|------|------|----------|------|
| 리뷰 코멘트 수집 | gh API | ✅ |
| 작업 폴더 탐색 | 로컬 파일 시스템 | ✅ |
| 리뷰 내용으로 관련 코드 검색 | `get_for_task` | ✅ |
| 코드 위치 확인 | `find_symbol` | ✅ |
| ANALYSIS 재진입 | ANALYSIS와 동일 | ✅ |

#### 커버리지 검증 결론

**11개 CKS MCP tool로 coding-agent의 전체 워크플로우가 커버됨. 누락 시나리오 없음.**

`get_subgraph`가 "인터페이스 구현체 찾기", "테스트 코드 찾기" 등 특정 관계 탐색의 범용(catch-all) 역할을 한다.
향후 이런 쿼리가 빈번하게 발생하면 전용 tool(`find_implementations`, `find_tests`)로 분리를 검토한다.

---

## 6. Skill 기반 Smart Dummy 전략

### 6.1 배경

CKV와 CKG 모두 현재 개발 진행 중이며 완성되지 않았다.
그러나 CKS에는 CKV/CKG 완료를 기다리지 않고 진행할 수 있는 작업이 존재한다.
또한 CKS의 11개 MCP tool 구현·테스트·coding-agent 연동을 CKV/CKG 완료 전에 시작할 수 있다면,
전체 프로젝트의 병렬 진행이 가능해진다.

### 6.2 Smart Dummy의 정의

Smart Dummy = **Claude Code 세션에서 skill을 활용하여 go-stablenet 프로젝트 코드를
실제로 읽고 분석하여 응답을 생성하는 임시 구현체.**

임의 데이터를 반환하거나, 다른 backend로 우회하는 것이 **아니다**.
Claude Code의 내장 tool(Read, Grep, Glob, Bash)을 skill이 조합하여
go-stablenet 코드를 실제로 검색·분석하고, CKS MCP tool과 동일한 형태의 응답을 구성한다.

```
[현재: Fake — 하드코딩 데이터 반환]
CKS tool 호출 → Fake client → 하드코딩 Hit[] 반환 → 무의미

[Smart Dummy — Claude Code skill이 실제 코드 분석]
CKS tool 호출 → Smart Dummy
                    │
                    ├─ Grep: go-stablenet 코드에서 키워드 검색
                    ├─ Read: 관련 파일 읽기
                    ├─ go/parser: AST 기반 심볼 탐색
                    └─ 결과를 CKS 인터페이스 포맷으로 조립
                    │
                → 의미 있는 응답 반환

[나중: Real CKV/CKG — 완성 후 교체]
CKS tool 호출 → Real CKV/CKG client → 인덱스 기반 검색 → 고품질 응답
```

### 6.3 교체 전략

CKS의 `ckvclient.Client`와 `ckgclient.Client` 인터페이스가 이미 정의되어 있다.
Smart Dummy는 이 인터페이스를 구현하므로, real CKV/CKG 완료 시 **인터페이스 변경 없이 교체**한다.

```
단계 1 (현재):  CKS → Fake (하드코딩)
단계 2 (Dummy): CKS → Smart Dummy (skill 기반 실제 코드 분석)
단계 3 (Real):  CKS → Real CKV/CKG (인덱스 기반)
```

교체는 CKS 설정(config)에서 backend를 지정하는 것으로 수행.
인터페이스 계약이 동일하므로 CKS 파이프라인 코드, MCP tool 핸들러, 
coding-agent 호출 코드 모두 변경 없음.

### 6.4 Smart Dummy가 담당하는 CKS MCP tool별 동작

| CKS MCP tool | Smart Dummy 동작 방식 |
|-------------|---------------------|
| `get_for_task` | Grep + Read로 관련 코드 검색 → 간이 EvidencePack 조립 |
| `semantic_search` | Grep 기반 키워드 매칭 (벡터 검색 대신) |
| `search_text` | Grep/ripgrep 기반 텍스트 검색 |
| `find_symbol` | go/parser로 심볼 정의 탐색, 또는 Grep으로 `func <name>` 패턴 매칭 |
| `find_callers` | Grep으로 함수명 호출 패턴 검색 + Read로 컨텍스트 확인 |
| `find_callees` | Read로 함수 본문 읽기 → 함수 호출 패턴 추출 |
| `get_subgraph` | find_callers + find_callees 결합 |
| `impact_analysis` | find_callers 기반 역방향 탐색 (간이 버전) |
| `change_history` | git log --follow로 파일/함수 변경 이력 수집 |
| `health` | dummy 상태 반환 |
| `freshness` | git diff HEAD로 변경 파일 확인 |

Smart Dummy는 real CKV/CKG보다 **정확도가 낮고 속도도 느리다**.
그러나 **실제 코드를 분석하므로 의미 있는 결과를 반환**하며,
CKS 파이프라인 전체를 E2E로 테스트하고 coding-agent 연동을 검증하기에 충분하다.

### 6.5 Smart Dummy로 병렬 진행 가능한 작업

| 작업 | Smart Dummy로 진행 가능 여부 | 이유 |
|------|---------------------------|------|
| S-6~S-9: CKS MCP tool 핸들러 구현 | ✅ | dummy backend로 동작 확인 가능 |
| S-10: CKG client 인터페이스 확장 | ✅ | 인터페이스 정의는 backend 독립 |
| S-11: CKV client 인터페이스 확장 | ✅ | 인터페이스 정의는 backend 독립 |
| S-5: Vocabulary resolver | ✅ | CKS 내부 기능, backend 독립 |
| S-12: Hybrid Search RRF | ✅ | dummy 결과로 로직 테스트 가능 |
| S-14: Smart Dummy 자체 구현 | ✅ | — |
| P-1~P-5: coding-agent plugin | ✅ | CKS MCP tool이 dummy로 동작하면 연동 테스트 가능 |
| D-1: 도메인 지식 목록 정리 | ✅ | backend 독립 |

CKV/CKG 완료를 기다려야 하는 작업:
| 작업 | 이유 |
|------|------|
| S-1: Real embedder 연결 | CKV real embedder 필요 |
| S-4: E2E eval (정량 측정) | real backend 기반 recall/precision 측정 |
| V-6: Real eval 실행 | real embedder 필요 |

### 6.6 수정된 작업 순서

```
Phase 1: Smart Dummy + 인터페이스 구축 (CKV/CKG 완료 불필요)
──────────────────────────────────────────────────────────────
S-14: Smart Dummy 구현 (CKV/CKG 인터페이스에 맞는 skill 기반 구현체)
S-10: CKG client 인터페이스 확장 (4개 메서드 추가)
S-11: CKV client 인터페이스 확장 (Freshness 추가)
S-6~S-9: CKS MCP 9개 tool 핸들러 구현 (Smart Dummy backend)

Phase 2: CKS 내부 기능 + coding-agent 연동 (CKV/CKG 완료 불필요)
──────────────────────────────────────────────────────────────
S-5: Vocabulary resolver
S-12: Hybrid Search RRF
D-1: 도메인 지식 목록 정리

Phase 3: coding-agent Plugin MVP (CKV/CKG 완료 불필요)
──────────────────────────────────────────────────────────────
P-1~P-5: Plugin skeleton → Agent pipeline
(CKS Smart Dummy로 실제 동작 확인)

Phase 4: Real backend 교체 (CKV/CKG 완료 후)
──────────────────────────────────────────────────────────────
S-1: Real embedder 연결
Smart Dummy → Real CKV/CKG 교체 (config 변경)
S-4: E2E eval (정량 측정)
```

이 순서의 핵심: **Phase 1~3은 CKV/CKG 개발과 완전히 병렬 진행 가능**.
CKV/CKG 작업자와 CKS/coding-agent 작업자가 동시에 진행하고,
CKV/CKG 완료 시 Phase 4에서 교체만 수행.

---

## 7. 방향 확인 요약

### 7.1 올바르게 진행 중인 것

| 항목 | 근거 |
|------|------|
| 4-프로젝트 분리 아키텍처 | coding-agent / cks / ckv / ckg 각각 독립 책임 |
| State machine + Document-driven handoff | coding-agent HLD §8: 구조화된 상태 전이 + 아티팩트 기반 통신 |
| Pre-LLM Security Gate | Jira GW MCP + CKS 내장 filter 이중 방어 |
| Agent 모델 라우팅 | Planner: Opus, Impl/Eval: Sonnet — 비용/품질 균형 |
| PR 기반 평가 전략 | SWE-bench 표준과 동일 발상 |

### 7.2 수정/보완이 필요한 것

| 항목 | 현재 상태 | 필요 조치 |
|------|----------|----------|
| **CKV** | 개발 중 (real embedder 미연동) | Ollama + qwen3-embedding 연동 (V-1) |
| **CKG** | 개발 중 (수정 진행 중) | 완성 후 CKS real backend 교체 |
| **CKS MCP tool** | 2개만 존재 | 11개로 확장 (S-6~S-9) |
| **CKS backend** | Fake (하드코딩) | Smart Dummy 구현 → 나중에 real 교체 (S-14) |
| **ChainBench MCP** | 미존재 | Phase ε에서 별도 설계 (I-3) |
| **coding-agent 코드** | 설계 완료, 구현 미착수 | CKS Smart Dummy로 연동 테스트 가능 |

---

## 8. 도메인 지식 코퍼스 (Domain Knowledge Corpus)

### 8.1 왜 코드만으로는 부족한가

CKV가 코드만 임베딩하면, 자연어 쿼리와 코드 사이의 의미 연결이 끊어진다.

```
쿼리: "합의 실패 시 블록 롤백"

[코드만 임베딩]
코드에 "합의 실패"라는 텍스트 없음. Finalize(), verifyVotes() 등 함수명만 존재.
→ 의미 검색 실패 또는 부정확

[도메인 지식 포함]
도메인 문서: "WBFT에서 quorum(2f+1) 미달 시 Finalize 실패. block은 pending 유지."
→ "합의 실패" ↔ 도메인 문서 ↔ Finalize() 코드 연결
→ 정확한 검색
```

도메인 지식 문서는 자연어 쿼리(한국어/모호 표현)와 코드 심볼 사이의 **의미 브릿지** 역할을 한다.

### 8.2 현재 go-stablenet에 존재하는 문서

| 문서 | 줄 수 | 내용 | 임베딩 적합도 |
|------|------|------|-------------|
| `.claude/docs/CLAUDE_DEV_GUIDE.md` | 1241 | 아키텍처, 제약사항, 개발 규칙 | ✅ 높음 |
| `.claude/docs/SYSTEM_CONTRACT_FLOW.md` | 558 | 시스템 컨트랙트 배포/초기화/업그레이드 흐름 | ✅ 높음 |
| `.claude/docs/BUILD_SOURCE_FILES.md` | 453 | 빌드 참여 파일 목록 | ⚠️ 중간 (목록 위주) |
| `.claude/docs/REVIEW_GUIDE.md` | 161 | 코드 리뷰 가이드 | ⚠️ 중간 |
| `.claude/docs/review-test-result-with-ast.md` | 301 | 시스템 컨트랙트 체크리스트 | ✅ 높음 |
| `CLAUDE.md` | ~80 | 프로젝트 개요 | ✅ 높음 |
| `docs/postmortems/2021-08-22-split-postmortem.md` | ? | 장애 사후 분석 | ✅ 높음 |

이 문서들은 **실무 가이드**로서 유용하지만, 개념 수준의 도메인 지식이 부족하다.

### 8.3 추가로 필요한 도메인 지식 카테고리

| 카테고리 | 예시 내용 | 현재 존재 | 코드와의 연결 |
|----------|----------|----------|-------------|
| **블록체인 기본 개념** | block, transaction, state trie, receipt, EVM | ❌ | core/, core/types/, core/vm/ |
| **BFT 합의 원리** | WBFT/QBFT 동작, quorum, round change, preprepare/prepare/commit 메시지 | 부분 | consensus/wbft/ |
| **시스템 컨트랙트 개념** | 거버넌스, 밸리데이터, NCP, 코인, 하드포크 업그레이드 | ✅ 있음 | systemcontracts/ |
| **go-stablenet 고유 개념** | fee delegation, native stablecoin, genesis 구조 | 부분 | core/types/tx_fee_delegation.go, core/stablenet_genesis.go |
| **동시성 패턴** | miner↔consensus goroutine, istanbul message handler 동시성 | ❌ | consensus/wbft/, eth/handler_istanbul.go |
| **네트워크 프로토콜** | istanbul message, peer discovery, tx propagation | ❌ | eth/, p2p/ |
| **geth 원본 vs 포크 차이** | 어디를 수정했고 왜 수정했는지 | ❌ | 전체 (고유 파일 목록은 CLAUDE.md에 있음) |
| **장애 시나리오** | split, stall, reorg, double sign, quorum loss | 부분 (1건) | consensus/, core/ |

### 8.4 도메인 지식 코퍼스 작업 원칙

**잘못된 정보가 임베딩되면 검색 결과가 오히려 악화된다.** 따라서:

1. **별도 작업으로 분리**: 임베딩할 데이터를 먼저 정리하고 검증한 후에 인덱싱한다
2. **사람 검증 필수**: LLM이 초안을 작성하더라도, 도메인 전문가가 사실 검증을 수행한다
3. **코드 연결 명시**: 각 도메인 문서에 관련 코드 경로/심볼을 명시하여 검색 연결성 보장
4. **점진 추가**: 한 번에 모든 카테고리를 채우지 않고, 우선순위에 따라 점진적으로 추가
5. **버전 관리**: 도메인 문서도 go-stablenet 레포 내 `.claude/docs/` 에 커밋하여 변경 추적

### 8.5 도메인 지식 코퍼스 작업 리스트

| ID | 작업 | 설명 | 우선순위 | 의존 |
|----|------|------|---------|------|
| **D-1** | 임베딩 대상 데이터 목록 정리 | 카테고리별로 어떤 내용을 문서화할지 구체적 항목 나열 | **P0** | — |
| **D-2** | 기존 문서 적합성 검토 | go-stablenet `.claude/docs/` 기존 7개 문서의 정확성·완성도 검증 | P1 | D-1 |
| **D-3** | BFT 합의 원리 문서 작성 | WBFT/QBFT 동작, 메시지 흐름, quorum, round change. consensus/wbft/ 코드 연결 명시 | P1 | D-1 |
| **D-4** | go-stablenet 고유 개념 문서 보강 | fee delegation, native stablecoin, genesis 구조. 코드 경로 연결 | P1 | D-1 |
| **D-5** | 동시성 패턴 문서 작성 | miner↔consensus, istanbul message handler 동시성. lock/channel 사용 패턴 | P2 | D-1 |
| **D-6** | geth 포크 차이 문서 작성 | 수정 지점과 수정 이유. CLAUDE.md 고유 파일 목록을 기반으로 확장 | P2 | D-1 |
| **D-7** | 장애 시나리오 문서 확장 | split, stall, reorg, quorum loss. 기존 postmortem 확장 | P2 | D-1 |
| **D-8** | 검증 완료 문서 인덱싱 | 검증된 도메인 문서를 CKV 인덱스에 포함하여 빌드 | P1 | D-2~D-4, V-3 |

### 8.6 기술적 참고: CKV의 문서 임베딩 지원 현황

CKV는 이미 `.md` 파일을 파싱하여 heading 단위로 청크를 분할하고 임베딩하는 기능을 갖추고 있다:
- `internal/parse/markdown/parser.go`: heading-section 단위 청크 분할
- `internal/parse/prdoc/parser.go`: PR description/commit message 청크 분할
- Chunk types: `ChunkDoc` (DocSection), `ChunkDoc` (ADRSection), `ChunkPRBackground`, `ChunkPRSolution`, `ChunkCommitMessage`

따라서 도메인 지식 문서가 go-stablenet 레포의 `.claude/docs/` 에 `.md` 파일로 추가되면,
`ckv build --src=<go-stablenet>` 실행 시 코드와 함께 자동으로 임베딩된다.
**CKV 코드 수정은 필요 없다.**

---

## 9. 전체 작업 ID 통합 인덱스

### 9.1 CKV 작업

| ID | 작업 | 우선순위 |
|----|------|---------|
| V-1 | Ollama embedder 어댑터 구현 | **P0** |
| V-2 | qwen3-embedding:0.6b 연동 테스트 | **P0** |
| V-3 | go-stablenet 인덱스 구축 | **P0** |
| V-4 | go-stablenet glossary 작성 | P1 |
| V-5 | PR fixture 12개 확장 | P1 |
| V-6 | Real embedder 기준 eval 실행 | P1 |
| V-7 | Contextual Enrichment 확장 | P2 |
| V-8 | coding-agent MCP tool 인터페이스 확인 | P1 |

### 9.2 CKG 작업

| ID | 작업 | 우선순위 |
|----|------|---------|
| G-1 | go-stablenet 인덱스 최신화 확인 | **P0** |
| G-2 | store.Reader API 검증 | P1 |
| G-3 | PR breadcrumb 검증 | P1 |
| G-4 | Real BM25 Score 노출 (F-1) | P1 |
| G-5 | coding-agent MCP tool 인터페이스 확인 | P1 |
| G-6 | CamelCase FTS5 토크나이저 | P2 |

### 9.3 CKS 작업

| ID | 작업 | 우선순위 |
|----|------|---------|
| S-1 | Ollama embedder → intent classifier 연결 | **P0** |
| S-2 | go-stablenet config 작성 | **P0** |
| S-3 | go-stablenet eval 시나리오 작성 | P1 |
| S-4 | E2E dogfood eval (go-stablenet) | P1 |
| S-5 | Vocabulary resolver 구현 | P1 |
| S-6 | CKS MCP tool 확장: `semantic_search`, `search_text` | P1 |
| S-7 | CKS MCP tool 확장: `find_symbol`, `find_callers`, `find_callees`, `get_subgraph` | P1 |
| S-8 | CKS MCP tool 확장: `impact_analysis`, `change_history` | P1 |
| S-9 | CKS MCP tool 확장: `freshness` | P1 |
| S-10 | CKG client 인터페이스 확장: ImpactOfChange, EvidenceForIntent, GetNodePRs, GetSubgraph | P1 |
| S-11 | CKV client 인터페이스 확장: Freshness | P1 |
| S-12 | Hybrid Search RRF 정식 적용 | P1 |
| S-13 | Cross-encoder Reranker | P2 |
| S-14 | Smart dummy 구현 (CKV/CKG 완료 전 개발용) | P1 |

### 9.4 Domain Knowledge Corpus 작업

| ID | 작업 | 우선순위 |
|----|------|---------|
| D-1 | 임베딩 대상 데이터 목록 정리 | **P0** |
| D-2 | 기존 문서 적합성 검토 | P1 |
| D-3 | BFT 합의 원리 문서 작성 | P1 |
| D-4 | go-stablenet 고유 개념 문서 보강 | P1 |
| D-5 | 동시성 패턴 문서 작성 | P2 |
| D-6 | geth 포크 차이 문서 작성 | P2 |
| D-7 | 장애 시나리오 문서 확장 | P2 |
| D-8 | 검증 완료 문서 인덱싱 | P1 |

### 9.5 coding-agent 작업

| ID | 작업 | 우선순위 |
|----|------|---------|
| P-1 | Phase 1: Plugin skeleton | P1 |
| P-2 | Phase 2: Jira GW MCP | P1 |
| P-3 | Phase 3: CKS-CKV 연동 | **P0** |
| P-4 | Phase 4: CKS-CKG 연동 | P1 |
| P-5 | Phase 5: Agent pipeline | P1 |
| P-6 | Phase 6: Evaluator + ChainBench | P2 |
| P-7 | Phase 7: PR + Review cycle | P2 |

### 9.6 외부 인프라 작업

| ID | 작업 | 우선순위 |
|----|------|---------|
| I-1 | Ollama 설치 + qwen3-embedding pull | **P0** |
| I-2 | Atlassian MCP 인증 | P1 |
| I-3 | ChainBench MCP 설계 | P2 |
