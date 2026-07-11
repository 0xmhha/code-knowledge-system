---
flow: ep-loop-consensus
entry_point: EP-LOOP-CONSENSUS   # + EP-LOOP-CONSENSUS-TIMEOUT
trigger: "코어 이벤트 루프 — 제안/합의메시지/타임아웃/확정완료/재시도 채널 수신"
root_symbol: "consensus/wbft/core.(*Core).handleEvents"
summary: "합의 코어의 심장 루프. 외부 메시지뿐 아니라 라운드 타임아웃·확정완료로도 스스로 깨어난다."
links: [spine-consensus-rounds, spine-blockimport]
called_by: [ep-main]   # 기동 시 core.Start로 시작됨; 이후 자체 구동
---

# Flow: EP-LOOP-CONSENSUS — 합의 이벤트 루프 (타이머·이벤트 구동)

> [ep-main](ep-main.md)의 `core.Start`로 시작되어 스스로 도는 루프. 외부 메시지(ep-p2p-consensus,
> Seal)뿐 아니라 **타이머로도 깨어나는** 독립 시작점이다.

### STEP loopcons-01
- symbol: `consensus/wbft/core.(*Core).handleEvents` (select 루프)
- at: `consensus/wbft/core/handler.go:102`
- kind: stablenet
- calls: [loopcons-02, loopcons-03]
- reads: `events.Chan()`, `timeoutSub`, `finalCommittedSub`, `retryTimeoutSub`
- writes: 라운드 상태
- emits: —
- branches:
  - when: "RequestEvent(제안 블록)" → then: "handleRequest (spine-consensus-rounds rounds-01)" at: `core/handler.go:118`
  - when: "MessageEvent(합의 메시지)" → then: "handleEncodedMsg → 성공 시 Gossip (spine-consensus-rounds)" at: `core/handler.go:128`
  - when: "backlogEvent(보류된 미래 메시지)" → then: "handleDecodedMessage" at: `core/handler.go:136`
  - when: "채널 닫힘" → then: "루프 종료(엔진 정지)" at: `core/handler.go:112`
- invariant: [INV-CONSENSUS-01]
- prose: 코어 루프는 여러 채널을 동시에 지켜본다. 제안(RequestEvent)·합의 메시지(MessageEvent)·보류 메시지(backlog)가 오면 합의 상태 기계(spine-consensus-rounds)로 보낸다. 채널이 닫히면 엔진이 멈춘다.

### STEP loopcons-02
- symbol: `consensus/wbft/core.(*Core).handleTimeoutMsg`
- at: `consensus/wbft/core/handler.go:159` (timeoutSub 수신)
- kind: stablenet
- calls: [spine-consensus-rounds]   # rounds-TO 라운드 체인지로
- reads: `timeoutSub.Chan()`, 타임아웃 취소 여부
- writes: 라운드 증가
- emits: ROUND-CHANGE
- branches:
  - when: "타임아웃이 취소됨(새 라운드 시작됨)" → then: "무시" at: `core/handler.go:157`
  - when: "유효한 라운드 타임아웃" → then: "handleTimeoutMsg → broadcastRoundChange (spine-consensus-rounds rounds-TO)" at: `core/handler.go:159`
  - when: "retryTimeout" → then: "같은 라운드 RoundChange 재전파" at: `core/handler.go:170`
- invariant: [INV-CONSENSUS-03]
- prose: 합의가 시간 내 안 끝나면 라운드 타임아웃이 이 루프를 깨워 라운드 체인지를 시작한다(취소됐으면 무시). 재시도 타임아웃이면 같은 라운드 RoundChange를 다시 뿌린다. 이것이 외부 입력 없이도 합의가 진행되게 하는 liveness 장치다.

### STEP loopcons-03
- symbol: `consensus/wbft/core.(*Core).handleFinalCommittedMsg`
- at: `consensus/wbft/core/handler.go:161` (finalCommittedSub 수신) → `final_committed.go`
- kind: stablenet
- calls: [loopcons-01]   # 다음 시퀀스로
- reads: `finalCommittedSub.Chan()`
- writes: 시퀀스 증가, 새 라운드 시작
- emits: —
- branches:
  - when: "블록 확정 완료 이벤트" → then: "startNewRound(다음 높이로)" at: `core/handler.go:168`
- invariant: —
- prose: 블록이 확정되면 그 이벤트가 루프를 깨워 다음 높이의 새 라운드를 시작한다 — 합의가 끊임없이 이어진다.

---

## prose 렌더 (분기/실패 포함)
> 합의 코어 루프는 제안·합의 메시지·타임아웃·확정완료·재시도 채널을 동시에 지켜본다. 제안과 메시지는
> 합의 상태 기계(spine-consensus-rounds)로 보내 성공 시 재가십하고, 라운드 타임아웃이 오면(취소 안 됐으면)
> 라운드 체인지를 시작하며, 재시도 타임아웃이면 RoundChange를 다시 뿌린다. 블록이 확정되면 다음 높이의
> 새 라운드를 연다. 즉 외부 메시지가 없어도 타이머만으로 합의가 전진하는 독립 시작점이다.
