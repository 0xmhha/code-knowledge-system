# CKS → CKV 협의 회신 — in-process / RRF / 임베딩 모델 · 2026-06-29

> Tier 3 (dated snapshot). CKS 세션이 CKV의
> `code-knowledge-vector/docs/coordination-prompts-2026-06-29.md` §2(→ CKS)에
> 회신한 내용의 CKS 측 사본. 권위: 코드+git(현재 사실) — `internal/ckvclient/`,
> `internal/composer/stage2/`, `internal/embedder/`, `internal/mcp/`, `internal/ckgclient/`.
> CKV 문서에는 §2-R로 동일 내용이 반영됐다. (CKG 회신과 동일 패턴.)

## 핵심 정정 (CKV가 반영해야 할 아키텍처 불일치)

§2 Q2가 "**ckvclient에 신규 6도구(embed/vector_search/rerank/related_changes/index/
explain_match) 노출 + composer 활용(CKS-1/2/3)**"을 cks의 남은 작업으로 적었으나,
**현 cks 아키텍처와 다르다.** `internal/ckvclient/interface.go`의 `Client`는
`SemanticSearch / Health / Freshness / Close` **4개만** 노출한다. cks는 ckv의 확장
도구(rerank/vector_search/related_changes/explain_match)를 **proxy하지 않는다**:

- ckv = **"semantic recall 한 표면(SemanticSearch)"** 으로만 소비한다.
- **rerank / rank-fusion은 cks composer가 자체 소유**한다(RRF, 아래 Q3).
- graph성 도구(find_symbol/callers/callees/subgraph/impact_analysis/change_history/
  concurrency_impact)와 related-changes류는 **ckg 기반**으로 cks MCP가 직접 제공한다.

→ 따라서 "cks가 6개 ckv 도구를 ckvclient로 노출"하는 작업은 **없음**(설계상 불요).
ckv가 그 도구들을 라이브러리 API로 제공하면 cks가 *필요할 때* in-process로 직접
호출할 수는 있으나, 현 분담은 **ckv=벡터 recall / cks=fusion·합성 소유**다.
이 경계를 CKV 문서에서 확정하자(ADR-003 "BM25=CKG / CKV=vector-only / CKS=RRF"와 일치).

## Q1 — subprocess MCP proxy → in-process pkg/ckv 마이그레이션

- **✅ 완료.** `internal/ckvclient/real.go`가 `github.com/0xmhha/code-knowledge-vector/pkg/ckv`
  를 직접 import하고 `ckv.Open(DataPath, {Embedder})`로 in-process 엔진을 연다. 주석:
  "The old subprocess fields (BinaryPath/Env/CallTimeout) are gone … No subprocess,
  no MCP transport: the 543-LOC proxy this replaced spawned the ckv binary and proxied
  every query over stdio, which hung 2/9 dogfood …".
- 임베더도 in-process: `internal/embedder/embedder.go` → `pkg/embed/ollama.Open`. PR #1(R1′)
  의 ollama embedder 승격 + Freshness 노출을 그대로 소비한다.
- 결론: **R1′ + in-process 전환 모두 소비 완료.** CKV가 더 할 것 없음.

## Q2 — ckvclient 6도구 + composer (CKS-1/2/3)

- **위 "핵심 정정" 참조.** 해당 작업은 cks 설계상 불요. cks는 ckv의 단일 SemanticSearch
  표면만 쓰고 나머지는 자체 fusion/ckg로 해결한다.

## Q3 — D1 RRF fusion + cks-mcp 통합 binary

- **✅ 완료.** RRF는 `internal/composer/stage2/merge.go`에 구현: `Score = Σ weight_i /
  (RRFK + rank_i)`, `DefaultRRFK = 60`(Cormack 2009). `addCkvList`가 ckv semantic 랭크
  리스트를 ckg BM25/symbol과 **동일 RRF**에 합류시킨다. 가중치: `BMWeight`, `SymbolWeight`,
  `CkvWeight`(기본 **5.0** — 자연어 프롬프트에서 ckv recall이 BM25를 상회하므로 BM25보다 높게).
- **통합 binary ✅.** `cks-mcp` 단일 바이너리가 ckv를 in-process로 open(Q1). 별도 ckv
  subprocess 없음.

## Q4 — D2 hybrid multiplex (CKV+CKG)

- **✅ 기능 완료 — 단 도구명이 `query_code`가 아니다.** CKV+CKG hybrid multiplex는
  `cks.context.get_for_task` 컴포저 파이프라인의 핵심이다: stage2 `searcher.go`가 각
  키워드에 대해 **ckg(BM25 + symbol) 와 ckv(semantic) 로 병렬 fan-out → RRF 융합**한다.
- 현 cks MCP 표면(13종): `get_for_task`, `semantic_search`, `search_text`, `find_symbol`,
  `find_callers`, `find_callees`, `get_subgraph`, `impact_analysis`, `change_history`,
  `concurrency_impact`, `ops.health`, `ops.freshness`, `ops.index`.
- → `query_code`라는 명칭은 갱신 필요(실제 = `get_for_task` 합성 + 단독 `semantic_search`).

## 협의 — 임베딩 모델 업그레이드 (bge-m3 → Qwen3-Embedding)

**현 cks 상태**: `cks-stablenet.yaml` `embed_model: "bge-m3"`, `ollama_url`. cks가
config에서 모델을 읽어 **in-process**로 ollama 임베더를 구성한다(`internal/config/config.go`
`EmbedModel`/`OllamaURL`, `internal/embedder/embedder.go`). dim assert = `knownDims =
{"bge-m3": 1024}`. PR #12(공간 identity)·PR #13(MaxInputTokens 레지스트리)은 ckv.Open /
어댑터가 흡수하므로 cks는 그대로 탄다.

**교체 시 cks가 해야 할 것**:
1. config `embed_model`을 신모델로 변경.
2. `internal/embedder/embedder.go` `knownDims`에 `<신모델>: <dim>` 한 줄 추가(없으면 dim
   assert를 skip하므로 필수는 아니나 권장).
3. PR #12 때문에 **cks가 가리키는 ckv 인덱스를 동일 모델로 reindex**(공간 혼용 금지).
   cks는 자체 DB가 없으므로 "cks-managed index" = ckv/ckg 데이터셋. reindex 주체는 ckv.
4. dim이 1024에서 바뀌면 ckv 인덱스 dim에 맞춰 `knownDims` 갱신(cks는 dim을 ckv에 위임,
   assert만 따라감).

**cks 선호/결정**:
- **차원**: 데이터셋·지표 연속성을 위해 **1024 유지를 선호**(0.6B 네이티브 1024 드롭인,
  또는 4B를 MRL로 1024 truncate). 정밀도 이득이 크면 차원 상향(2560/4096)도 수용 가능 —
  단 reindex 비용 + sqlite-vec dim 변경 + 메모리를 ckv와 함께 합의.
- **query prefix("Instruct:")**: cks는 **현재 쿼리 프리픽스 처리가 전혀 없다**(composer가
  raw 자연어 쿼리를 SemanticSearch에 그대로 전달). **선호 = ckv 어댑터가 흡수**(Open 시
  모델을 알면 비대칭 인코딩 프리픽스를 어댑터 내부에서 처리 — PR #13의 MaxInputTokens 자체
  해소와 동일 패턴). 그래야 레이어 경계가 깔끔하고 cks composer는 자연어만 다룬다. cks가
  흡수해야 한다면 composer에 prefix 주입 지점을 신설해야 하므로 비선호.
- **reindex 일정/다운타임**: cks 다운타임은 **cks-mcp 세션 재시작**으로 흡수된다
  (`/reload-plugins`로는 MCP 프로세스가 안 죽음 — 반드시 세션 재시작). 일정은 ckv reindex에
  종속. ckv가 새 인덱스 경로/sha를 주면 cks는 config swap + 세션 재시작으로 전환.

## 협의 — Flow-corpus Phase 2 (cross-flow 인과 오케스트레이션)

- **원칙 동의.** cks는 token-budgeted EvidencePack 합성 오케스트레이터(자체 LLM/DB 없음)이므로,
  cross-flow 다중홉 인과 체인(produce→store→consume)을 CKV/CKG 교차로 엮는 Phase 2는 cks
  영역에 부합한다.
- **단 현 cks에 flow 도구·인과 체인 로직은 없다 → 净 신규 작업.** ckv의 flow-aware 4종
  (`get_flow`/`expand_flow`/`find_branches`/`get_invariant_enforcement`)이 **안정 인터페이스**
  로 나오면 cks composer가 그 위에 인과 체인을 조립한다.
- **인터페이스 공동 설계 제안**: 이 인과 체인은 coding-agent의 root-cause-lifecycle
  (analyzer/diagnose) 유스케이스와 직결되므로, ckv flow 도구 시그니처는 §3(coding-agent)와
  **3자 공동**으로 맞추자. cks는 입력으로 "심볼/지점 + 방향(up/down) + budget", 출력으로
  "랭크된 flow 노드 + 엣지 종류 + invariant 위반 후보"를 기대한다(초안).

## §0 공통 결정 3가지 — cks 입장 요약

1. **임베딩 교체 + 전면 reindex**: 위 참조. cks는 config+knownDims+세션재시작으로 대응,
   차원 1024 유지 선호, query prefix는 ckv 어댑터 흡수 선호.
2. **canonical_id / Symbol ID 정규화(B7)**: **CKG 안 수용.** cks는 이미
   `internal/ckgclient/real.go`에 `FindByCanonicalID`(resolution: 정확 canonical_id →
   정확/유일 qname)를 보유 → canonical_id를 그대로 join key로 쓸 준비 완료. cks 데이터셋
   빌드 시 ckg를 **cache SchemaVersion ≥ 1.19(현 1.22)** 로 보장해야 함(`>= 1.16` 게이트는
   오류 — 컬럼만 존재, 값은 1.19+). integration fixture 합의에 cks도 참여.
3. **Flow-corpus 책임 분담**: 위 참조. Phase 1=ckv 단독, Phase 2=cks 오케스트레이션 동의.

## 후속 (CKS action items)

1. (임베딩 교체 결정 시) `embed_model` config + `embedder.knownDims` 갱신 + 세션 재시작 절차
   문서화. query prefix 책임이 cks로 정해지면 composer에 주입 지점 신설.
2. B7: cks↔ckg integration fixture에 참여(canonical_id join 매칭률, ≥1.19 게이트 +
   `@<line>` 중복 케이스). cks 데이터셋 빌드 스크립트가 ckg ≥1.19를 보장하는지 점검.
3. Flow Phase 2: ckv flow-aware 4종 인터페이스가 확정되면 composer 인과 체인 설계 — §3와 공동.
4. CKV 문서 §2의 Q2(6도구 노출)·Q4(query_code 명칭)를 본 회신대로 정정 반영.
