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

## D-1~D-5 + R1/R2 — CKS 결정 (2026-06-29 갱신, coding-agent §3-R / CKV §3-R-CKV·§5·§6 / CKG §1-R2 검토 후)

> coding-agent가 제기(§3-R)하고 CKV(§3-R-CKV)·CKG(§1-R2)가 결정한 D-1~D-5 + CKV 비전
> 가드레일 R1/R2에 대한 CKS 측 결정. CKV 문서에 `§2-R2`로 반영.

**D-1 — 재인덱싱 커밋 `0bf2f4d1b` 핀: ✅ 수용 + 독자 재빌드 안 함**
- CKS 데이터셋 빌드(`scripts/build-stablenet-dataset.sh`)는 커밋을 핀하지 않고 checkout된
  트리를 빌드한다(현 go-stablenet HEAD = `44d75d1`, 핀 대상 `0bf2f4d1b`와 다름).
- ⚠️ **숨은 충돌 해소**: "1회 재인덱싱 3자 커버"는 빌드 파라미터까지 같아야 성립한다. CKG는
  `make eval-build-dbs LANG=auto`(sol/proto 포함), CKS 스크립트는 `--lang go`라 결과 그래프가
  달라진다. → **CKS는 독자 `--lang go` 재빌드를 하지 않고, CKG가 `0bf2f4d1b`+`LANG=auto`로
  만든 canonical `graph.db`(경로/sha 공표분)를 config로 가리킨다.** CKV 인덱스도 동일 커밋분을
  가리킨다. CKS 측 비용 = config swap + 세션 재시작뿐.
- **모델 축**(CKV 제안 수용): 커밋은 `0bf2f4d1b` 고정, 모델만 2회 — reindex-A(bge-m3 baseline) /
  reindex-B(Qwen3 A/B). CKS는 각 인덱스 경로/sha로 config swap.

**D-2 — schema ≥1.19 게이트: ✅ 확인**
- CKS는 `internal/ckgclient/real.go` `FindByCanonicalID` 보유 → canonical_id join 준비 완료.
- CKG가 공표하는 manifest `schema_version`(1.22) + `graph.db` sha를 **배선 전 단언**한다(≥1.19
  아니면 join NULL 위험). cks 데이터셋 config가 가리키는 graph가 그 sha와 일치하는지 점검.

**D-3 — parity 분리: ✅ 동의 + CKS 표면 작업 소유 인정**
- ① recall/rerank류 = cks proxy 불요(cks가 단일 SemanticSearch 소비 + RRF 자체 소유, ADR-003 정합).
- ② flow/invariant = **cks `cks_context_*` 표면 신규 노출 필요(현재 미구현)**. 코드 확인: cks MCP
  표면 13종에 flow/invariant 전무. CKG가 control-flow 데이터를 제공해도, CKV가 도구를 내도,
  **cks 표면 노출이 없으면 coding-agent에 안 닿는다** → 그 표면 노출은 CKS 소관임을 인정.

**D-4 — `get_invariant_enforcement` + flow-aware 4종 cks 표면 노출: ✅ Phase 2 deliverable로 확정**
- CKS는 flow-aware 4종(`get_flow`/`expand_flow`/`find_branches`/`get_invariant_enforcement`)의
  `cks_context_*` MCP 표면 노출을 **Phase 2 deliverable로 확정**한다(post-Phase-2 defer 안 함, R2 정합).
  → coding-agent H-가드레일(코드-도출 구현 불변식) 해금.
- **조건/의존**: CKV가 밑단 도구(Phase D bounded lookup)의 **안정 인터페이스(시그니처)를 제공**해야
  실효. 인터페이스는 3자 공동설계 — 출발점 = coding-agent repo 시그니처 표 + CKS 초안(입력
  {심볼/지점, 방향 up/down, budget} → 출력 {랭크된 flow 노드, 엣지 종류, invariant 위반 후보}).
- cks 노출 형상: get_for_task 합성과 별개로 flow 도구를 직접 호출 가능한 `cks_context_*` 도구로 추가.

**D-5 — P3/R06: ⚪ CKS 무관(CKG 소관).**

**R1 (비전 가드레일 — 차원): ✅ 동의, 단 차원 실측·결정은 후순위 작업**
- CKS의 기존 "1024 유지 선호"는 연속성=편의 근거였음을 인정 → **철회**. 차원은 reindex-B에서
  1024-truncate vs full-dim **정밀도 실측** 후 "정밀도 이득 대비 비용"으로 결정한다(측정 전 1024 확정 금지).
- **단 지금은 다른 작업을 우선**한다 — 실제 차원 실측·결정은 임베딩 교체(reindex-B) 단계에서 수행.
  CKS는 dim을 ckv 인덱스에 위임(`embedder.knownDims` assert만)하므로 full-dim 채택 비용 낮음
  (knownDims 한 줄 + reindex). coding-agent도 "dim은 팩 형상 불변이면 무관" 확인.

**R2 (비전 가드레일 — parity 어려운 절반): ✅ 동의**
- flow/invariant의 cks 표면 노출은 "옵션 기능"이 아니라 북극성(모호한 묘사→정확한 위치+현상→원인
  인과) 구현 경로 → post-Phase-2 defer 금지. D-4를 Phase 2로 못 박는 것으로 이행.

**schema_version 정책: ✅ serviceable fail-loud 우선**
- coding-agent의 "호환 불가 스키마 = `ops.health.serviceable=false` fail-loud > silent fallback"에
  동의. CKS는 호환 불가 graph/모델 불일치를 조용히 degrade하지 않고 `serviceable=false`로 노출한다
  (degraded ⇒ not-serviceable, 2026-06-15 정책 유지).

## 후속 (CKS action items)

1. **D-1/D-4 데이터셋 정렬**: CKG가 `0bf2f4d1b`+`LANG=auto`로 빌드한 canonical graph.db 경로/sha를
   받으면, cks config(`cks-stablenet.yaml`)를 그 graph + 동일 커밋 ckv 인덱스로 swap → 세션 재시작.
   독자 `--lang go` 재빌드 금지.
2. **D-4 Phase 2 표면 노출**: CKV 밑단 도구 인터페이스 확정 후, flow-aware 4종 +
   `get_invariant_enforcement`를 `cks_context_*` MCP 도구로 추가(internal/mcp/). 3자 공동 시그니처.
3. **R1 차원(후순위)**: 임베딩 교체(reindex-B) 단계에서 1024-truncate vs full-dim 실측 → 차원 결정 →
   `embed_model` config + `embedder.knownDims` 갱신 + 세션 재시작. (지금은 보류, 다른 작업 우선.)
4. **B7 fixture**: cks↔ckg integration fixture 참여(canonical_id join 매칭률, ≥1.19 게이트 +
   `@<line>` 중복 케이스).
5. CKV 문서 §2의 Q2(6도구 노출)·Q4(query_code 명칭) 정정은 §2-R에 반영됨.

## §3-R-CKV-2 / coding-agent #44 대응 — 언어 스코프 + D-3 범위 (2026-06-29 갱신)

> CKV `§3-R-CKV-2`(CKS §2-R2 수용 → 5세션 수렴 + 언어 스코프 쟁점)와 coding-agent
> #44(D-1~D-5를 Phase 2 전제로 격상 + D-3 범위 확장)에 대한 CKS 대응. CKV 문서에 `§2-R3`로 반영.

**언어 스코프(proto): ✅ 동의 — CKS 런타임 영향 없음**
- CKG `LANG=auto`는 sol/proto 포함이나 CKV는 proto 미파싱(go/sol/ts/js/md) → CKG proto 노드는
  ckv 청크가 설계상 없음. **매칭률 측정은 공유언어(go/sol/ts/js)로 스코프, proto는 분모 제외**에
  동의(CKV 제안 분모 = 공유언어 CKV청크 총수 수용).
- **CKS 런타임은 영향 없음**: cks는 canonical_id join(ckg, `FindByCanonicalID`)과 semantic
  recall(ckv)을 **독립 fan-out → RRF 병합**한다. proto 심볼은 ckg graph 도구로는 잡히고 ckv
  semantic 기여만 0이 된다 → **graph-only 커버리지로 graceful, join 에러 아님**(by design).

**D-3 범위 확장: ✅ 정렬**
- coding-agent #44가 D-3(b)를 `flow·invariant·conventions`로 명시 → CKS Phase 2 표면 노출
  deliverable(D-4)을 **flow-aware 4종 + `get_invariant_enforcement` + `find_invariants` +
  `get_conventions`** 로 정렬한다(CKV가 안정 인터페이스로 제공하는 분 기준).

**수렴 상태**: CKV가 "5세션 수렴 완료" 선언. CKS 측 미해결 결정 없음 — 남은 건 *실행*(D-1/D-2
데이터셋 정렬 통지 → config swap, D-4 인터페이스 확정 후 표면 구현). Phase 2 측정은 D-1/D-2
합의 게이트를 따른다.

## Phase D flow 도구 — CKS 회신 (2026-06-30, CKV §9.1 계약 + §9.2 킥오프 대응)

> CKV가 ckg 정본 그래프(schema 1.23) + ckv 완전 인덱스(bge-m3@1024, sha `c0e448f2…`,
> 15,909청크) + flow corpus(Phase B) 완료 후, §9.1 flow 도구 계약을 제안하고 §9.2로 cks
> 표면 노출 착수를 요청. 아래는 4개 질문에 대한 CKS 회신. CKV 문서에 `§9.2-R`로 반영.
>
> **go.mod 갱신 완료**: `code-knowledge-graph` → `v0.0.0-20260630052220-3ada0adada04`,
> `code-knowledge-vector` → `v0.0.0-20260629073327-83b963fa8a18` (build·vet 클린).

**Q1 — §9.1 계약 검토: ✅ 대체로 §2-R2와 일치, 조정 2건 요청**
- 매핑 확인: CKS §2-R2 기대({지점, 방향 up/down, budget} → {랭크 flow노드, 엣지종류, invariant
  위반후보})와 §9.1이 정합 — `expand_flow{step_id,direction,hops}`=지점+방향, `find_branches`(score
  보유=랭크)=증상→원인/invariant 위반후보, `get_flow`=topological 구조, `get_invariant_enforcement`
  =H-가드레일 enabler. schema_version·citation(file/line) 표준 ✓.
- **조정①(budget 캡)**: CKS는 token-budgeted EvidencePack을 만든다. `get_flow`에 출력 상한이 없어
  큰 flow면 budget 초과 → `max_steps?`(또는 budget) 추가 요청. `expand_flow`에 `limit?`(neighbors
  상한), `get_invariant_enforcement`의 `enforced_at`에 상한 옵션. (`find_branches`는 `k` 보유 ✓.)
- **조정②(canonical_id 명시 필드)**: "symbol↔ckg 조인은 canonical_id 경유"라 했으나 step 출력 형상은
  `symbol`+`citation`만. CKS가 `FindByCanonicalID`로 ckg join 시 재해석 없이 쓰려면 **각 step/symbol
  출력에 `canonical_id` 필드 명시** 요청.
- 이 2건 반영 시 §9.1 확정 동의.

**Q2 — 표면 노출 방식: ✅ in-process `ckvclient` 4메서드 (ckv MCP proxy 아님)**
- CKS는 §2-R에서 subprocess/MCP proxy(543-LOC)를 제거하고 in-process `pkg/ckv`로 이행함 → 일관성·
  성능상 **`ckvclient.Client` 인터페이스에 4메서드 추가(`GetFlow`/`ExpandFlow`/`FindBranches`/
  `GetInvariantEnforcement`) → `pkg/ckv.Engine` 직접 호출**. ckv MCP proxy 안 함. (D-3 확장분
  `find_invariants`/`get_conventions`도 동일 in-process 방식.)

**Q3 — get_for_task 합성과 별개 직접 호출 도구: ✅ 동의**
- flow 4종(+`find_invariants`/`get_conventions`)을 `cks_context_*` 직접 호출 MCP 도구로 노출.
  coding-agent analyzer/diagnose가 root-cause-lifecycle(produce→store→consume)에서 직접 호출.
- 도구명 제안: `cks.context.get_flow` / `expand_flow` / `find_branches` /
  `get_invariant_enforcement` (+ `find_invariants` / `get_conventions`).

**Q4 — 일정/선행조건**
- **CKS 선행조건**: CKV가 §9.1(조정 반영) `pkg/ckv.Engine` 메서드를 출시 + 버전 태그. (현재 pkg/ckv에
  flow 메서드 미구현 확인 → Real 구현 본체는 그 출시 후.)
- **CKS 병렬 착수 가능(지금, CKV 출시 불필요)**:
  1. `ckvclient.Client` 인터페이스에 4(+2)메서드 시그니처 + 입출력 타입 추가, `Fake`/`Dummy` 스텁 구현
     (컴파일 가능).
  2. `internal/mcp/flow.go`에 도구 등록 + 입출력 스키마 + 핸들러 골격.
  3. 계약·표면 테스트 선작성.
- **CKV 출시 후**: go.mod bump → `ckvclient` Real 본체에 `pkg/ckv.Engine` 호출 연결 → 표면 활성.
- **Phase 2 인과 오케스트레이션**(expand_flow 다중홉 produce→store→consume 조립)은 그 다음 단계.

## CKS 작업 리스트 (Phase D, 2026-06-30 갱신)

**즉시 착수(CKV 출시 불필요)**
- T1: `ckvclient.Client` 인터페이스 확장 — `GetFlow/ExpandFlow/FindBranches/GetInvariantEnforcement`
  (+`FindInvariants/GetConventions`) 시그니처 + 입출력 타입(§9.1 + 조정①②). `Fake`/`Dummy` 스텁.
- T2: `internal/mcp/flow.go` — `cks.context.*` 도구 6종 등록 + 스키마 + 핸들러 골격.
- T3: 계약/표면 테스트(스텁 기반) 선작성.

**CKV 출시 후(gated)**
- T4: go.mod bump → `ckvclient/real.go`에 `pkg/ckv.Engine` flow 메서드 호출 연결.
- T5: 데이터셋 정렬 — `cks-stablenet.yaml`을 ckg 정본(`knowledge-data/pr-77-2`, schema 1.23) +
  ckv 인덱스(`knowledge-data/pr-77-2/ckv/vector.db`, sha `c0e448f2…`)로 swap + 세션 재시작.
- T6: end-to-end 검증(coding-agent analyzer가 직접 호출로 flow/invariant 도달).

**후속**
- T7: Phase 2 인과 체인 오케스트레이션(composer, expand_flow 다중홉).
- T8(후순위): R1 차원 실측·결정(reindex-B, Qwen3).

## Phase D 갱신 — CKV flow 출시 반영 (2026-06-30, ckv `b8e9622`)

> CKV가 28커밋으로 Phase D를 출시(`b8e9622`, MCP 15→19): `pkg/ckv.Engine`에 `GetFlow`/
> `ExpandFlow`/`FindBranches`/`GetInvariantEnforcement` 구현됨. 이에 맞춰 CKS 스캐폴딩을
> 실제 API로 정렬하고 **Real을 배선(T4 완료)**. go.mod: ckv → `v0.0.0-20260630112714-b8e9622aa4e2`,
> ckg → `v0.0.0-20260630111249-2c871a6f0158` (build·vet·test 클린).

**출시 API와 §9.1 초안 차이 (CKS 조정 ①② 미채택 → cks-side 정렬):**
- ① **budget 캡 미채택**: `ExpandFlow(stepID,direction,hops)`·`GetInvariantEnforcement(invID)`에
  limit/max 파라미터 없음 → CKS가 **fetch 후 cks-side로 캡 적용**(MaxSteps/Limit/max). 조정① 의도(토큰 budget) 보존.
- ② **canonical_id 미채택**: `FlowStepView`는 `Symbol`+`Citation`만 → cks 타입에서 CanonicalID 드롭,
  **join은 Symbol 경유**(`FindByCanonicalID`가 qname도 해소). 추후 cks가 canonical_id 보강 가능(follow-up).
- 필드 형상: `Reads/Writes/Emits`는 단일 `string`(목록 아님), `ExpandResult`는 `Direction`+`OriginBranches` 보유 → cks 타입 정렬.

**구현 상태 (실제 코드):**
- ✅ T1 `ckvclient.FlowClient`(인터페이스+cks 타입+Fake/Dummy) — `internal/ckvclient/flow.go`.
- ✅ T2 MCP 도구 4종 등록 — `internal/mcp/flow.go` (`get_flow`/`expand_flow`/`find_branches`/
  `get_invariant_enforcement`), `server.go` 배선, 골든 픽스처 갱신(13→17 cks.* 도구).
- ✅ T3 테스트 — `internal/ckvclient/flow_test.go`, `internal/mcp/flow_test.go` (Fake 해피패스·캡·
  미지원 백엔드 폴백). 전체 test/vet 클린.
- ✅ **T4 Real 배선** — `Real.{GetFlow,ExpandFlow,FindBranches,GetInvariantEnforcement}`가
  `r.eng.*` 호출 + ckv타입→cks타입 변환 + cks-side 캡. (백엔드 누출 방지, SemanticSearch와 동일 패턴.)
- ⚠️ **정정**: 위 T1 목록의 `(+FindInvariants/GetConventions)` 2종은 T2의 flow-4종과 **동시 출시가
  아니었다** — 당시 ckv `pkg/ckv`에 미노출(gated)이라 flow-4종만 랜딩(골든 13→17). 두 지식조회
  도구는 **2026-07-12에 별도 출시**(M5): ckv facade PR #35(`pkg/ckv`에 `FindInvariants`/
  `GetConventions` 노출) + cks PR #34(`FlowClient` 배선 + `cks.context.find_invariants`/
  `get_conventions` 등록, 골든 17→19). 이제 T1의 6종이 모두 cks 표면에 노출됨.

**완료 (2026-07-01, 라이브 검증):**
- ✅ T5 데이터셋 정렬 — `cks-stablenet.yaml`(gitignored)의 ckg/ckv 경로를 `knowledge-data/pr-77-2`
  (graph.db schema 1.23 + ckv/vector.db, indexed_head `0bf2f4d1b`)로 swap. ⚠️ 실행 중 cks-mcp는
  **세션 재시작 필요**(`/reload-plugins` 불가 — 메모리 cc-reload-plugins-no-mcp-restart).
- ✅ T6 end-to-end — 두 경로로 검증:
  - `ckvclient.Real` 직접(env-gated 통합테스트 `flow_live_test.go`, ollama bge-m3@1024):
    GetFlow `ep-cli-init`=5 steps(root main.initGenesis@chaincmd.go:191), GetInvariantEnforcement
    `INV-CONSENSUS-01`=4 sites(commit.go:123 등), ExpandFlow=1 neighbor+2 branches, FindBranches
    "정족수 부족"=9 matches. **CKV §9-R 라이브 케이스와 일치.**
  - 실 `cks-mcp` 서버(stdio): tools/list=17 cks 도구(flow 4종 광고), tools/call get_flow
    (max_steps 캡 적용)·get_invariant_enforcement(4 sites) 정상.

**후속:** T7 인과 오케스트레이션(composer 다중홉) · T8(후순위) R1 차원 실측.

**잔여 계약 회신(CKV에):** 조정 ①(budget 캡)·②(canonical_id)는 cks-side로 흡수했으므로 CKV 필수
변경 아님. 다만 ② canonical_id를 step에 실으면 cks가 ckg join 시 재해석을 줄일 수 있어 **선택적 개선**으로
재요청(미차단). find_invariants/get_conventions(D-3 확장)는 별도 계약 확정 후 동일 방식 노출 예정.
