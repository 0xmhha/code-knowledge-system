# go-stablenet 동작 코퍼스 (entry-point flow corpus)

> **이 코퍼스가 무엇이고 왜 만드는가**
>
> go-stablenet이 실행 중일 때, 이 프로그램의 어떤 기능을 쓰든 그 사용은 반드시 **하나의 entry
> point에서 시작**된다. 그 시작점에서 코드를 walk하면 "이 요청이 실제로 어떻게 수행되는가"를
> 순서가 있는 문장으로 적을 수 있다. 이 코퍼스는 **모든 entry point를 그렇게 walk해, 한 단계
> (step)씩 구조화된 단위로 적어 둔 것**이다.
>
> 세 가지 용도를 동시에 만족하도록 설계했다:
> 1. **cks 적재용** — 각 step이 심볼(`pkg.Func @ 파일:라인`)에 묶이고 호출/상태 엣지를 가지므로,
>    vector DB(의미 검색)와 graph DB(영향 분석)에 그대로 넣을 수 있다.
> 2. **벤치마크 정답용** — "이 작업은 이 step들을 거친다 / 이 심볼·상태에 영향을 준다"가 명시돼
>    있어, cks의 검색·영향 분석 결과를 채점할 ground truth가 된다.
> 3. **사람이 읽는 설계 문서** — 각 step의 `prose` 필드만 이으면 그대로 읽히는 서사가 된다.

## 왜 5개 에세이로는 안 됐는가

`make all`은 160개 패키지·781개 파일을 빌드한다. 이전에 만든 `walkthrough/` 에세이 5~6개는
**주요 척추만 추려 사람이 읽기 좋게 쓴 샘플**이었지, 모든 경로를 담은 코퍼스가 아니었다. 위 세
용도에는 부족하다. 이 `corpus/`가 그 자리를 대신한다. (단위가 곧 prose이므로 사람용도 겸한다.)

## 단위(unit) 형식 — flow step

코퍼스의 최소 단위는 **flow step**이다. 한 entry point의 한 실행 단계를 가리킨다.
`flows/*.md`의 각 step은 아래 필드를 갖는다(기계 파싱 가능 + 사람이 읽힘):

| 필드 | 의미 | DB 용도 |
|------|------|---------|
| `id` | 안정적 식별자 `<flow>-NN` | 노드 키, 벤치 참조 |
| `symbol` | `pkg.(Recv).Func` 정규명 | graph 노드 ↔ cks 심볼 매핑 |
| `at` | `파일:라인` | 코드 위치 |
| `kind` | `geth` \| `stablenet` | StableNet 고유 식별 |
| `calls` | 다음 step id(들) / 호출 심볼 | graph 호출 엣지 |
| `reads` / `writes` | 읽고/쓰는 상태(잔액·슬롯·DB·채널) | graph 데이터 엣지, 영향 분석 |
| `emits` | 발생 이벤트/메시지 | graph 이벤트 엣지 |
| `branches` | **조건별 분기·실패** `{when, then, at}` 목록 | 실패 조건 파악, 영향 분석, 음성(negative) 벤치 |
| `invariant` | 관련 `INV-*` (있으면) | 안전성 링크 |
| `prose` | 동작 서술(happy path **+ 주요 분기/실패 조건**) | vector 임베딩 청크 + 사람용 |

> **`branches`가 핵심이다.** happy path만 적으면 "언제·어떻게 실패하거나 다른 경로를 타는가"를
> 알 수 없어, 영향 분석과 실패 조건 파악이 불가능하다. 각 step에서 검사하는 조건(`when`)과 그
> 결과(`then` — 어떤 에러로 거부 / 어떤 대체 step으로 분기), 그리고 위치(`at`)를 모두 적는다.
> `prose`도 happy path만이 아니라 이 분기/실패를 문장에 엮는다. (분기가 없는 step은 `branches: []`.)

> 이 markdown 형식은 JSONL로 1:1 변환 가능하다(각 step → 한 객체). cks 적재 시 실제 스키마
> 매핑은 별도 단계이며, 핵심은 **모든 step이 `symbol`로 코드에 고정**되어 있다는 점이다 — cks가
> 이미 인덱싱한 심볼·호출그래프에 prose/엣지/불변식 레이어를 덧입히는 구조다.

## entry point 카탈로그 (완전 목록)

> 외부 요청뿐 아니라 **타이머·이벤트로 스스로 도는 내부 루프**도 독립적 시작점이므로 포함한다.
> (이 내부 루프들이 빠지면 합의 타임아웃·동기화·마이닝 같은 백그라운드 동작이 누락된다.)

### 외부 요청 시작점
| id | 시작점 | 루트 심볼 / 위치 | flow 파일 |
|----|--------|------------------|-----------|
| `EP-MAIN` | 프로세스 시작 | `main()` `cmd/gstable/main.go:271` | `flows/ep-main.md` |
| `EP-CLI-INIT` | `gstable init` | `initGenesis` `cmd/gstable/chaincmd.go` | `flows/ep-cli-init.md` |
| `EP-CLI-*` | account/console/db/snapshot 등 | `cmd/gstable/*cmd.go` | `flows/ep-cli-*.md` |
| `EP-RPC-HTTP` | JSON-RPC over HTTP | `httpServer.ServeHTTP` `node/rpcstack.go:198` | `flows/ep-rpc-*.md` |
| `EP-RPC-WS` | JSON-RPC over WebSocket | 동 `rpcstack.go` (ws 분기) | (RPC와 공유) |
| `EP-RPC-IPC` | JSON-RPC over IPC | `ipcServer.start` `rpcstack.go:595` | (RPC와 공유) |
| `EP-GRAPHQL` | `/graphql` | `handler.ServeHTTP` `graphql/service.go:35` | `flows/ep-graphql.md` |
| `EP-P2P-ETH` | eth 와이어 메시지(코드별) | `handleMessage` `eth/protocols/eth/handler.go:186` | `flows/ep-p2p-eth-*.md` |
| `EP-P2P-CONSENSUS` | WBFT 합의 메시지(타입별) | `handler_istanbul.go:132` → `backend/handler.go:70` | `flows/ep-p2p-consensus.md` |
| `EP-P2P-SNAP` | snap 동기화 메시지 | `eth/protocols/snap/handler.go` | `flows/ep-p2p-snap.md` |
| `EP-P2P-DISCOVERY` | discv4/v5 | `p2p/discover/*` | `flows/ep-discovery.md` |
| `EP-FILE-CONFIG` | TOML 설정 로드 | `loadConfig` `cmd/gstable/config.go:101` | `flows/ep-main.md`에 포함 |
| `EP-FILE-GENESIS` | 제네시스 JSON 로드 | `initGenesis` (EP-CLI-INIT와 공유) | `flows/ep-cli-init.md` |
| `EP-FILE-KEYSTORE` | 키스토어/키 로드 | `accounts/keystore` | `flows/ep-keystore.md` |
| `EP-FILE-CHAINDATA` | chaindata DB 오픈 | `OpenDatabaseWithFreezer` `eth/backend.go:134` | `flows/ep-main.md`에 포함 |

### 내부 루프(타이머·이벤트) 시작점
| id | 시작점 | 루트 심볼 / 위치 | flow 파일 |
|----|--------|------------------|-----------|
| `EP-LOOP-CONSENSUS` | 합의 이벤트 루프 | `Core.handleEvents` `consensus/wbft/core/handler.go:102` | `flows/ep-loop-consensus.md` |
| `EP-LOOP-CONSENSUS-TIMEOUT` | 라운드 체인지 타임아웃 | `handleTimeoutMsg` `core/handler.go:159` | `flows/ep-loop-consensus.md` |
| `EP-LOOP-MINER` | 마이너 워크 루프 | `newWorkLoopWBFT` `miner/worker.go:608` | `flows/ep-loop-miner.md` |
| `EP-LOOP-CHAINSYNC` | 체인 동기화 루프 | `chainSync.loop` `eth/handler.go:575` | `flows/ep-loop-chainsync.md` |
| `EP-LOOP-TXPOOL` | txpool reorg/eviction | `scheduleReorgLoop` `core/txpool/legacypool/legacypool.go` | `flows/ep-loop-txpool.md` |
| `EP-LOOP-BROADCAST` | tx/mined 블록 브로드캐스트 | `txBroadcastLoop`/`minedBroadcastLoop` `eth/handler.go:566/571` | `flows/ep-loop-broadcast.md` |

## 척추(spine) — flow들이 수렴하는 공통 경로

여러 entry point가 같은 하위 경로로 합쳐진다. 중복 서술을 막기 위해 공통 경로는 별도 flow로 두고
`calls`로 링크한다:
- `flows/spine-statetransition.md` — `state_transition.go` 상태 전이 5단계(잔액 이동).
- `flows/spine-finalize.md` — `processFinalize` / `distributeBaseFee`.
- `flows/spine-consensus-rounds.md` — PRE-PREPARE→PREPARE→COMMIT 정족수 확정.
- `flows/spine-blockimport.md` — `InsertChain` 검증·재실행·기록.

## 불변식 레이어

각 step의 `invariant` 필드가 `invariants.md`(추출된 `INV-*`)를 가리킨다. 이 레이어가 "이 코드는
어떤 성질을 지키는가"를 graph에 연결해, 영향 분석 시 "이걸 바꾸면 어떤 불변식이 위험한가"를
끌어낼 수 있게 한다.

## 생산 계획 / 진행 상태

전 entry point를 위 형식으로 채우는 것은 배치 작업이다. 각 flow는 (1) 코드 walk → (2) step
구조화 → (3) prose 작성 순으로 만든다.

- [x] README (형식·카탈로그·계획)
- [x] **견본** `flows/ep-rpc-sendrawtx.md` — 형식 확정용 앵커 (RPC `eth_sendRawTransaction`, 분기/실패 포함)
- [x] 척추 flow `flows/spine-statetransition.md` (상태 전이 5단계)
- [x] 척추 flow `flows/spine-finalize.md` (base fee 재분배·에폭·상태루트)
- [x] 척추 flow `flows/spine-consensus-rounds.md` (PRE-PREPARE→PREPARE→COMMIT + 타임아웃)
- [x] 척추 flow `flows/spine-blockimport.md` (헤더검증→본문→재실행→상태검증→기록)
- [x] 외부 요청 flow `flows/ep-main.md` (기동 + config/chaindata 로드)
- [x] 외부 요청 flow `flows/ep-cli-init.md` (제네시스 파일 로드 + 시스템 컨트랙트 주입)
- [x] 외부 요청 flow `flows/ep-p2p-eth.md` (eth 와이어 메시지 → txpool/import)
- [x] 외부 요청 flow `flows/ep-p2p-consensus.md` (WBFT 합의 메시지 → 코어)
- [x] 외부 요청 flow `flows/ep-graphql.md` (읽기 질의)
- [x] 내부 루프 flow `flows/ep-loop-consensus.md` (합의 이벤트·타임아웃 루프)
- [x] 내부 루프 flow `flows/ep-loop-miner.md` (블록 생성 루프)
- [x] 내부 루프 flow `flows/ep-loop-txpool.md` (풀 재정렬 루프)
- [x] 내부 루프 flow `flows/ep-loop-broadcast.md` (tx/블록 전파 루프)
- [x] 내부 루프 flow `flows/ep-loop-chainsync.md` (체인 동기화 루프)
- [x] `invariants.md` (16개 INV ↔ 강제 step 양방향 링크 + 다층 방어 표)
- [x] `SCHEMA.md` (JSONL 레코드 스키마 + vector/graph/벤치 매핑)
- [x] `tools/build_corpus.py` (flows + invariants → JSONL, 재현 가능)
- [x] 외부 요청 flow `flows/ep-p2p-snap.md` (상태 스냅샷 서빙/동기화)
- [x] 외부 요청 flow `flows/ep-discovery.md` (discv4/v5 노드 발견)
- [x] 외부 요청 flow `flows/ep-keystore.md` (키 파일 로드 + 서명)
- [x] `corpus.jsonl` 재생성 (flow 18 / step 79 / invariant 16 / edge 138 = 251 레코드)
- [ ] cks 실제 적재 통합 (JSONL → cks vector/graph store; cks 측 API 필요)

> **현재 단계:** `ep-rpc-sendrawtx.md` 견본으로 단위 형식을 확정한다. 이 형식이 세 용도(cks 적재 ·
> 벤치 정답 · 사람 읽기)에 맞으면, 위 목록을 같은 형식으로 채워 나간다.
