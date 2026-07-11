---
flow: ep-p2p-snap
entry_point: EP-P2P-SNAP
trigger: "피어가 보낸 snap 프로토콜 메시지 (GetAccountRange/GetStorageRanges/GetByteCodes/GetTrieNodes 및 응답)"
root_symbol: "eth/protocols/snap.HandleMessage"
summary: "상태(state) 스냅샷을 서빙하거나 동기화한다. 블록이 아닌 계정·스토리지·바이트코드·트리노드 단위. 요청 한도·잘못된 데이터에서 갈린다."
links: [ep-loop-chainsync]
called_by: []
---

# Flow: EP-P2P-SNAP — snap 프로토콜 (상태 스냅샷, geth 원본)

> eth 프로토콜이 *블록*을 나른다면 snap은 *상태*를 나른다. snap-sync 모드에서 빠른 상태 따라잡기에
> 쓰인다([ep-loop-chainsync](ep-loop-chainsync.md)의 snap 분기). 전부 geth 원본.

### STEP p2psnap-01
- symbol: `eth/protocols/snap.MakeProtocols` Run → `Handle` → `HandleMessage`
- at: `eth/protocols/snap/handler.go:87` → `:121` → `:133`
- kind: geth
- calls: [p2psnap-02, p2psnap-03]
- reads: `peer.rw.ReadMsg()`, `msg.Code`
- writes: —
- emits: —
- branches:
  - when: "`msg.Size > maxMessageSize`" → then: "errMsgTooLarge, 연결 종료" at: `eth/protocols/snap/handler.go:133`
  - when: "요청 메시지(GetAccountRange 등)" → then: "Service*Query로 서빙(p2psnap-02)" at: `eth/protocols/snap/handler.go`
  - when: "응답 메시지(AccountRange 등)" → then: "동기화 처리로 전달(p2psnap-03)" at: `eth/protocols/snap/handler.go`
  - when: "모르는 코드" → then: "errInvalidMsgCode" at: `eth/protocols/snap/handler.go`
- invariant: —
- prose: snap 메시지를 읽어, 요청이면 상태를 떠 응답을 만들고, 응답이면 동기화 처리로 넘긴다. 메시지가 너무 크거나 모르는 코드면 연결을 끊는다.

### STEP p2psnap-02 (서빙 측 — 상태 제공)
- symbol: `ServiceGetAccountRangeQuery` / `ServiceGetStorageRangesQuery` / `ServiceGetByteCodesQuery` / `ServiceGetTrieNodesQuery`
- at: `eth/protocols/snap/handler.go:331` / `:391` / `:504` / `:534`
- kind: geth
- calls: []
- reads: 로컬 상태 트리(계정·스토리지·코드·트리노드)
- writes: —
- emits: 피어로 AccountRange/StorageRanges/ByteCodes/TrieNodes 응답
- branches:
  - when: "응답 크기가 soft limit 초과" → then: "부분 응답으로 자름(truncate)" at: `eth/protocols/snap/handler.go:331`
  - when: "요청 트리노드가 과다(hard limit)" → then: "에러 반환" at: `eth/protocols/snap/handler.go:534`
  - when: "상태 루트가 로컬에 없음(pruned)" → then: "빈 응답" at: `eth/protocols/snap/handler.go`
- invariant: —
- prose: 요청받은 계정 범위·스토리지 범위·바이트코드·트리노드를 로컬 상태에서 떠서 응답한다. 응답이 크면 soft limit에서 자르고, 트리노드 요청이 과다하면 거부하며, 해당 상태 루트가 prune됐으면 빈 응답을 준다.

### STEP p2psnap-03 (동기화 측 — 상태 수신)
- symbol: `eth/protocols/snap.(*Syncer).OnAccounts` (외 OnStorage/OnByteCodes/OnTrieNodes)
- at: `eth/protocols/snap/sync.go:2480` (OnAccounts; OnByteCodes:2580, OnStorage:2691, OnTrieNodes:2840)
- kind: geth
- calls: [ep-loop-chainsync]
- reads: 수신한 계정·스토리지·코드, 대상 상태 루트
- writes: 로컬 상태 트리(스냅샷 → 트리 healing)
- emits: —
- branches:
  - when: "응답이 요청 범위/증명과 불일치(잘못된 데이터)" → then: "피어 드롭, 재요청" at: `eth/protocols/snap/sync.go`
  - when: "Merkle 증명 검증 실패" → then: "거부(상태 위조 차단)" at: `eth/protocols/snap/sync.go`
  - when: "대상 상태 루트 도달" → then: "snap sync 완료 → full 동기화로 전환" at: `eth/protocols/snap/sync.go`
- invariant: [INV-STATE-01]
- prose: 받은 상태 조각을 Merkle 증명으로 검증하며 로컬 트리를 채운다. 증명이 안 맞거나 범위가 어긋나면(상태 위조 시도) 피어를 드롭하고 재요청한다 — 즉 받은 상태도 대상 블록의 상태 루트에 대해 검증되므로, 잘못된 상태는 받아들여지지 않는다(INV-STATE-01과 같은 결정론 보장).

---

## prose 렌더 (분기/실패 포함)
> snap은 블록이 아닌 상태(계정·스토리지·코드·트리노드)를 나르는 프로토콜이다. `HandleMessage`는 메시지를
> 읽어 요청이면 로컬 상태를 떠 응답하고(크면 soft limit에서 자르고, 과다 요청·pruned 루트는 거부/빈 응답),
> 응답이면 동기화 처리로 넘긴다. 동기화 측은 받은 상태 조각을 Merkle 증명으로 검증하며 트리를 채우는데,
> 증명이 안 맞거나 범위가 어긋나면 피어를 드롭하고 재요청한다. 대상 상태 루트에 도달하면 full 동기화로
> 전환한다 — 잘못된 상태는 증명 검증에서 걸러진다.
