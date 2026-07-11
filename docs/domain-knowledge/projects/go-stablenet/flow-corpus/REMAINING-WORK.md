# 남은 작업 — go-stablenet 지식 코퍼스 & cks 검색 알고리즘 연구

> **이 문서의 목적**: 작업을 중단했다가 (다른 세션/시점에) 재개할 때, "우리가 무엇을 왜 하고
> 있었고, 무엇이 끝났고, 무엇을 해야 하는가"를 단독으로 이해할 수 있게 정리한 핸드오프 문서.
> 같은 디렉터리의 `corpus.jsonl`·`flows/`·`invariants.md`·`memory.md`와 함께 본다.

---

## 0. 한 줄 요약

go-stablenet 코드를 **entry-point 기준 동작 코퍼스**로 만들고(완료), 그 지식을 활용/검증하기 위해
**cks의 검색 알고리즘(ckv→ckg)을 두 방식으로 비교하는 실험**을 진행 중이다. 비교를 공정하게 하려고
**RetrievalTrace**(공용 JSON 기록)를 cks에 추가했고(완료, 미push), 이제 실험을 실제로 돌리는 일이 남았다.

---

## 1. 전체 맥락 — 왜 이 작업을 하나

**문제의식.** 사람이 작업을 요청할 때는 암묵지가 밴 용어("수수료 위임 검증 어디?")를 쓴다. 이 말을
그대로 **ckg**(그래프 + BM25/키워드)에 던지면 거의 못 찾는다 — ckg는 코드에 실제 등장하는 토큰/심볼에만
걸리기 때문(**lexical gap**). 그래서 **ckv**(벡터/의미)로 먼저 의미를 잡아 ckg가 매칭할 키워드·심볼로
번역한 뒤 ckg를 쿼리해야 정확한 코드를 그래프로 받을 수 있다. → 이것이 cks의 **순차 ckv→ckg funnel**.
(cks 구조 상세는 `memory.md` 참조.)

**연구 질문.** 이 ckv→ckg 검색을 어떤 알고리즘으로 돌릴 때 가장 정밀한가?
- **방식 A (LLM-RAG, 반복)**: coding-agent의 planner(LLM)가 `semantic_search → find_symbol →
  get_subgraph`를 hop마다 *판단하며* 호출. ckg-bench의 **M3**에 해당.
- **방식 B (결정론 Composer, LLM 없음)**: cks 내부 Composer가 ckv recall → 키워드 추출 → ckg
  BM25 rerank를 *고정 규칙*으로 수행. ckg-bench의 **M4**(`get_for_task`)에 해당.
- 둘 다 이미 존재한다. 핵심은 **공정 비교**: 같은 프롬프트에 대해 두 방식이 어떤 코드 경로를
  거쳐 어떤 seed에 도달하는지를 **같은 포맷으로** 기록해 정확도·비용을 나란히 채점하는 것.

**그 공통 포맷이 `RetrievalTrace`** — producer-agnostic JSON. B(Composer)는 이미 이걸 내보내고,
A(LLM)도 같은 shape로 내보내게 만들면 비교가 성립한다.

---

## 2. 지금까지 한 일 (완료)

### 2.1 지식 코퍼스 (이 디렉터리)
- **flows/ 18개** — entry-point에서 출발해 실제 코드 흐름을 문장으로 푼 것. 외부 요청 9
  (main, cli-init, rpc-sendrawtx, p2p-eth, p2p-consensus, graphql, p2p-snap, discovery, keystore) +
  척추 4(statetransition, finalize, consensus-rounds, blockimport) + 내부 루프 5(loop-consensus,
  loop-miner, loop-txpool, loop-broadcast, loop-chainsync). **각 step에 `branches`(조건별 분기·실패)**
  포함 — happy path만이 아니라 "언제 어떻게 실패/분기하는가"를 담음.
- **invariants.md** — INV 16개(CONSENSUS/LEDGER/STATE/TX/SEC/GENESIS/VALIDATOR), flow step과 양방향 링크.
- **corpus.jsonl** — 위를 기계 적재용으로 변환한 255 레코드(flow 18 / step 79 / invariant 16 / edge 142).
  스키마는 `SCHEMA.md`(vector=prose, graph=symbol+edges, 벤치=정답).
- **tools/build_corpus.py**(생성)·**tools/check_corpus.py**(구조 검사) — 재현 가능.
- **AUDIT.md** — 정합성 감사: 구조 결함 4 + 파서 취약점 2 발견·수정, load-bearing 코드 주장 25개를
  실제 소스와 대조(MISMATCH 0).
- 용도 3가지: ① cks 적재, ② bench ground-truth, ③ 사람용 설계 문서.
- ⚠️ 이 디렉터리는 **스냅샷 복사본**이다. 라이브 원본은 `~/Work/github/go-stablenet/.claude/docs/corpus/`
  (그 리포에서 `.claude/`는 미추적). 원본을 갱신하면 여기로 다시 복사 필요.

### 2.2 RetrievalTrace (cks 리포 — 사용자가 cks에서 처리)
- 위치: `~/Work/github/code-knowledge-system`, 브랜치 **`feature/retrieval-trace`**, 커밋 2개,
  **아직 push/PR 안 함**.
- `pkg/contract/retrievaltrace.go` — `RetrievalTrace`/`RetrievalStep`/`RetrievalStepKind`(enum:
  `ckv.recall`, `ckg.bm25`, `ckg.find_symbol`, `ckg.subgraph`, `ckg.impact`) + `IsValid`. `Producer`
  필드로 A(`agent`)/B(`composer`) 구분. `Steps`는 시퀀스라 A의 반복 tool-call도 B의 ckv 라운드도 같은 shape.
- `Composer.Compose` → 신규 `ComposeTraced(ctx, prompt) (EvidencePack, RetrievalTrace, error)`에
  위임(기존 호출자 무영향). trace는 stage1(ckv)+stage2(ckg) provenance로 빌드.
- 테스트 통과·전체 빌드 OK. **알려진 한계**: `CKVCalls`는 정확(라운드당 1), **`CKGCalls`는 미계측(0)** —
  정확 카운트는 클라이언트 계측 필요(Track A4).

---

## 3. 현재 상태 — 대기 중 ⏸

다른 세션에서 **ckv가 일부 한글 용어의 의미를 잘못 추출 → ckg에 엉뚱한 seed로 쿼리**하는 버그를
수정 중. **그 수정이 반영된 뒤 다시 검토**하기로 하고 멈춤. (이 버그는 §1의 lexical gap의 실제
발현이고, 마침 `RetrievalTrace`의 `failed_keywords`/`steps[].keywords`/`final_seeds`가 수정 전후
효과를 정량 확인하는 도구가 된다.)

> **재개 트리거**: ckv 수정 반영 알림 → 변경 검토 → 아래 Track A로.

---

## 4. Track A — 검색 알고리즘 실험 (주 목표, 남은 작업)

> 목표: 방식 A(LLM-RAG) vs 방식 B(결정론 Composer) 중 우수 알고리즘 규명 + 4-way 비교 레포트.

| # | 작업 | 맥락 / 왜 필요한가 | 위치 |
|---|------|--------------------|------|
| **A1** | **실 cks 백엔드로 `ComposeTraced` sanity-run** | fake가 아닌 진짜 ckv/ckg로 trace가 의미있게 나오는지 검증. 싸고, A2·A3 투자 전에 토대 리스크 제거. **추천 첫 작업.** | cks |
| **A2** | **방식 A의 trace 생성** | planner(LLM)가 호출하는 cks tool-call들을 같은 `RetrievalTrace`(Producer=`agent`)로 기록. **없으면 A vs B 비교 자체가 불가** (임계 경로). | coding-agent 플러그인 |
| **A3** | **ckg-bench 연결** | 골든셋 30문항으로 M3(A) vs M4(B) 실행, 양쪽 trace 수집, 채점(위치정확도·정답률·hallucination·정보량 + seed품질·step수·라운드·호출수). (임계 경로). | `go-stablenet/.coding-agent/bench/ckg-bench` |
| **A4** | **`CKGCalls` 정확 계측** | `ckgclient`/`ckvclient`에 호출 카운터 추가 → trace의 비용 지표 완성. *비용 결론* 전 필요. 단 *구조 비교*(seed/step/round)는 그 전에도 가능. | cks |
| **A5** | (선택) **MCP 표면에 trace 노출** | bench가 in-process가 아니라 **MCP로** 소비할 때만 필요. `get_for_task`가 EvidencePack과 함께 trace 반환. | cks `cmd/cks-mcp` |
| **A6** | **분석·레포트** | A vs B 우열 + 원 티켓의 4-way(M1 파일원문 / M2 그래프전체 / M3 LLM점진 / M4 자동선별) 비교 분석 산출. | bench |

**Track A 순서 권장**: A1 → (A2 + A3 묶어 "한 프롬프트로 A·B trace 나란히" 최소 실험) → A4(비용) → A6(레포트). A5는 필요 시.

---

## 5. Track B — 코퍼스 → cks 적재 (보류)

> 목표: §2.1 코퍼스를 cks의 ckv/ckg에 실제로 넣어 검색 품질을 높이거나, 실험의 정답셋으로 쓴다.

| # | 작업 | 맥락 / 왜 필요한가 | 비고 |
|---|------|--------------------|------|
| **B0** | **cks 적재 인터페이스 선결 조사** | cks(ckv/ckg)가 *외부 문서/주석*을 적재하는 API가 있는지, 아니면 *코드에서만* 그래프를 빌드하는지 확인. **B1 가능 여부를 결정.** 아직 확인 안 함. | cks 리포 조사 |
| **B1** | **`corpus.jsonl` 적재** | prose(step.prose, invariant.statement) → **ckv**(의미 검색 문서), edges/symbol(calls/enforces) → **ckg**(노드 주석). cks가 코드에서 못 뽑는 레이어(entry-point 그룹핑·분기/실패 조건·불변식 링크)를 덧입힘. | B0 결과에 의존 |
| **B2** | **코퍼스를 bench 골든셋/ground-truth로** | 코퍼스의 `enforces`/`final_seeds`/`failed_keywords`가 "이 질문의 정답 seed/실패 조건"을 제공 → Track A의 채점 기준. | Track A6와 합류 |

---

## 6. 두 트랙의 합류점

별개가 아니다. **Track B의 코퍼스(불변식·flow·실패 조건)는 Track A 실험의 평가 기준/정답셋**으로
쓰인다. 예: "수수료 위임 검증은 어디?"의 정답 seed = `corpus.jsonl`의 해당 INV `enforces` 또는
flow `final_seeds`. 즉 코퍼스는 실험의 채점자(A6/B2)에서 만난다.

---

## 7. 최종 단계 (사용자 지시 대기)

전 작업 완료 후, **사용자 지시 기반으로** 최종 정리:
- cks 브랜치 **PR** (`feature/retrieval-trace`, github.com/0xmhha/code-knowledge-system, gh 인증됨).
- 코드/문서 **정리**.
> 그때까지 push·PR·대규모 정리는 하지 않는다. cks 작업은 cks 리포에서 사용자가 직접 처리.

---

## 8. 리포/위치 지도

| 무엇 | 위치 | 상태 |
|------|------|------|
| 지식 코퍼스 (스냅샷) | `~/Work/github/study/docs/research/ai-knowledge-data/corpus/` (이 폴더) | 복사본 |
| 지식 코퍼스 (라이브 원본) | `~/Work/github/go-stablenet/.claude/docs/corpus/` | 미추적 |
| 분석 대상 코드 | `~/Work/github/go-stablenet/` (go-ethereum 포크 + WBFT) | — |
| RetrievalTrace 코드 | `~/Work/github/code-knowledge-system/` 브랜치 `feature/retrieval-trace` | 커밋 2, 미push |
| cks 본체 (ckv/ckg/composer) | `~/Work/github/code-knowledge-system/` | — |
| bench 하니스 (M1~M4) | `~/Work/github/go-stablenet/.coding-agent/bench/ckg-bench/` | — |
| coding-agent planner (방식 A) | `~/.claude/plugins/cache/coding-agent/...` | — |

---

## 9. 재개 방법 (체크리스트)

1. **ckv 한글 수정**이 반영됐는지 확인 → 변경 검토.
2. `memory.md`로 cks 구조·커밋 규칙 복기.
3. **A1**(실 cks `ComposeTraced` sanity-run)부터 — trace가 멀쩡한지.
4. 깨끗하면 **A2+A3**(방식 A trace + bench 연결)로 최소 비교 실험.
5. 구조 비교 검증되면 **A4**(CKGCalls 계측) → **A6**(레포트). 병행해 **B0**(적재 인터페이스 조사).
6. 모두 끝나면 **최종 단계**(PR·정리)를 사용자 지시로.
