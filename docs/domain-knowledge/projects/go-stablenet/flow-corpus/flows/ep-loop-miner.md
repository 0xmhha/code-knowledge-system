---
flow: ep-loop-miner
entry_point: EP-LOOP-MINER
trigger: "WBFT 워크 루프 — readyToCommitCh 신호 / 새 tx 이벤트로 블록 생성 트리거"
root_symbol: "miner.(*worker).newWorkLoopWBFT"
summary: "제안자가 블록을 짓고 봉인 요청까지 보내는 루프. 트랜잭션 채움·실행·봉인 결과에서 갈린다."
links: [spine-statetransition, spine-finalize, spine-consensus-rounds]
called_by: [ep-main]
---

# Flow: EP-LOOP-MINER — 블록 생성 루프 (StableNet 구동 방식)

> geth의 recommit 타이머 대신 WBFT 엔진의 `readyToCommitCh`로 구동된다(`miner/worker.go:608`,
> StableNet 고유). [ep-rpc-sendrawtx](ep-rpc-sendrawtx.md)/[ep-p2p-eth](ep-p2p-eth.md)가 넣은 tx의
> `NewTxsEvent`도 이 루프를 깨운다.

### STEP loopminer-01
- symbol: `miner.(*worker).newWorkLoopWBFT` → `commitWork` → `prepareWork`
- at: `miner/worker.go:608` → `:1320` → `:1142`
- kind: stablenet
- calls: [loopminer-02]
- reads: `readyToCommitCh`, 부모 헤더, `engine.Prepare`
- writes: 새 블록 헤더(번호·타임스탬프·gasLimit·baseFee)
- emits: —
- branches:
  - when: "readyToCommitCh 신호(합의가 새 라운드 준비됨)" → then: "commit(NewHead)" at: `miner/worker.go:666`
  - when: "자신이 이번 라운드 제안자 아님" → then: "블록 생성 안 함(대기)" at: `miner/worker.go` (proposer 체크)
  - when: "`engine.Prepare` 실패" → then: "작업 중단" at: `miner/worker.go:1180`
- invariant: —
- prose: 합의 엔진이 "새 라운드 준비됨" 신호를 보내면 워크 루프가 깨어나 헤더를 짓고 `engine.Prepare`로 난수·라운드를 준비한다. 자신이 제안자가 아니면 블록을 만들지 않는다.

### STEP loopminer-02
- symbol: `miner.(*worker).fillTransactions` → `commitTransactions` → `applyTransaction`
- at: `miner/worker.go:1230` → `:985`
- kind: stablenet
- calls: [spine-statetransition]   # tx별 상태 전이
- reads: `txpool.Pending()` (base fee/팁 필터)
- writes: 상태(tx 실행 결과), 영수증
- emits: —
- branches:
  - when: "pending 트랜잭션 실행" → then: "applyTransaction (spine-statetransition)" at: `miner/worker.go:985`
  - when: "tx가 가스 한도 초과/실패" → then: "스킵 또는 중단(interrupt)" at: `miner/worker.go`
  - when: "블록 가스 한도 도달" → then: "트랜잭션 채움 종료" at: `miner/worker.go`
- invariant: [INV-LEDGER-01]
- prose: txpool에서 실행 가능한 트랜잭션을 base fee/팁으로 거른 뒤 하나씩 `applyTransaction`(spine-statetransition)으로 실행해 상태를 바꾸고 영수증을 모은다. 블록 가스 한도에 닿으면 채움을 멈춘다.

### STEP loopminer-03
- symbol: `miner.(*worker).commit` → `engine.FinalizeAndAssemble`
- at: `miner/worker.go:1391` → `:1401`
- kind: stablenet
- calls: [spine-finalize]
- reads: 실행된 tx·영수증·상태
- writes: 완성된 블록(상태 루트 포함)
- emits: taskCh로 task 전송
- branches:
  - when: "FinalizeAndAssemble 에러(base fee 분배 실패 등)" → then: "블록 폐기" at: `miner/worker.go:1401`
- invariant: [INV-LEDGER-02, INV-STATE-02]
- prose: 모인 트랜잭션으로 `FinalizeAndAssemble`(spine-finalize)을 불러 base fee 재분배·에폭·상태 루트까지 마감한 블록을 만들고, 봉인 작업(task)을 다음 단계로 보낸다. 마감 중 오류면 블록을 버린다.

### STEP loopminer-04
- symbol: `miner.(*worker).taskLoop` → `engine.Seal`
- at: `miner/worker.go:768` → `:804`
- kind: stablenet
- calls: [spine-consensus-rounds]   # Seal → RequestEvent → 합의
- reads: task(블록)
- writes: `pendingTasks[sealHash]`
- emits: `EventMux.Post(RequestEvent)` (backend.Seal 내부)
- branches:
  - when: "이전과 같은 sealHash" → then: "중복 작업 무시" at: `miner/worker.go:789`
  - when: "stopCh 신호" → then: "봉인 중단" at: `miner/worker.go`
- invariant: —
- prose: 완성된 블록을 `engine.Seal`에 넘긴다. WBFT 백엔드의 Seal은 블록을 RequestEvent로 만들어 합의 코어에 올리고(spine-consensus-rounds), 봉인 완료를 commitCh에서 기다린다.

### STEP loopminer-05
- symbol: `miner.(*worker).resultLoop` → `chain.WriteBlockAndSetHead`
- at: `miner/worker.go:819` → `:870`
- kind: stablenet
- calls: [spine-blockimport]   # 기록 부분 공유 (write & set head)
- reads: `resultCh`(봉인된 블록)
- writes: 블록·상태 DB 기록, head 설정
- emits: `NewMinedBlockEvent` (→ ep-loop-broadcast)
- branches:
  - when: "이미 체인에 있는 블록" → then: "무시" at: `miner/worker.go:829`
  - when: "task를 sealHash로 못 찾음" → then: "스킵" at: `miner/worker.go:833`
  - when: "Anzeon" → then: "GovValidator에서 gasTip 갱신" at: `miner/worker.go:876`
- invariant: —
- prose: 합의로 봉인된 블록이 resultCh로 돌아오면 체인에 쓰고 head로 올린 뒤 `NewMinedBlockEvent`를 쏘아 네트워크에 알린다(ep-loop-broadcast). Anzeon이면 컨트랙트에서 가스팁을 갱신한다.

---

## prose 렌더 (분기/실패 포함)
> WBFT 워크 루프는 합의가 "새 라운드 준비" 신호를 보낼 때 깨어나 헤더를 짓고(제안자가 아니면 안 만듦),
> txpool의 pending을 base fee로 걸러 하나씩 실행해(spine-statetransition) 상태와 영수증을 모은다.
> 블록 가스 한도에 닿으면 멈추고, `FinalizeAndAssemble`(spine-finalize)로 base fee 재분배·상태 루트까지
> 마감한다. 그 블록을 `Seal`에 넘기면 합의 코어로 올라가(spine-consensus-rounds) 봉인을 기다리고,
> 봉인돼 돌아오면 체인에 써 head로 올리고 `NewMinedBlockEvent`로 전파한다.
