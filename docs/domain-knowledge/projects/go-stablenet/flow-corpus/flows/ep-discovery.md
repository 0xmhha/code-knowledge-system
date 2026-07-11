---
flow: ep-discovery
entry_point: EP-P2P-DISCOVERY
trigger: "UDP 디스커버리 패킷 수신 (discv4: ping/pong/findnode/neighbors, discv5: 암호화 핸드셰이크 기반)"
root_symbol: "p2p/discover.(*UDPv4).readLoop"
summary: "UDP로 피어를 발견해 노드 테이블을 채운다. 결과는 다이얼링→연결로 이어진다. 디코드·본드·만료에서 갈린다."
links: [ep-main, ep-p2p-eth]
called_by: []
---

# Flow: EP-P2P-DISCOVERY — 노드 디스커버리 (UDP, geth 원본)

> 발견된 노드는 p2p 서버의 다이얼 대상이 되어, 연결되면 [ep-p2p-eth](ep-p2p-eth.md)/
> [ep-p2p-consensus](ep-p2p-consensus.md)의 입력원이 된다. 전부 geth 원본.

### STEP disc-01
- symbol: `p2p/discover.(*UDPv4).readLoop` → `handlePacket`
- at: `p2p/discover/v4_udp.go:514` → `:543`
- kind: geth
- calls: [disc-02]
- reads: UDP 소켓 raw 패킷
- writes: —
- emits: —
- branches:
  - when: "패킷 디코드/서명 검증 실패" → then: "드롭" at: `p2p/discover/v4_udp.go:543`
  - when: "패킷이 너무 큼" → then: "드롭" at: `p2p/discover/v4_udp.go:514`
  - when: "정상" → then: "타입별 핸들러로 분기(disc-02)" at: `p2p/discover/v4_udp.go:543`
- invariant: —
- prose: UDP 소켓에서 패킷을 읽어 디코드·서명 검증한다. 깨지거나 과대 패킷은 드롭하고, 유효하면 타입(ping/pong/findnode/neighbors)별 핸들러로 보낸다.

### STEP disc-02
- symbol: `(*UDPv4).handlePing` / `handleFindnode` / `ensureBond`
- at: `p2p/discover/v4_udp.go:657` / `:719` / `:568`
- kind: geth
- calls: [disc-03]
- reads: 패킷 내용, 노드 테이블, bond 상태
- writes: 노드 테이블(새/갱신 노드)
- emits: PONG / NEIGHBORS 응답
- branches:
  - when: "PING" → then: "PONG 응답 + 송신자 bond 갱신" at: `p2p/discover/v4_udp.go:657`
  - when: "FINDNODE인데 송신자와 bond 안 됨" → then: "무시(unbonded 거부)" at: `p2p/discover/v4_udp.go:719`
  - when: "FINDNODE & bond됨" → then: "가까운 노드들 NEIGHBORS로 응답" at: `p2p/discover/v4_udp.go:719`
  - when: "응답(PONG/NEIGHBORS)이 미요청/만료" → then: "드롭(unsolicited)" at: `p2p/discover/v4_udp.go`
- invariant: —
- prose: PING엔 PONG으로 답하며 송신자를 bond(검증된 이웃)로 기록하고, FINDNODE엔 bond된 상대에게만 가까운 노드 목록을 NEIGHBORS로 준다. 요청한 적 없거나 만료된 응답은 드롭한다.

### STEP disc-03
- symbol: 노드 테이블 갱신 → p2p 다이얼 → 연결
- at: `p2p/discover/table.go` (테이블) → `p2p/dial.go` → `p2p/server.go`
- kind: geth
- calls: [ep-p2p-eth]
- reads: 노드 테이블
- writes: 다이얼 대상 큐, 새 피어 연결
- emits: —
- branches:
  - when: "테이블에 새 후보 노드" → then: "다이얼 스케줄러가 연결 시도" at: `p2p/dial.go`
  - when: "연결 성공 & 프로토콜 협상" → then: "ep-p2p-eth/ep-p2p-consensus 시작" at: `p2p/server.go:1029`
  - when: "최대 피어 수 도달" → then: "다이얼 보류" at: `p2p/dial.go`
- invariant: —
- prose: 디스커버리로 채워진 노드 테이블에서 다이얼 스케줄러가 연결을 시도하고, 연결·프로토콜 협상에 성공하면 eth/합의 프로토콜 루프가 시작된다(즉 디스커버리는 그 두 entry point의 공급원). 최대 피어 수에 닿으면 다이얼을 멈춘다.

### STEP disc-v5 (분기 — discv5)
- symbol: `(*UDPv5).readLoop` → `handlePacket` → `handle` (handlePing/handleFindnode/handleWhoareyou)
- at: `p2p/discover/v5_udp.go:661` → `:693` → `:754`
- kind: geth
- calls: [disc-03]
- reads: 암호화된 v5 패킷, 세션
- writes: 노드 테이블, 세션 상태
- emits: 응답 패킷
- branches:
  - when: "세션 없음(Unknown 패킷)" → then: "WHOAREYOU 핸드셰이크 시작" at: `p2p/discover/v5_udp.go:778`
  - when: "WHOAREYOU 수신" → then: "핸드셰이크 키 교환" at: `p2p/discover/v5_udp.go:794`
  - when: "응답이 미요청 call에 대응" → then: "드롭" at: `p2p/discover/v5_udp.go:723`
- invariant: —
- prose: discv5는 v4와 목적은 같지만 세션 기반 암호화를 쓴다 — 세션이 없으면 WHOAREYOU로 핸드셰이크해 키를 교환한 뒤 ping/findnode를 처리한다. 미요청 응답은 드롭한다. 결과는 v4와 같이 노드 테이블을 채운다.

---

## prose 렌더 (분기/실패 포함)
> 디스커버리는 UDP로 피어를 찾는다. `readLoop`가 패킷을 읽어 디코드·서명 검증하고(깨지면 드롭), 타입별로
> 분기한다 — PING엔 PONG으로 답하며 상대를 bond로 기록하고, FINDNODE엔 bond된 상대에게만 가까운 노드를
> NEIGHBORS로 준다(unbonded 요청·미요청 응답은 무시). discv5는 세션 암호화를 써 세션이 없으면 WHOAREYOU로
> 핸드셰이크한다. 이렇게 채워진 노드 테이블에서 다이얼 스케줄러가 연결을 시도하고, 성공하면 eth/합의
> 프로토콜이 시작된다 — 디스커버리는 그 entry point들의 공급원이다.
