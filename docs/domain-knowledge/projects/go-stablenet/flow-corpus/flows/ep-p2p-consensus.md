---
flow: ep-p2p-consensus
entry_point: EP-P2P-CONSENSUS
trigger: "연결된 피어가 보낸 WBFT 합의 메시지 (PrePrepare/Prepare/Commit/RoundChange)"
root_symbol: "eth.(*handler).handleConsensus"
summary: "합의 메시지를 eth 핸들러보다 먼저 가로채 WBFT 코어 이벤트 루프에 전달. 엔진상태·중복·서명에서 갈린다."
links: [spine-consensus-rounds]
called_by: []
---

# Flow: EP-P2P-CONSENSUS — WBFT 합의 메시지 (StableNet 고유)

> 합의 메시지는 표준 eth 핸들러로 가기 전에 별도 합의 프로토콜이 가로챈다(`eth/quorum_protocol.go`,
> `eth/handler_istanbul.go`). 전부 StableNet 고유 코드.

### STEP p2pcons-01
- symbol: `eth.(*handler).makeQuorumConsensusProtocol` Run → `handleConsensusLoop` → `handleConsensus`
- at: `eth/quorum_protocol.go:44` → `eth/handler_istanbul.go:61` → `:115` → `:132`
- kind: stablenet
- calls: [p2pcons-02]
- reads: `protoRW.ReadMsg()`, eth 피어 등록 여부
- writes: 피어에 합의 RW 채널 부착
- emits: —
- branches:
  - when: "eth 피어가 아직 등록 안 됨" → then: "EthPeerRegistered 대기" at: `eth/handler_istanbul.go:81`
  - when: "메시지 읽기 실패/연결 끊김" → then: "루프 종료" at: `eth/handler_istanbul.go:134`
- invariant: —
- prose: 합의 프로토콜 `Run`은 기존 eth 피어가 등록되길 기다렸다가 그 피어에 합의용 채널을 붙이고, 합의 메시지를 끝없이 읽는 루프(`handleConsensus`)로 들어간다.

### STEP p2pcons-02
- symbol: `eth.(*handler).handleConsensusMsg` → `consensus.Handler.HandleMsg`
- at: `eth/handler_istanbul.go:155` → `:166`
- kind: stablenet
- calls: [p2pcons-03]
- reads: 합의 엔진(WBFT 백엔드)의 Handler 인터페이스
- writes: —
- emits: —
- branches:
  - when: "엔진이 Handler를 구현 안 함" → then: "처리 안 함(false)" at: `eth/handler_istanbul.go:156`
  - when: "HandleMsg가 consumed=false" → then: "합의 메시지 아님 → 무시" at: `eth/handler_istanbul.go:146`
- invariant: —
- prose: 합의 엔진의 `HandleMsg`에 메시지를 위임한다. 엔진이 합의 핸들러를 가지지 않거나 메시지가 합의용이 아니면 그냥 넘어간다.

### STEP p2pcons-03
- symbol: `consensus/wbft/backend.(*Backend).HandleMsg`
- at: `consensus/wbft/backend/handler.go:70`
- kind: stablenet
- calls: [p2pcons-04]
- reads: `msg.Code`, `recentMessages`/`knownMessages`(중복 캐시), `coreStarted`
- writes: 중복 캐시 갱신
- emits: `istanbulEventMux.Post(MessageEvent)`
- branches:
  - when: "메시지 코드가 WBFT 합의 코드 아님" → then: "false 반환(eth 핸들러로 넘김)" at: `backend/handler.go:73`
  - when: "엔진 미가동(`!coreStarted`)" → then: "ErrStoppedEngine" at: `backend/handler.go:74`
  - when: "디코드 실패" → then: "errDecodeFailed" at: `backend/handler.go:79`
  - when: "이미 본 메시지(knownMessages 히트)" → then: "드롭(재전파 안 함)" at: `backend/handler.go:91`
- invariant: —
- prose: 메시지가 WBFT 합의 코드인지 확인하고(아니면 eth 핸들러로 넘김), 엔진이 안 돌면 거부, 디코드 실패면 거부한다. 처음 보는 메시지만 `MessageEvent`로 만들어 합의 코어의 이벤트 버스에 던지고, 이미 본 메시지는 드롭한다(중복 가십 방지).

### STEP p2pcons-04
- symbol: `consensus/wbft/core.(*Core).handleEvents` → `handleEncodedMsg`
- at: `consensus/wbft/core/handler.go:102` → `:188`
- kind: stablenet
- calls: [spine-consensus-rounds]
- reads: `MessageEvent`, 메시지 서명
- writes: —
- emits: 성공 시 다른 검증자에게 Gossip
- branches:
  - when: "메시지 코드 무효" → then: "에러, 처리 중단" at: `core/handler.go:191`
  - when: "서명 검증 실패(`verifySignatures`)" → then: "거부(유효 검증자 메시지만 집계)" at: `core/handler.go:188`
  - when: "디코드 후 타입별 분기" → then: "deliverMessage → handle{Preprepare/Prepare/Commit/RoundChange}Msg (spine-consensus-rounds)" at: `core/handler.go`
  - when: "성공 처리" → then: "Gossip으로 재전파" at: `core/handler.go:135`
- invariant: [INV-CONSENSUS-01, INV-CONSENSUS-02]
- prose: 코어 이벤트 루프가 `MessageEvent`를 받아 서명을 검증하고(유효 검증자 메시지만 통과), 메시지 종류에 따라 PRE-PREPARE/PREPARE/COMMIT/RoundChange 핸들러로 분기시킨다(spine-consensus-rounds). 성공적으로 처리한 메시지는 다른 검증자에게 다시 가십한다.

---

## prose 렌더 (분기/실패 포함)
> 합의 프로토콜 `Run`은 eth 피어 등록을 기다렸다 합의 채널을 붙이고 메시지를 읽는다. `handleConsensus`가
> 엔진의 `HandleMsg`에 위임하면, 백엔드는 메시지가 WBFT 합의 코드인지(아니면 eth로 넘김)·엔진이 도는지·
> 디코드가 되는지 보고, 처음 보는 메시지만 `MessageEvent`로 이벤트 버스에 던진다(이미 본 건 드롭).
> 코어 이벤트 루프는 그 이벤트의 서명을 검증해 유효 검증자 메시지만 받아들이고, 종류대로 합의 상태
> 기계(spine-consensus-rounds)로 분기시키며 성공 시 재가십한다.
