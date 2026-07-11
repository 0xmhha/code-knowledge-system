---
flow: ep-loop-broadcast
entry_point: EP-LOOP-BROADCAST
trigger: "NewTxsEvent / NewMinedBlockEvent 구독 — 트랜잭션·채굴 블록을 피어에 전파"
root_symbol: "eth.(*handler).txBroadcastLoop / minedBroadcastLoop"
summary: "풀에 든 tx와 새로 만든 블록을 피어에게 퍼뜨리는 루프. 전파 대상 선정에서 갈린다."
links: [ep-p2p-eth]
called_by: [ep-main]
---

# Flow: EP-LOOP-BROADCAST — 전파 루프 (이벤트 구동)

> [ep-main](ep-main.md)의 `handler.Start`가 띄우는 고루틴들(`eth/handler.go:566`,`:571`). 로컬에서
> 일어난 일을 네트워크로 내보내는 출구 — 다른 노드 입장에선 [ep-p2p-eth](ep-p2p-eth.md)의 입력이 된다.

### STEP loopbcast-01
- symbol: `eth.(*handler).txBroadcastLoop`
- at: `eth/handler.go:566` (구독은 `:565` `SubscribeTransactions`)
- kind: geth
- calls: [ep-p2p-eth]   # 다른 노드의 EP-P2P-ETH 입력이 됨
- reads: `txsCh`(NewTxsEvent)
- writes: —
- emits: 피어로 Transactions / NewPooledTransactionHashes 전송
- branches:
  - when: "tx 이벤트 수신" → then: "일부 피어엔 전체 tx, 나머지엔 해시만 announce" at: `eth/handler.go` (BroadcastTransactions)
  - when: "구독 종료" → then: "루프 종료" at: `eth/handler.go`
- invariant: —
- prose: txpool이 쏜 `NewTxsEvent`를 받아, 일부 피어에게는 트랜잭션 전체를, 나머지에게는 해시만 알린다(대역폭 절약). 이는 받는 노드 쪽에서 ep-p2p-eth의 입력이 된다.

### STEP loopbcast-02
- symbol: `eth.(*handler).minedBroadcastLoop`
- at: `eth/handler.go:571` (구독은 `:570` `NewMinedBlockEvent`)
- kind: geth
- calls: [ep-p2p-eth]
- reads: `minedBlockSub`(NewMinedBlockEvent)
- writes: —
- emits: 피어로 NewBlock / NewBlockHashes 전송
- branches:
  - when: "채굴/확정 블록 이벤트" → then: "일부 피어엔 블록 전체(NewBlock), 나머지엔 해시(NewBlockHashes)" at: `eth/handler.go` (BroadcastBlock)
  - when: "구독 종료" → then: "루프 종료" at: `eth/handler.go`
- invariant: —
- prose: 로컬에서 확정된 블록 이벤트(ep-loop-miner의 `NewMinedBlockEvent`)를 받아, 일부 피어엔 블록 전체를 나머지엔 해시만 전파한다. 받는 노드 쪽에서 ep-p2p-eth → spine-blockimport로 이어진다.

---

## prose 렌더 (분기/실패 포함)
> 전파 루프는 두 갈래다. tx 브로드캐스트 루프는 txpool의 `NewTxsEvent`를 받아 일부 피어엔 트랜잭션
> 전체를, 나머지엔 해시만 알린다. 블록 브로드캐스트 루프는 마이너의 `NewMinedBlockEvent`를 받아 일부
> 피어엔 블록 전체(NewBlock)를, 나머지엔 해시(NewBlockHashes)만 보낸다. 이 출력은 다른 노드 입장에서
> ep-p2p-eth의 입력이 되어, 트랜잭션은 txpool로 블록은 import로 흘러간다.
