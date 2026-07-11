---
flow: spine-blockimport
entry_point: SPINE
trigger: "블록을 체인에 반영 — InsertChain (네트워크 수신 블록 또는 로컬 확정 블록)"
root_symbol: "core.(*BlockChain).insertChain"
summary: "블록을 헤더검증→본문검증→상태재실행→상태검증→기록 순으로 반영. 각 검증에서 거부된다."
links: [spine-statetransition, spine-finalize]
called_by: [ep-p2p-eth, spine-consensus-rounds, ep-loop-chainsync]   # 네트워크/확정/동기화 블록
---

# Flow: SPINE — 블록 import

> 네트워크로 받은 블록(ep-p2p-eth), 합의로 확정돼 broadcaster가 넣은 블록(spine-consensus-rounds),
> 동기화로 받은 블록(ep-loop-chainsync)이 모두 여기로 온다. **제안자를 믿지 않고 재실행해 검증**한다(INV-STATE-01).

### STEP import-01
- symbol: `core.(*BlockChain).InsertChain` → `insertChain`
- at: `core/blockchain.go:1522` → `:1561`
- kind: geth
- calls: [import-02]
- reads: 블록 묶음
- writes: 체인 락
- emits: —
- branches:
  - when: "블록 묶음이 비연속/순서 오류" → then: "에러 반환" at: `core/blockchain.go` (sanity)
  - when: "이미 아는 블록" → then: "ErrKnownBlock으로 건너뜀" at: `core/blockchain.go:1691`
- invariant: —
- prose: 블록 묶음을 받아 체인 락을 잡고 한 블록씩 import 루프를 돈다. 비연속이거나 이미 아는 블록은 건너뛴다.

### STEP import-02
- symbol: `consensus/wbft/engine.(*Engine).verifyHeader` (via `VerifyHeaders`)
- at: `core/blockchain.go:1585` → `consensus/wbft/engine/engine.go:196`
- kind: stablenet
- calls: [import-03]
- reads: 헤더 필드, valSet, WBFTExtra, prepared/committed seal, parent
- writes: —
- emits: —
- branches:
  - when: "블록 시간이 미래" → then: "ErrFutureBlock(보류/거부)" at: `engine.go:203`
  - when: "uncle 존재(UncleHash≠nil)" → then: "거부(WBFT는 uncle 금지)" at: `engine.go:208`
  - when: "difficulty ≠ WBFTDefaultDifficulty" → then: "거부" at: `engine.go:213`
  - when: "gasLimit > MaxGasLimit" → then: "거부" at: `engine.go:217`
  - when: "Shanghai/Cancun 활성" → then: "거부(WBFT 제약)" at: `engine.go:220`
  - when: "parent 없음 / timestamp < parent+period / baseFee 불일치" → then: "거부" at: `engine.go:256-288`
  - when: "signer가 블랙리스트(상태 있으면)" → then: "거부" at: `engine.go:291`
  - when: "committed seal 수 < QuorumSize" → then: "에러 'lack of seal count'(정족수 미달 블록 차단)" at: `engine.go:1341`
  - when: "BLS 집계 서명 검증 실패" → then: "ErrInvalidSeal" at: `engine.go:1364`
  - when: "RANDAO reveal/mix 불일치" → then: "거부" at: `engine.go:313`
  - when: "verifyGasTip 불일치" → then: "거부" at: `engine.go:329`
- invariant: [INV-CONSENSUS-04]
- prose: 합의 엔진이 헤더를 검증한다 — 미래시간·uncle·난이도·gas 한도·금지 포크·부모/타임스탬프/baseFee·signer 블랙리스트를 보고, 무엇보다 **committed seal이 정족수만큼 있고 BLS 집계 서명이 블록 해시에 유효한지** 확인한다. 정족수 미달이거나 위조 seal이면 전파만으로는 체인에 못 들어온다.

### STEP import-03
- symbol: `core.(*BlockValidator).ValidateBody`
- at: `core/block_validator.go:53`
- kind: geth
- calls: [import-04]
- reads: 블록 본문, 헤더 루트들
- writes: —
- emits: —
- branches:
  - when: "uncle 존재" → then: "거부" at: `block_validator.go:62`
  - when: "tx 루트 ≠ 헤더 TxHash" → then: "거부" at: `block_validator.go:68`
  - when: "withdrawals 루트 불일치(Shanghai)" → then: "거부" at: `block_validator.go:73`
  - when: "blob gas 사용량 불일치(Cancun)" → then: "거부" at: `block_validator.go:87`
  - when: "parent 블록 없음" → then: "거부" at: `block_validator.go:113`
- invariant: —
- prose: 본문 무결성을 본다 — uncle 없음, 트랜잭션 루트가 헤더와 일치, withdrawals/blob 정합. 어기면 거부한다.

### STEP import-04
- symbol: `core.(*StateProcessor).Process`
- at: `core/blockchain.go:1777` → `core/state_processor.go:60`
- kind: stablenet
- calls: [spine-statetransition, spine-finalize]   # 트랜잭션 재실행 + 동일 finalize
- reads: 부모 상태, 블록 트랜잭션
- writes: 상태(재실행 결과), 영수증·로그·usedGas
- emits: —
- branches:
  - when: "어떤 tx의 applyTransaction 실패(가스풀 부족 등)" → then: "전체 import 에러(블록 무효)" at: `state_processor.go:90`
  - when: "Shanghai 전인데 withdrawals 존재" → then: "에러" at: `state_processor.go:98`
- invariant: [INV-STATE-01, INV-LEDGER-01, INV-STATE-02]
- prose: 부모 상태에서 시작해 블록의 모든 트랜잭션을 **생성 때와 똑같은** `applyTransaction`(spine-statetransition)으로 재실행하고, 끝에서 **동일한** `engine.Finalize`(spine-finalize, base fee 재분배 포함)를 적용한다. 한 tx라도 적용 불가하면 블록 전체가 무효가 된다.

### STEP import-05
- symbol: `core.(*BlockValidator).ValidateState`
- at: `core/blockchain.go:1786` → `core/block_validator.go:124`
- kind: geth
- calls: [import-06]
- reads: 재실행 결과(usedGas·영수증·상태), 헤더(GasUsed·ReceiptHash·Bloom·Root)
- writes: —
- emits: —
- branches:
  - when: "block.GasUsed ≠ 재실행 usedGas" → then: "거부" at: `block_validator.go:126`
  - when: "bloom 불일치" → then: "거부" at: `block_validator.go:131`
  - when: "영수증 루트 불일치" → then: "거부" at: `block_validator.go:136`
  - when: "**재실행 상태 루트 ≠ 헤더 Root**" → then: "거부(가장 중요한 결정론 게이트)" at: `block_validator.go:142`
- invariant: [INV-STATE-01]
- prose: 재실행 결과가 헤더와 맞는지 본다 — gas 사용량·bloom·영수증 루트, 그리고 **상태 루트가 헤더의 Root와 일치**하는지. 어긋나면(제안자와 다른 상태 변화) 블록을 거부한다. 이것이 모든 정직 노드를 같은 원장으로 수렴시키는 게이트다.

### STEP import-06
- symbol: `core.(*BlockChain).writeBlockAndSetHead` / `writeBlockWithState`
- at: `core/blockchain.go:1817` → `:1355`
- kind: geth
- calls: []
- reads: 검증 통과 블록·상태
- writes: 상태 트리 디스크 커밋(`state.Commit` `:1377`), 블록·영수증·TD 기록, head 갱신
- emits: ChainEvent / ChainHeadEvent / LogsFeed
- branches:
  - when: "fork choice상 canonical" → then: "head로 설정 + ChainHeadEvent" at: `blockchain.go:1476`
  - when: "side chain(재구성 불필요)" → then: "기록만, head 변경 없음 + ChainSideEvent" at: `blockchain.go:1490`
  - when: "reorg 필요" → then: "체인 재구성 후 head 변경" at: `blockchain.go:1459`
- invariant: —
- prose: 검증을 통과하면 상태 트리를 디스크에 커밋하고 블록·영수증을 기록한다. fork choice가 canonical로 판정하면 head로 올리고 이벤트를 쏘며, side chain이면 기록만, reorg가 필요하면 체인을 재구성한다.

---

## 이 flow가 보존하는 약속 (요약)
- 정족수 미달/위조 seal 블록은 헤더 검증에서 탈락(INV-CONSENSUS-04).
- 받은 블록을 동일 코드로 재실행해 상태 루트가 헤더와 일치할 때만 받아들인다(INV-STATE-01) →
  잘못된 상태 변화는 사전 거름(txpool)이 뚫려도 여기서 최종 탈락.
- 재실행이 생성과 동일 `applyTransaction`+`Finalize`를 타므로 공급 보존도 그대로 유지(INV-LEDGER-01).

## prose 렌더 (분기/실패 포함)
> `InsertChain`은 블록을 한 개씩 반영한다(이미 아는 블록은 건너뜀). 먼저 합의 엔진이 헤더를 검증해
> 미래시간·uncle·난이도·금지 포크·부모/baseFee·블랙리스트를 보고, committed seal이 정족수만큼 있고
> BLS 서명이 유효한지 확인한다 — 미달/위조면 거부. 다음 본문(tx 루트·withdrawals·blob)을 검증하고,
> 부모 상태에서 모든 트랜잭션을 생성 때와 똑같이 재실행한 뒤 동일 finalize를 적용한다(한 tx라도 실패면
> 블록 무효). 그리고 재실행 결과의 gas·bloom·영수증 루트, 무엇보다 **상태 루트가 헤더와 일치**하는지
> 본다 — 어긋나면 거부. 통과하면 상태를 디스크에 커밋하고 블록을 기록하며, canonical이면 head로 올리고
> reorg가 필요하면 재구성한다.
