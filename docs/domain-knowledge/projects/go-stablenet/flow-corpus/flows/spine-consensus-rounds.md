---
flow: spine-consensus-rounds
entry_point: SPINE
trigger: "제안 블록(RequestEvent) 또는 합의 메시지(MessageEvent)가 코어 이벤트 루프에 도착"
root_symbol: "consensus/wbft/core.(*Core).handleEvents"
summary: "QBFT 3단계(PRE-PREPARE→PREPARE→COMMIT)로 블록을 확정. 정족수·digest·서명·라운드에서 갈린다."
links: [spine-blockimport]
called_by: [ep-loop-consensus, ep-p2p-consensus, ep-loop-miner]   # 제안(Seal)·수신 메시지 양쪽
---

# Flow: SPINE — 합의 라운드 (QBFT consensus)

> [ep-p2p-consensus](ep-p2p-consensus.md)로 받은 메시지와 [ep-loop-miner](ep-loop-miner.md)의 제안
> (Seal→RequestEvent)이 모두 코어 이벤트 루프로 모여 이 3단계를 돈다. **블록이 확정되는 유일한
> 지점**이며 안전성(INV-CONSENSUS-*)의 핵심.

### STEP rounds-01
- symbol: `consensus/wbft/core.(*Core).handleRequest`
- at: `consensus/wbft/core/request.go:33`
- kind: stablenet
- calls: [rounds-02]
- reads: 제안 블록, 현재 상태/라운드
- writes: `c.current.pendingRequest`
- emits: PRE-PREPARE(제안자일 때)
- branches:
  - when: "`checkRequestMsg` 실패(블록 부적합)" → then: "거부" at: `request.go:38`
  - when: "미래 블록" → then: "errFutureMessage → 보류(나중에 재처리)" at: `core/handler.go:124`
  - when: "StateAcceptRequest & 라운드 0 & 자신이 제안자" → then: "sendPreprepareMsg로 PRE-PREPARE 전파" at: `request.go:52`
  - when: "제안자가 아님" → then: "요청만 저장, 전파 안 함" at: `request.go`
- invariant: —
- prose: 제안 블록을 검사해 저장하고, 자신이 이번 라운드 제안자면 PRE-PREPARE를 전파해 합의를 연다. 미래 블록이면 보류했다가 다시 처리한다.

### STEP rounds-02
- symbol: `consensus/wbft/core.(*Core).handlePreprepareMsg`
- at: `consensus/wbft/core/preprepare.go:114`
- kind: stablenet
- calls: [rounds-03]
- reads: `preprepare.Source`, `Sequence`, `Round`, `Proposal`, valSet
- writes: `c.current.SetPreprepare`, 상태→StatePreprepared
- emits: PREPARE
- branches:
  - when: "제안자가 현재 제안자가 아님" → then: "errNotFromProposer, 무시" at: `preprepare.go:122`
  - when: "sequence ≠ proposal number" → then: "errInvalidPreparedBlock" at: `preprepare.go:128`
  - when: "라운드 > 0 인데 justification 실패" → then: "errInvalidPreparedBlock(거짓 fork 제안 차단)" at: `preprepare.go:135`
  - when: "블록 검증(`backend.Verify`) 실패" → then: "거부 / 미래블록이면 보류" at: `preprepare.go:142`
- invariant: [INV-CONSENSUS-03]
- prose: PRE-PREPARE가 진짜 제안자에게서 왔고 시퀀스가 맞는지 보고, **라운드>0이면 justification으로 이전에 prepare된 블록을 계승했는지 검증**(거짓 fork 제안 차단)한 뒤 블록 유효성까지 확인하면 받아들이고 PREPARE를 전파한다.

### STEP rounds-03
- symbol: `consensus/wbft/core.(*Core).handlePrepareMsg`
- at: `consensus/wbft/core/prepare.go:87`
- kind: stablenet
- calls: [rounds-04]
- reads: `prepare.Digest`, prepare seal, `WBFTPrepares.Size()`, `QuorumSize()`
- writes: `WBFTPrepares.Add`, (정족수 시) `preparedRound`/`preparedBlock` lock
- emits: COMMIT(정족수 시)
- branches:
  - when: "digest ≠ 현재 제안 해시" → then: "errInvalidMessage(다른 블록 표 무시)" at: `prepare.go:93`
  - when: "prepare seal 검증 실패" → then: "errInvalidMessage" at: `prepare.go:105`
  - when: "PREPARE 수 ≥ QuorumSize & 아직 Prepared 이전" → then: "블록 lock + 상태 Prepared + broadcastCommit" at: `prepare.go:121`
  - when: "정족수 미달" → then: "표만 누적, 대기" at: `prepare.go:142`
- invariant: [INV-CONSENSUS-01, INV-CONSENSUS-03]
- prose: 현재 제안 블록과 같은 digest의 PREPARE만(서명 검증 후) 누적한다. 수가 정족수(`QuorumSize()=ceil(N − (N−1)/3)`)에 도달하면 그 블록을 lock하고(preparedRound/Block — 라운드 변경 시 계승 근거) COMMIT을 전파한다. 미달이면 대기한다.

### STEP rounds-04
- symbol: `consensus/wbft/core.(*Core).handleCommitMsg`
- at: `consensus/wbft/core/commit.go:90`
- kind: stablenet
- calls: [rounds-05]
- reads: `commit.Digest`, commit seal, `WBFTCommits.Size()`, `QuorumSize()`
- writes: `WBFTCommits.Add`
- emits: —
- branches:
  - when: "digest ≠ 현재 제안 해시" → then: "errInvalidMessage(다른 블록 표 무시)" at: `commit.go:96`
  - when: "commit seal 검증 실패" → then: "errInvalidMessage" at: `commit.go:108`
  - when: "COMMIT 수 ≥ QuorumSize" → then: "commitWBFT(블록 확정)" at: `commit.go:123`
  - when: "정족수 미달" → then: "표만 누적, 대기" at: `commit.go:127`
- invariant: [INV-CONSENSUS-01, INV-CONSENSUS-02]
- prose: 같은 digest의 COMMIT만(서명 검증 후) 누적하고, **정족수에 도달한 그 순간에만** `commitWBFT`로 블록을 확정한다. 두 충돌 블록이 각각 정족수를 모을 수 없으므로(교집합에 정직 검증자) 같은 높이에서 한 블록만 확정된다.

### STEP rounds-05
- symbol: `consensus/wbft/core.(*Core).commitWBFT` → `backend.(*Backend).Commit`
- at: `consensus/wbft/core/commit.go:137` → `consensus/wbft/backend/backend.go:213`
- kind: stablenet
- calls: [spine-blockimport]   # 확정 블록이 체인에 기록/전파됨
- reads: PREPARE/COMMIT seal들, `proposedBlockHash`
- writes: 헤더에 prepared/committed seal 박음(`CommitHeader`), 블록 봉인
- emits: `commitCh`(내 제안 블록) 또는 broadcaster.Enqueue(남의 블록)
- branches:
  - when: "확정 블록 == 내가 제안한 블록" → then: "commitCh로 전달 → 마이너 resultLoop가 WriteBlockAndSetHead" at: `backend.go:241`
  - when: "남이 제안한 블록" → then: "broadcaster.Enqueue → import 경로(spine-blockimport)" at: `backend.go:246`
  - when: "`backend.Commit` 에러" → then: "broadcastNextRoundChange(라운드 변경)" at: `core/commit.go:175`
- invariant: [INV-CONSENSUS-04]
- prose: 모인 seal을 헤더에 박아 블록을 봉인하고, 내가 제안한 블록이면 commitCh로 흘려 마이너가 체인에 쓰게 하며, 남의 블록이면 import 경로로 보낸다. 커밋이 실패하면 라운드 변경을 시작한다.

### STEP rounds-TO (분기 — 타임아웃 경로)
- symbol: `consensus/wbft/core.(*Core).handleTimeoutMsg` → `broadcastRoundChange` → `handleRoundChangeMsg`
- at: `consensus/wbft/core/handler.go:159` → `roundchange.go:52` / `:100`
- kind: stablenet
- calls: [rounds-02]   # 새 라운드에서 다시 PRE-PREPARE로
- reads: 타임아웃, RoundChange 메시지, `preparedRound`/`preparedBlock`, justification
- writes: 라운드 증가, RoundChange 누적
- emits: ROUND-CHANGE
- branches:
  - when: "라운드 타이머 만료" → then: "broadcastRoundChange(자신의 prepared 증거 첨부)" at: `handler.go:159`
  - when: "RoundChange 정족수 & 자신이 새 제안자" → then: "highestPrepared 블록 계승해 sendPreprepareMsg" at: `roundchange.go:170`
  - when: "justification 불충분" → then: "제안 거부" at: `justification.go:50`
- invariant: [INV-CONSENSUS-03]
- prose: 합의가 시간 내 안 끝나면 라운드를 올리며, 각 노드는 자신이 prepare한 블록 증거를 RoundChange에 실어 보낸다. 새 제안자는 모인 RoundChange 중 가장 높은 prepared 블록을 계승해야 하며(justification 강제), 이로써 라운드가 바뀌어도 충돌 블록이 끼어들 수 없다.

---

## 이 flow가 보존하는 약속 (요약)
- 블록 확정은 오직 COMMIT 정족수(`ceil(N − (N−1)/3)`)에서만(INV-CONSENSUS-01); 두 충돌 블록 동시 확정 불가
  (INV-CONSENSUS-02); 라운드 변경 시 prepared 블록 계승 강제(INV-CONSENSUS-03).
- 확정 블록은 import에서 seal 정족수·BLS가 재검증된다(INV-CONSENSUS-04, spine-blockimport).

## prose 렌더 (분기/실패 포함)
> 코어 이벤트 루프는 제안(RequestEvent)과 합의 메시지(MessageEvent)를 받는다. 제안자는 `handleRequest`로
> PRE-PREPARE를 연다(미래 블록이면 보류). 수신측 `handlePreprepareMsg`는 진짜 제안자·시퀀스 일치를 보고,
> 라운드>0이면 justification으로 이전 prepared 블록 계승을 검증한 뒤(거짓 fork 차단) PREPARE를 전파한다.
> `handlePrepareMsg`는 같은 digest의 PREPARE만 모아 정족수에 닿으면 블록을 lock하고 COMMIT을 전파하며,
> `handleCommitMsg`는 같은 digest의 COMMIT만 모아 정족수에 닿는 순간에만 `commitWBFT`로 확정한다.
> 확정 블록은 내가 제안한 것이면 commitCh로 마이너에, 남의 것이면 import 경로로 간다. 시간 내 합의가
> 안 되면 타임아웃으로 라운드를 올리고, 새 제안자는 모인 RoundChange 중 가장 높은 prepared 블록을
> 계승해야 한다 — 그래서 라운드가 바뀌어도 같은 높이엔 한 블록만 확정된다.
