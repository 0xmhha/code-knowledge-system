---
flow: ep-loop-txpool
entry_point: EP-LOOP-TXPOOL
trigger: "새 체인 head(ChainHeadEvent) 또는 새 tx 큐 이벤트로 풀 재정렬"
root_symbol: "core/txpool/legacypool.(*LegacyPool).scheduleReorgLoop"
summary: "체인 head가 바뀌거나 tx가 들어올 때 풀을 reset·promote·demote하고 pending을 알린다. 잔액·논스 변화에서 갈린다."
links: [ep-loop-miner]
called_by: [ep-main]
---

# Flow: EP-LOOP-TXPOOL — txpool 재정렬 루프 (이벤트 구동)

> 풀에 든 트랜잭션의 실행 가능성은 **체인 상태가 바뀔 때마다** 재평가되어야 한다. 이 루프가 그
> 백그라운드 시작점이다. (삽입 자체는 [ep-rpc-sendrawtx](ep-rpc-sendrawtx.md) step 08~11.)

### STEP looptxpool-01
- symbol: `core/txpool/legacypool.(*LegacyPool).scheduleReorgLoop`
- at: `core/txpool/legacypool/legacypool.go:1294`
- kind: geth
- calls: [looptxpool-02]
- reads: `queueTxEventCh`(새 tx), 체인 head 변경 신호(`chainHeadCh`)
- writes: 재정렬 작업 배치
- emits: —
- branches:
  - when: "새 tx 큐 이벤트" → then: "해당 계정을 dirty로 모아 promote 대상에 포함" at: `legacypool.go:1340`
  - when: "체인 head 변경" → then: "reset(새 상태 기준 재평가) 트리거" at: `legacypool.go` (chainHead)
  - when: "shutdown" → then: "루프 종료" at: `legacypool.go`
- invariant: —
- prose: 새 트랜잭션이 큐에 들어오거나 체인 head가 바뀌면 이 루프가 재정렬 배치를 모은다. head 변경은 새 상태 기준으로 풀 전체를 다시 평가하게(reset) 만든다.

### STEP looptxpool-02
- symbol: `(*LegacyPool).runReorg` → `reset` / `promoteExecutables` / `demoteUnexecutables`
- at: `core/txpool/legacypool/legacypool.go:1364`
- kind: geth
- calls: [looptxpool-03]
- reads: 새 상태(`currentState`), 각 계정 논스·잔액, pending/queue
- writes: pending↔queue 이동, 무효 tx 제거
- emits: —
- branches:
  - when: "계정 논스가 올라가 실행됨" → then: "해당 tx 제거(이미 블록에 포함)" at: `legacypool.go` (demote)
  - when: "queued tx의 논스가 실행 가능해짐" → then: "promote(queue→pending)" at: `legacypool.go:1003`
  - when: "잔액 부족으로 더는 못 냄" → then: "demote/제거" at: `legacypool.go` (demote)
  - when: "promote 가격 인상 부족(교체)" → then: "거부" at: `legacypool.go`
- invariant: [INV-TX-02]
- prose: 새 상태를 기준으로, 이미 블록에 포함돼 논스가 올라간 tx를 빼고, 논스가 실행 가능해진 queued tx를 pending으로 올리며(promote), 잔액이 모자라진 tx를 demote/제거한다. 즉 풀은 항상 "현재 상태에서 실행 가능한 것"만 pending에 둔다.

### STEP looptxpool-03
- symbol: `(*LegacyPool).txFeed.Send(NewTxsEvent)`
- at: `core/txpool/legacypool/legacypool.go:1440`
- kind: geth
- calls: [ep-loop-miner]
- reads: 새로 pending이 된 tx들
- writes: —
- emits: `core.NewTxsEvent`
- branches:
  - when: "새로 pending된 tx 있음" → then: "NewTxsEvent 브로드캐스트 → 마이너·브로드캐스트 루프 구독" at: `legacypool.go:1440`
  - when: "변화 없음" → then: "이벤트 안 쏨" at: `legacypool.go`
- invariant: —
- prose: 재정렬로 새로 pending이 된 트랜잭션이 있으면 `NewTxsEvent`를 쏜다 — 마이너(ep-loop-miner)와 브로드캐스트 루프(ep-loop-broadcast)가 이를 구독한다.

---

## prose 렌더 (분기/실패 포함)
> txpool 재정렬 루프는 새 트랜잭션이 들어오거나 체인 head가 바뀔 때 깨어난다. head 변경은 새 상태
> 기준으로 풀을 reset하게 만든다. 재정렬은 이미 블록에 포함돼 논스가 올라간 tx를 빼고, 논스가 실행
> 가능해진 queued tx를 pending으로 올리며, 잔액이 부족해진 tx를 demote한다. 새로 pending된 tx가 있으면
> `NewTxsEvent`를 쏘아 마이너와 브로드캐스트 루프가 이어받는다. 즉 풀은 항상 현재 상태에서 실행 가능한
> 것만 pending에 유지한다.
