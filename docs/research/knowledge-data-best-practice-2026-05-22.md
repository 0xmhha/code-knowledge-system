# Knowledge Data Best Practice & 권장 기술 스택

> 작성: 2026-05-22
> 대상: go-stablenet 코드 이해/수정 지원을 위한 cks/ckg/ckv 통합 평가
> Status: research (구현 시작 전 의사결정 근거)

## 0. Executive Summary

사용자가 명세한 ckv의 실제 역할은 vector retrieval만이 아니라 **"한국어/모호 표현 → 코드 정확 키워드 변환"** 까지 포함한다. 이건 ADR-003이 가정한 vector-only 모델로는 *충분하지 않다*. 그러나 ADR-003을 영구 supersede하기 전에 **3-leg BM25 (ckv/ckg/cks 모두 임시 BM25) → 평가로 위치 결정** 이라는 사용자 방향이 정직한 경로다. 이미 ckg `pkg/bm25.Scorer` 인터페이스가 깔끔하게 추상화되어 있어 (Index/Score/TopK + Document/ScoredDoc) 재사용에 큰 비용이 없다.

본질적 빠진 조각은 **Vocabulary Bridge** — 도메인 어휘집(한국어 ↔ 코드 정확 용어) 자동 추출/적용이다. 이건 코드 변경 없이도 `.claude/docs/CLAUDE_DEV_GUIDE.md`와 `SYSTEM_CONTRACT_FLOW.md`에서 자동 추출 가능하다.

---

## 1. 사용자 명세 → 본질

### 1.1 ckv의 *진짜* 역할 (사용자가 2026-05-22 명시)

```
[사람의 모호한 지시]
    ↓ "0번 블록 어떻게 처리해?"
[ckv: vocabulary bridge + semantic search]
    ├─ 한국어/모호 → 코드 정확 용어 매핑
    │   "0번 블록" → "genesis", "genesis block",
    │                "GenesisAlloc", "core/genesis.go"
    └─ 사람 의도 → 관련 코드 chunk 추출 (현재 구현된 부분)
    ↓ 확장된 정확 키워드
[ckg: 정확 키워드 → 그래프 정확 검색] (이미 잘 됨)
    ↓ 정확 노드/엣지/시그니처
[cks: 두 결과 fusion → EvidencePack]
    ↓ 통합 컨텍스트
[coding agent → Claude / Cursor / ...]
```

**핵심 관찰**: ckv의 **두 번째 역할 (vocabulary bridge)** 이 현재 ckv 코드에 *명시적 구현이 없다*. evaluation-design 문서의 Phase 1~5에도 빠져 있다. 이게 evaluation 시 hallucination이 잘 안 잡히는 진짜 원인일 가능성.

### 1.2 사용자 예시 (한국어 ↔ 코드 매핑)

1. "0번 블록" ↔ genesis, genesis block, GenesisAlloc, core/genesis.go
2. "합의 알고리즘", "합의" ↔ consensus, wbft
3. "시스템 컨트랙트" ↔ system contract, `*.sol`, 관련 `*.go`

go-stablenet `.claude/docs/CLAUDE_DEV_GUIDE.md` 정독 결과 **이미 산재된 형태로 존재** — 자동 추출 가능 (75% 정도, 명시적 표는 없으나 본문에 매핑 명시).

---

## 2. R-A — Code RAG에서 Semantic Gap 해결 Best Practice

### 2.1 기술 카탈로그

| # | 기법 | 동작 | 비용 | 코드 도메인 적합도 | 작은 corpus 적합도 |
|---|---|---|---|---|---|
| 1 | **Hybrid dense+sparse (RRF)** | vector + BM25를 reciprocal rank fusion으로 결합 | 낮음 | ★★★★★ | ★★★★★ |
| 2 | **Domain glossary** | 사람/문서 관리 명시적 매핑표 (한국어 ↔ 코드) | 매우 낮음 | ★★★★★ | ★★★★★ |
| 3 | **Query expansion (rule-based)** | glossary 기반 alias 추가 | 낮음 | ★★★★ | ★★★★★ |
| 4 | **HyDE** (Gao 2022) | LLM이 hypothetical answer → 그 임베딩으로 검색 | 중간 (LLM 호출) | ★★★ | ★★★ |
| 5 | **MultiQuery** | LLM이 1 query → N개 다양 표현 → fusion | 중간 | ★★★★ | ★★★ |
| 6 | **Cross-encoder reranker** | top-30 vector hit → cross-encoder 재정렬 (BGE Reranker M3 등) | 중간 | ★★★★★ | ★★★★ |
| 7 | **Entity linking / canonicalization** | surface form ↔ canonical qname (fuzzy + dict) | 낮음 | ★★★★★ | ★★★★★ |
| 8 | **Knowledge Graph + Vector (GraphRAG)** | Microsoft GraphRAG 패턴 — graph traversal + chunk embedding 결합. 본 프로젝트는 이미 ckg(graph)+ckv(vector)로 정렬됨 | 큼 (구조 변경) | ★★★★★ | ★★★ (작은 corpus엔 과함) |
| 9 | **Multi-vector / ColBERT** | 토큰별 임베딩 + late interaction | 큼 (인덱스 크기↑) | ★★★★ | ★★★ |
| 10 | **Symbol-aware chunking** | AST/symbol 경계로 chunk 분할 (ckv 이미 적용) | 낮음 | ★★★★★ | ★★★★★ |
| 11 | **Iterative agentic RAG** | LLM이 검색 결과 보고 다시 query | 매우 큼 | ★★★ | ★★ (느림) |
| 12 | **Embedder fine-tune (LoRA)** | Korean+code 페어로 작은 fine-tune | 매우 큼 | ★★★★ | ★★ (데이터 부족) |

### 2.2 실제 도구 사례 매핑

| 도구 | 사용 패턴 | 참고할 점 |
|---|---|---|
| Sourcegraph Cody | LSIF graph + dense embeddings + symbol search hybrid | symbol resolution 정확도 — qname canonical 잡는 방식 |
| Aider | repomap (ranked AST graph, no embeddings) + sparse retrieval | 작은 corpus엔 graph만으로도 충분 가능 신호 |
| bloop | dense + sparse + LLM reranker | 3단 hybrid 파이프라인 사례 |
| Continue.dev | per-language tree-sitter chunking + embeddings | symbol-level chunking 정착 사례 (ckv 이미 적용) |
| Microsoft GraphRAG | LLM이 entity/relation 추출 → graph + community → query 시 graph traversal | go-stablenet은 이미 ckg가 graph → community summary만 추가하면 적용 가능 |
| RepoFusion / CodeRetriever 논문 | code-specific retrieval 평가 메트릭 (recall@k / MRR / NDCG) | 평가 메트릭 정착 사용 |

### 2.3 정석 패턴 (knowledge data 구성 best practice)

```
[Raw Code + Docs]
    │
    ├─→ AST/symbol chunking (보존된 의미 단위)
    │       └─→ Embedding (multilingual, domain-finetune 옵션)
    │              └─→ Vector store
    │
    ├─→ Sparse index (BM25, code-aware tokenizer)
    │       └─→ Inverted index
    │
    ├─→ Graph extraction (AST + call graph + type hierarchy)
    │       └─→ Graph store (이미 ckg)
    │
    └─→ Domain glossary 추출 (문서 + 코드 주석)
            └─→ Vocabulary mapping table

[Query 시]
    User query (모호/한국어/혼합)
        │
        ├─→ Vocabulary resolver (glossary lookup + LLM disambiguation)
        │       → canonical keywords + aliases
        │
        ├─→ Hybrid retrieval (vector + BM25, RRF fusion)
        │
        ├─→ Graph traversal (related symbols, callers/callees)
        │
        └─→ Cross-encoder rerank → top-K
            → Citation-enforced Evidence Pack
```

이 패턴이 **GraphRAG + Hybrid retrieval + Glossary**의 통합형이고, 본 프로젝트는 이미 80% 이상 정렬되어 있다.

---

## 3. R-B — go-stablenet 환경 권장 기술 스택

### 3.1 환경 특성 (제약 식별)

| 특성 | 영향 |
|---|---|
| Corpus 작음 (전체 781 file, 고유 영역 ~80 file) | fine-tune 비현실, glossary는 가능 |
| 도메인 특수성 강함 (WBFT/gov_*/system contracts) | 일반 모델로는 의미 약함, glossary 가치 큼 |
| Korean queries | multilingual embedder + glossary 매핑 필수 |
| Go + Solidity 혼재 | per-language tokenizer/chunker — ckv 이미 적용 |
| 풍부한 .claude/docs | 자동 glossary 추출 가능 |
| BUILD_SOURCE_FILES.md 명세 명확 | 인덱싱 범위 결정 비용 0 |

### 3.2 권장 스택 (Tier 1 → Tier 3 점진 도입)

**Tier 1: 필수 — 즉시 적용 (~1주)**

| 컴포넌트 | 위치 | 이유 |
|---|---|---|
| T1.A Hybrid retrieval (vector + BM25) | ckv 단독 모드 + cks fusion 둘 다 | 사용자 R4 직접 충족. ckg `pkg/bm25.Scorer` 재사용 |
| T1.B Domain glossary (자동 추출) | cks 신규 모듈 + glossary YAML | .claude/docs 파싱 → 한국어/영문 매핑표 자동 생성 |
| T1.C Query expansion (rule-based) | cks 신규 모듈 | T1.B glossary를 query 전처리 단계에 wire |
| T1.D Symbol qname alignment | ckv `Chunk.CKGNodeID` 활용 | ckg ↔ ckv 일관 정합 — citation 정확도↑ |

Tier 1 효과 예상: recall +15~25%, hallucination -30~50% (best practice 사례 기준).

**Tier 2: 큰 효과 — 평가 후 도입 (~2주)**

| 컴포넌트 | 위치 | 이유 |
|---|---|---|
| T2.E Cross-encoder reranker | cks fusion 직전 | top-30 → top-5 정확도. BGE Reranker M3 (multilingual) |
| T2.F Stable-net 고유 영역 corpus (D6-B) | ckv 인덱싱 입력 | fork 영역 제외 → 신호/잡음비↑ |
| T2.G IntentRouter 확장 | cks (이미 부분 적용) | intent별 다른 BM25 가중치 / vocab profile |

**Tier 3: Research — 평가 안정 후 (~1-2개월)**

| 컴포넌트 | 위치 | 이유 |
|---|---|---|
| T3.H HyDE | coding agent layer (cks는 LLM 호출 금지 원칙) | 모호 query 보강. coding agent repo로 위치 |
| T3.I GraphRAG community summary | ckg 측 | 모듈 간 관계 요약. 본 corpus 작아서 우선순위↓ |
| T3.J Embedder fine-tune | 별도 | 데이터 부족 → 효과 불확실 |

### 3.3 권장 시작점

```
Tier 1 (1주) → 평가 N=30 한국어 query 측정 → 결정:
  - BM25 위치 (ADR-003 supersede 여부)
  - glossary 자동화 vs 사람 큐레이션
  - reranker 도입 여부
```

---

## 4. R-C — ckg ↔ ckv 키워드 공유 기술

이미 부분적으로 설계되어 있음. ckv `Chunk` 구조에 **`CKGNodeID` 필드가 존재** — 양쪽 alignment를 의도한 흔적.

### 4.1 옵션 비교

| 옵션 | 동작 | 작업 위치 | 비용 | 권장도 |
|---|---|---|---|---|
| C1 Shared qname (canonical name) | 동일 베이스 코드에서 같은 symbol에 같은 qname. 양쪽 `commit_hash` 일치 강제 | ckv `Chunk.CKGNodeID` 활용 + cks가 검증 | 작음 | ★★★★★ |
| C2 Shared manifest (sibling indexes) | ckg manifest와 ckv manifest를 sibling으로 두고 cks가 dual-mount | cks `internal/manifest` 모듈 추가 | 작음 | ★★★★ |
| C3 Symbol Resolution Service | 별도 mini-service — surface name → canonical qname | cks 신규 모듈 | 중간 | ★★★ |
| C4 Glossary file (committed) | `docs/CODE_GLOSSARY.yaml` — 도메인 어휘 + canonical mapping. 양쪽이 학습 | go-stablenet repo 또는 cks repo | 작음 | ★★★★★ |
| C5 Cross-tool query orchestration | cks가 모호 query → ckv vocab expand → expanded keywords로 ckg → fusion | cks core 변경 | 중간 | ★★★★ |

### 4.2 권장 조합

```
C1 (qname canonical) + C2 (manifest sibling) + C4 (glossary)
    → cks가 C5 orchestration 패턴으로 묶음
```

**근거**:
- C1+C2는 *데이터 레이어* 일관성 (commit_hash + qname 정합)
- C4는 *도메인 어휘 레이어* — 사람이 큐레이션 가능한 YAML
- C5는 *런타임 통합* — cks가 conductor 역할

C3는 cks가 이미 그 역할을 하므로 *중복* — 회피.

---

## 5. R-D — cks 측 신규 기능 식별

R-C 권장 조합 적용 시 cks에 필요한 신규 기능:

| # | 모듈 | 위치 | 역할 | 효과 |
|---|---|---|---|---|
| D1 | `internal/glossary/` | cks 신규 | .claude/docs → YAML 추출 + loader | T1.B/C 기반 |
| D2 | `internal/vocab/resolver.go` | cks 신규 | 모호 query → canonical keywords (glossary + fuzzy match) | T1.C |
| D3 | `internal/manifest/dual.go` | cks 신규 | ckg + ckv manifest 정합 검증 (commit_hash 일치) | C2 |
| D4 | `internal/fusion/rrf.go` | cks 신규 (또는 기존 stage2 확장) | RRF fusion across (ckv vector / ckv-BM25 / ckg-BM25 / ckg-graph) | T1.A 완성 |
| D5 | `internal/bm25/local.go` | cks 임시 | cks 측 BM25 — 평가 비교용 (사용자 결정 "3-leg BM25"). ckg `pkg/bm25.Scorer` 재사용 | 평가 |
| D6 | `internal/reranker/` | cks 신규 (Tier 2) | BGE Reranker M3 — top-30 → top-5 | T2.E |
| D7 | `internal/intent/` | cks 기존 확장 | intent별 vocab profile + fusion weight | T2.G |

### 5.1 Effort estimate (cks 기준)

| 모듈 | LOC | 외부 의존 | 우선순위 |
|---|---|---|---|
| D1 glossary loader | ~150 | YAML | Tier 1 |
| D2 vocab resolver | ~200 | rapidfuzz (옵션) | Tier 1 |
| D3 dual manifest | ~80 | — | Tier 1 |
| D4 RRF fusion | ~100 | — | Tier 1 |
| D5 cks-local BM25 (임시) | ~50 (ckg `pkg/bm25` import) | ckg | Tier 1 (평가용) |
| D6 reranker | ~300 + 모델 download | ONNX | Tier 2 |
| D7 intent enhancement | ~150 | — | Tier 2 |
| **Tier 1 total** | **~580 LOC** | — | 1주 예상 |
| **Tier 2 total** | **~450 LOC** | — | 2주 예상 |

---

## 6. D6 — Corpus 명세 (인덱싱 입력 확정)

### 6.1 핵심 (반드시 포함 — 사용자 명시)

`.claude/skills`가 정의/언급하는 파일 + `BUILD_SOURCE_FILES.md` 빌드 참여 파일:

```
# 코드 (StableNet 고유 영역, ~80 files)
consensus/wbft/**/*.go                  # 39 files (7 packages)
systemcontracts/*.go                    # 9 files
systemcontracts/*.sol                   # Solidity 원본
cmd/gstable/**/*.go                     # 11 files
cmd/genesis_generator/**/*.go           # 3 files
core/stablenet_genesis.go
core/types/{istanbul,state_account_extra,tx_fee_delegation}.go
core/vm/native_manager.go
params/{config_wbft,network_params}.go
eth/{handler_istanbul,quorum_protocol}.go
eth/gasprice/anzeon.go
eth/protocols/eth/qlight_deps.go

# 도메인 문서 (glossary 추출 source)
CLAUDE.md
.claude/docs/CLAUDE_DEV_GUIDE.md
.claude/docs/SYSTEM_CONTRACT_FLOW.md
.claude/docs/BUILD_SOURCE_FILES.md
.claude/docs/REVIEW_GUIDE.md
.claude/docs/review-test-result-with-ast.md

# 워크플로우 (trigger 키워드 + 절차)
.claude/skills/{complexity,qr-gate,delta-log,milestone,handoff,
  check-reviews,do-review,pr-review}/SKILL.md
.claude/commands/stablenet-review-code.md
```

### 6.2 추가 학습 후보 (사용자 질문 "어떤 정보가 더 추가되면 좋을지")

| 후보 | 효과 | 비용 | 권장도 |
|---|---|---|---|
| G1: 빌드 참여 781 file 전체 (geth fork 영역 포함) | go-stablenet 전체 컨텍스트 | 인덱싱 6h+. throughput buffer 확보 후 | Tier 3 |
| G2: `.git/log` (커밋 메시지 + diff stat) | "이 함수는 왜 추가됐는가" history-aware query | ckv Phase C에서 다룰 예정 | Tier 2 |
| G3: `tests/` 파일 (특히 시나리오) | "이 기능이 어떻게 사용되는가" 예시 | 작음 — 인덱싱만 | Tier 1 |
| G4: chainbench 시나리오 (`/Users/.../chainbench/tests/*.sh`) | E2E 동작 예시 → 사용자 의도 추론 도움 | 작음, 별도 corpus 권장 | Tier 2 |
| G5: 공식 문서 / RFC (있다면) | 외부 표준 vs 내부 구현 매핑 | 사용자 확인 필요 | 조건부 |
| G6: 이전 PR 리뷰 코멘트 (GitHub) | 도메인 어휘 + 결정 근거의 자연 source | 큰 가치, 큰 수집 비용 | Tier 2/3 |
| G7: HLD 문서 (`stablenet-ai-agent/claudedocs/`) | 시스템 전체 모델 | 외부 path, 사용자 결정 | 조건부 |

**권장 시작점**: 핵심 + G3 (tests 시나리오). G1/G2는 throughput / Phase C 작업 진척 후.

### 6.3 Negative fixture (D5-D, 사용자 R3 충족)

```yaml
# 존재하지 않는 기능 → 빈 결과 / 낮은 score 기대
- query: "이더리움 PoW 합의"
  expected_empty: true
  reason: WBFT 전용, PoW 없음
- query: "Casper FFG"
  expected_empty: true
  reason: Beacon chain 영역 아님
- query: "MetaMask 통합"
  expected_empty: true
  reason: client-side 코드 없음
```

---

## 7. 사용자 방향 반영 — "3-leg BM25" 임시 적용

### 7.1 구현 위치별 형태

| 위치 | 임시 BM25 형태 | 측정 대상 |
|---|---|---|
| **ckv** | `internal/query/bm25/` 신설 — chunk-aware tokenizer + ckg `pkg/bm25.Scorer` 재사용 | vector vs vector+BM25 (ckv 단독) |
| **ckg** | 이미 `pkg/bm25/scorer.go` 존재 (qname+signature+doc) | graph-only vs graph+BM25 |
| **cks** | `internal/bm25/local.go` 신설 — chunk-level RRF 시 사용 | ckv-BM25 vs cks-BM25 vs ckg-BM25 어디서 효과 큰가 |

### 7.2 측정 후 결정 (ADR-006 후보)

- 한 곳만 valuable → 나머지 2개 제거 (ADR-006 정식 결정)
- 모두 valuable → 3-leg RRF 유지 (ADR-006으로 ADR-003 supersede)
- ckv가 가장 유리 → ADR-003 supersede + ckv-only
- cks가 가장 유리 → ADR-003 유지

### 7.3 평가 fixture 권장 (N=30 한국어 query, 5개 예시)

```yaml
- query: "0번 블록에 시스템 컨트랙트 어떻게 주입돼?"
  expected_qname: ["SetupGenesisBlockWithOverride", "systemcontracts.SystemContracts"]
  expected_file: ["core/stablenet_genesis.go", "systemcontracts/systemcontracts.go"]
- query: "합의 알고리즘 라운드 체인지 동작"
  expected_qname: ["RoundChange", "roundchange.go"]
- query: "수수료 위임 트랜잭션 어떻게 검증돼?"
  expected_qname: ["FeeDelegateDynamicFeeTx", "tx_fee_delegation.go"]
- query: "거버넌스 컨트랙트 업그레이드 흐름"
  expected_qname: ["GovValidator", "GovCouncil", "Upgrades", "config_wbft.go"]
- query: "블랙리스트 계정 가스 처리"
  expected_qname: ["Blacklisted", "AccountExtra", "anzeon.go"]
```

---

## 8. 참조

- 사용자 명세 ckv 역할 (2026-05-22 대화)
- ckv `docs/evaluation-design-2026-05-22.md`
- ckv `docs/adr/003-bm25-dual-track.md` (유보 상태)
- ckg `pkg/bm25/scorer.go` (재사용 가능 Scorer 인터페이스)
- go-stablenet `.claude/docs/{CLAUDE_DEV_GUIDE,SYSTEM_CONTRACT_FLOW,BUILD_SOURCE_FILES}.md`
- Microsoft GraphRAG (https://microsoft.github.io/graphrag/)
- Gao et al. 2022 — Precise Zero-Shot Dense Retrieval without Relevance Labels (HyDE)

---

## 9. 변경 이력

| 일자 | 변경 |
|---|---|
| 2026-05-22 | 초안. R-A/B/C/D + D6 + 3-leg BM25 임시 적용 권장안 정리. |
