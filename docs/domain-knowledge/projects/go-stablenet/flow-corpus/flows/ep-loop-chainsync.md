---
flow: ep-loop-chainsync
entry_point: EP-LOOP-CHAINSYNC
trigger: "동기화 루프 — 더 무거운 체인을 가진 피어가 보이면 따라잡기 시작"
root_symbol: "eth.(*chainSyncer).loop"
summary: "뒤처진 노드가 피어로부터 블록을 받아 따라잡는 루프. 동기 필요 판정·다운로드·import에서 갈린다."
links: [spine-blockimport]
called_by: [ep-main]
---

# Flow: EP-LOOP-CHAINSYNC — 체인 동기화 루프 (이벤트·주기 구동)

> [ep-main](ep-main.md)의 `handler.Start`가 띄우는 `chainSync.loop`(`eth/handler.go:575`). 받은
> 블록은 결국 [spine-blockimport](spine-blockimport.md)와 같은 검증을 거친다. 대부분 geth 원본.

### STEP loopsync-01
- symbol: `eth.(*chainSyncer).loop` → `nextSyncOp`
- at: `eth/sync.go:92` (loop; `nextSyncOp` :162, `startSync` :258)
- kind: geth
- calls: [loopsync-02]
- reads: 피어들의 TD/높이, 현재 체인 head
- writes: —
- emits: —
- branches:
  - when: "더 무거운(높은 TD) 피어 있음 & 피어 수 충분" → then: "startSync(동기화 시작)" at: `eth/sync.go:118`
  - when: "이미 최신 / 피어 부족" → then: "동기화 안 함(대기)" at: `eth/sync.go` (nextSyncOp nil)
  - when: "이미 동기화 진행 중" → then: "중복 시작 안 함" at: `eth/sync.go`
- invariant: —
- prose: 동기화 루프는 주기적으로/피어 변화 시 깨어나, 자기보다 무거운 체인을 가진 피어가 있고 피어 수가 충분하면 따라잡기를 시작한다. 이미 최신이거나 피어가 부족하면 대기한다.

### STEP loopsync-02
- symbol: `eth.(*handler).doSync` → `Downloader.synchronise`
- at: `eth/sync.go:264` → `eth/downloader/downloader.go:368`
- kind: geth
- calls: [loopsync-03]
- reads: 대상 피어, 동기화 모드(full/snap)
- writes: 헤더·본문·영수증 페치 큐
- emits: —
- branches:
  - when: "동기화 모드 snap" → then: "스냅 상태 동기화 경로(ep-p2p-snap)" at: `eth/downloader/downloader.go`
  - when: "피어가 잘못된 데이터 제공" → then: "피어 드롭, 동기화 실패" at: `eth/downloader/downloader.go`
  - when: "이미 동기화 중(`errBusy`)" → then: "거부" at: `eth/downloader/downloader.go:368`
- invariant: —
- prose: 대상 피어로부터 헤더·본문(·영수증)을 병렬로 내려받는다. snap 모드면 상태 스냅샷 경로를 타고, 피어가 잘못된 데이터를 주면 드롭한다.

### STEP loopsync-03
- symbol: `Downloader.importBlockResults` → `blockchain.InsertChain`
- at: `eth/downloader/downloader.go:1492` → `:1515`
- kind: geth
- calls: [spine-blockimport]
- reads: 내려받은 블록 결과
- writes: (InsertChain을 통해) 체인·상태
- emits: —
- branches:
  - when: "InsertChain이 검증 실패 인덱스 반환" → then: "동기화 중단, 피어 드롭(잘못된 체인)" at: `eth/downloader/downloader.go:1515`
  - when: "정상" → then: "다음 배치 계속" at: `eth/downloader/downloader.go:1676`
- invariant: [INV-CONSENSUS-04, INV-STATE-01]
- prose: 내려받은 블록을 `blockchain.InsertChain`에 넣는다 — 즉 동기화로 받은 블록도 spine-blockimport와 **똑같이** seal 정족수·BLS·재실행·상태 루트 검증을 거친다. 검증에 실패하면 그 피어를 잘못된 체인으로 보고 드롭한다.

---

## prose 렌더 (분기/실패 포함)
> 동기화 루프는 주기적으로 깨어나, 자기보다 무거운 체인을 가진 피어가 있고 피어가 충분하면 따라잡기를
> 시작한다(최신이거나 피어 부족이면 대기). `doSync`→`Downloader.synchronise`가 대상 피어에서 헤더·본문을
> 내려받고(snap 모드면 상태 스냅샷 경로), 받은 블록을 `InsertChain`에 넣어 spine-blockimport와 같은
> seal·재실행·상태 검증을 받게 한다. 검증에 실패하면 그 피어를 드롭한다 — 동기화 경로로도 잘못된
> 블록은 체인에 들어올 수 없다.
