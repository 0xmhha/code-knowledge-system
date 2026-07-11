---
flow: ep-p2p-eth
entry_point: EP-P2P-ETH
trigger: "연결된 피어가 보낸 eth 와이어 메시지 (NewBlock, Transactions, NewPooledTransactionHashes 등)"
root_symbol: "eth/protocols/eth.handleMessage"
summary: "eth 프로토콜 메시지를 코드별로 분기해 txpool 또는 블록 import로 보낸다. 디코드·검증에서 갈린다."
links: [ep-rpc-sendrawtx, spine-blockimport]
called_by: []
---

# Flow: EP-P2P-ETH — eth 와이어 프로토콜 메시지

> 피어 연결 시 P2P 서버가 띄운 프로토콜 `Run` 루프에서 메시지를 끝없이 읽어 처리한다(geth 원본).
> 합의 메시지는 이 경로 전에 별도로 가로채진다([ep-p2p-consensus](ep-p2p-consensus.md)).

### STEP p2peth-01
- symbol: `p2p.(*Peer).startProtocols` → `eth/protocols/eth.MakeProtocols` Run → `Handle`
- at: `p2p/peer.go:425` → `eth/protocols/eth/handler.go:100` → `:154`
- kind: geth
- calls: [p2peth-02]
- reads: 협상된 프로토콜 능력
- writes: 피어 객체
- emits: —
- branches:
  - when: "프로토콜 능력 협상 실패" → then: "연결 종료" at: `p2p/server.go:988`
  - when: "핸드셰이크(상태교환) 실패/네트워크ID·제네시스 불일치" → then: "연결 거부" at: `eth/protocols/eth/handshake.go`
- invariant: —
- prose: 피어가 연결되면 능력 협상 후 eth 프로토콜의 `Run`이 핸드셰이크(네트워크ID·제네시스·TD 교환)를 하고, 통과하면 메시지 처리 루프 `Handle`로 들어간다. 네트워크/제네시스가 다르면 연결을 끊는다.

### STEP p2peth-02
- symbol: `eth/protocols/eth.handleMessage`
- at: `eth/protocols/eth/handler.go:186`
- kind: geth
- calls: [p2peth-03a, p2peth-03b]
- reads: `peer.rw.ReadMsg()`, `msg.Code`
- writes: —
- emits: —
- branches:
  - when: "`msg.Size > maxMessageSize`" → then: "errMsgTooLarge, 연결 종료" at: `eth/protocols/eth/handler.go`
  - when: "`msg.Code`가 핸들러 맵에 없음" → then: "errInvalidMsgCode" at: `eth/protocols/eth/handler.go:216`
  - when: "코드별 핸들러 매칭" → then: "해당 handler 호출(NewBlock/Transactions/...)" at: `eth/protocols/eth/handler.go:169`
- invariant: —
- prose: 메시지를 하나 읽어 코드별 핸들러 맵에서 처리기를 찾아 호출한다. 메시지가 너무 크거나 모르는 코드면 연결을 끊는다.

### STEP p2peth-03a (트랜잭션 계열)
- symbol: `handleTransactions` / `handleNewPooledTransactionHashes` → `ethHandler.Handle` → `txFetcher`
- at: `eth/protocols/eth/handlers.go:572` / `:514` → `eth/handler_eth.go:60` → `eth/fetcher/tx_fetcher.go:299`
- kind: geth
- calls: [ep-rpc-sendrawtx]   # txFetcher가 addTxs 콜백으로 txpool.Add 호출 (검증 step 공유)
- reads: 트랜잭션/해시 목록
- writes: txFetcher 큐
- emits: —
- branches:
  - when: "디코드 실패" → then: "연결 종료" at: `eth/protocols/eth/handlers.go:572`
  - when: "Transactions" → then: "txFetcher.Enqueue → txpool.Add" at: `eth/handler_eth.go:73`
  - when: "NewPooledTransactionHashes" → then: "txFetcher.Notify(필요 시 GetPooledTransactions 요청)" at: `eth/handler_eth.go:70`
- invariant: —
- prose: 트랜잭션이나 그 해시 알림을 받아 txFetcher에 넘긴다. txFetcher는 결국 `txpool.Add`를 호출하므로, 이후 검증·삽입은 ep-rpc-sendrawtx의 step 08~11과 동일하다(같은 txpool 검증을 공유).

### STEP p2peth-03b (블록 계열)
- symbol: `handleNewBlock` / `handleNewBlockhashes` → `ethHandler.Handle` → `blockFetcher`
- at: `eth/protocols/eth/handlers.go:301` → `eth/handler_eth.go:126` → `eth/fetcher/block_fetcher.go`
- kind: geth
- calls: [spine-blockimport]   # blockFetcher가 insertChain 콜백으로 InsertChain 호출
- reads: 블록/블록해시
- writes: blockFetcher 큐
- emits: —
- branches:
  - when: "디코드/TD 검증 실패" → then: "연결 종료" at: `eth/protocols/eth/handlers.go:303`
  - when: "NewBlock" → then: "blockFetcher.Enqueue → InsertChain" at: `eth/handler_eth.go:134`
  - when: "NewBlockHashes" → then: "blockFetcher.Notify(헤더/본문 요청)" at: `eth/handler_eth.go:119`
- invariant: [INV-CONSENSUS-04, INV-STATE-01]
- prose: 새 블록이나 블록 해시 알림을 받아 blockFetcher에 넘긴다. blockFetcher는 `blockchain.InsertChain`을 호출하므로, 이후 헤더 seal 검증·재실행·상태 검증은 spine-blockimport와 동일하다 — 전파만으로는 정족수 미달/위조 블록이 들어올 수 없다.

---

## prose 렌더 (분기/실패 포함)
> 피어가 연결되면 능력 협상과 핸드셰이크(네트워크ID·제네시스 일치)를 거쳐 eth 프로토콜 메시지 루프가
> 돈다. `handleMessage`는 메시지를 읽어 코드별 핸들러로 분기하는데, 메시지가 너무 크거나 모르는 코드면
> 연결을 끊는다. 트랜잭션·해시 알림은 txFetcher를 거쳐 `txpool.Add`로 가 ep-rpc-sendrawtx와 같은
> 검증을 받고, 새 블록·블록해시는 blockFetcher를 거쳐 `InsertChain`으로 가 spine-blockimport와 같은
> seal·상태 검증을 받는다. 디코드/검증 실패는 연결 종료로 이어진다.
