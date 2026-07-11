---
flow: spine-statetransition
entry_point: SPINE   # 여러 flow가 링크하는 공통 경로 (마이너 실행 / import 재실행 양쪽에서 호출)
trigger: "한 트랜잭션의 실행 — applyTransaction → ApplyMessage → TransitionDb"
root_symbol: "core.(*StateTransition).TransitionDb"
summary: "한 트랜잭션이 잔액을 움직이는 다섯 단계. 결제자 선택·블랙리스트·잔액·실행오류에서 갈린다."
links: [spine-finalize]
called_by: [ep-loop-miner, spine-blockimport]   # 생성·import 모두 같은 코드를 탄다 (INV-STATE-01/02)
---

# Flow: SPINE — 상태 전이 (state transition)

> 마이너의 블록 생성(EP-LOOP-MINER)과 받은 블록의 import 재실행(spine-blockimport)이 **동일하게**
> 타는 경로. 그래서 생성과 검증의 상태 루트가 일치해야 한다(INV-STATE-01, INV-STATE-02).

### STEP statetx-01
- symbol: `core.(*StateTransition).preCheck`
- at: `core/state_transition.go:459` (호출은 `TransitionDb` `:459`)
- kind: geth
- calls: [statetx-02]
- reads: `state.GetNonce(from)`, gas pool, fee 필드, `head.BaseFee`
- writes: —
- emits: —
- branches:
  - when: "논스 불일치" → then: "ErrNonceTooLow/High, 실행 중단" at: `state_transition.go` (preCheck)
  - when: "London인데 gasFeeCap < baseFee" → then: "ErrFeeCapTooLow" at: `state_transition.go:387`
  - when: "blob feeCap < blobBaseFee" → then: "거부" at: `state_transition.go:420`
  - when: "SetCode tx인데 to==nil 또는 auth 비어있음" → then: "ErrSetCodeTxCreate/ErrEmptyAuthList" at: `state_transition.go:426`
- invariant: —
- prose: 실행 전 합의 규칙을 검사한다 — 논스, fee cap이 base fee 이상인지, blob 수수료, SetCode 형식. 통과하면 마지막에 `buyGas`를 호출한다(`:434`).

### STEP statetx-02
- symbol: `core.(*StateTransition).buyGas`
- at: `core/state_transition.go:262`
- kind: stablenet   # 수수료 위임 결제자 선택
- calls: [statetx-03]
- reads: `msg.FeePayer`, `msg.From`, `state.GetBalance(payer)`
- writes: `state.SubBalance(payer, gasLimit*gasPrice)` `:341`
- emits: —
- branches:
  - when: "FeePayer 지정 & ≠ From(위임 tx)" → then: "payer = FeePayer" at: `state_transition.go:263-270`
  - when: "payer 잔액 < gasLimit*gasPrice" → then: "ErrInsufficientFunds(가스), 중단" at: `state_transition.go` (buyGas balance check)
  - when: "gas pool 부족" → then: "ErrGasLimitReached" at: `core/gaspool.go`
- invariant: [INV-LEDGER-03]
- prose: 가스 전액(gasLimit×gasPrice)을 선결제한다. 결제자는 일반 tx면 발신자, 위임 tx면 FeePayer이며, 그 계정 잔액이 모자라면 가스 부족으로 중단한다. 성공하면 결제자 잔액에서 가스값을 뺀다.

### STEP statetx-03
- symbol: `core.(*StateTransition).TransitionDb` (intrinsic gas + 사전 검사)
- at: `core/state_transition.go:478`
- kind: stablenet   # Anzeon 블랙리스트 검사
- calls: [statetx-04]
- reads: `msg.Data`, `msg.Value`, `state.CanTransfer`, `state.IsBlacklisted`
- writes: `st.gasRemaining -= intrinsicGas`
- emits: —
- branches:
  - when: "gasRemaining < intrinsicGas" → then: "ErrIntrinsicGas" at: `state_transition.go:482`
  - when: "value > 0 & !CanTransfer(from)" → then: "ErrInsufficientFundsForTransfer" at: `state_transition.go:492`
  - when: "Shanghai & 컨트랙트 생성 & initcode > MaxInitCodeSize" → then: "ErrMaxInitCodeSizeExceeded" at: `state_transition.go:497`
  - when: "Anzeon & 발신자 블랙리스트" → then: "ErrBlacklistedAccount" at: `state_transition.go:504`
  - when: "Anzeon & 수신자 블랙리스트(이체)" → then: "ErrBlacklistedAccount" at: `state_transition.go:509`
- invariant: [INV-SEC-01]
- prose: intrinsic gas를 빼고, 송금 가능 잔액·initcode 크기를 확인하며, Anzeon이면 발신자·수신자 블랙리스트를 막는다(txpool에서 막혔어도 여기서 한 번 더 — 실행 계층 최종 차단). 통과하면 EVM 실행으로 들어간다.

### STEP statetx-04
- symbol: `core/vm.(*EVM).Call` / `.Create` → `core.Transfer`
- at: `core/state_transition.go:547` / `:524` → `core/evm.go:138`
- kind: geth
- calls: [statetx-05]
- reads: `msg.Data`, `value`, 컨트랙트 코드
- writes: `SubBalance(sender, value)` `evm.go:140`, `AddBalance(recipient, value)` `evm.go:141`; 컨트랙트 스토리지
- emits: 로그(LOG opcode), AddTransferLog
- branches:
  - when: "컨트랙트 생성(to==nil)" → then: "`Create` 경로" at: `state_transition.go:524`
  - when: "value==0" → then: "Transfer 호출 안 함(잔액 변화 없음)" at: `core/evm.go:205`
  - when: "EVM 실행 중 revert/out-of-gas/잘못된 opcode" → then: "vmerr 설정; 상태 롤백되지만 가스는 소비됨(합의엔 영향 없음)" at: `state_transition.go:521`
  - when: "위임 코드(EIP-7702) 대상" → then: "delegation 타깃 warming" at: `state_transition.go:542`
- invariant: [INV-LEDGER-01]
- prose: 송금액 value가 있으면 발신자에서 빼고 수신자에 같은 액을 더해(합 불변) EVM 코드를 실행한다. 컨트랙트 생성이면 Create 경로를 타고, value가 0이면 이체를 건너뛴다. 실행이 revert/out-of-gas로 실패하면 상태 변화는 롤백되지만 소비된 가스는 돌아오지 않으며, 이 vm 에러는 합의에 영향을 주지 않는다(영수증 status만 실패로 기록).

### STEP statetx-05
- symbol: `core.(*StateTransition).refundGas`
- at: `core/state_transition.go:556` → `:663`
- kind: stablenet   # 위임 tx는 FeePayer에게 환급
- calls: [statetx-06]
- reads: `st.gasRemaining`, refund 카운터, `msg.GasPrice`
- writes: `AddBalance(payer, remaining*gasPrice)` `:677/:679`
- emits: —
- branches:
  - when: "London 이후" → then: "환급 상한 gasUsed/5 (EIP-3529)" at: `state_transition.go:556`
  - when: "London 이전" → then: "환급 상한 gasUsed/2" at: `state_transition.go:553`
  - when: "FeePayer 있음" → then: "FeePayer에게 환급" at: `state_transition.go:677`
  - when: "FeePayer 없음" → then: "발신자에게 환급" at: `state_transition.go:679`
- invariant: [INV-LEDGER-03]
- prose: 안 쓴 가스와 EIP-3529 환급분을 ①에서 선결제한 같은 결제자에게 돌려준다(위임이면 FeePayer). 그래서 결제자의 실부담은 "사용 가스×gasPrice"로 정착한다.

### STEP statetx-06
- symbol: `core.(*StateTransition).TransitionDb` (수수료 적립)
- at: `core/state_transition.go:559`
- kind: stablenet   # base fee를 coinbase에 안 줌(finalize에서 재분배)
- calls: [spine-finalize]   # base fee 몫은 블록 단위 finalize에서 처리
- reads: `msg.GasPrice`, `GasTipCap`, `GasFeeCap`, `Context.BaseFee`, `gasUsed`
- writes: `AddBalance(Coinbase, gasUsed*effectiveTip)` `:572`
- emits: AuthorizedTxExecuted 로그(Anzeon authorized 계정 시)
- branches:
  - when: "London" → then: "effectiveTip = min(GasTipCap, GasFeeCap-BaseFee)" at: `state_transition.go:561`
  - when: "NoBaseFee & feeCap==0 & tipCap==0(시뮬레이션)" → then: "수수료 적립 건너뜀" at: `state_transition.go:565`
  - when: "위임 tx & Anzeon & FeePayer 블랙리스트" → then: "ErrBlacklistedAccount" at: `state_transition.go:577`
- invariant: [INV-LEDGER-01, INV-LEDGER-02, INV-SEC-01]
- prose: 실효 팁(=min(tipCap, feeCap−baseFee))만큼 "사용가스×팁"을 coinbase에 더한다. **base fee 몫(사용가스×baseFee)은 여기서 아무에게도 더하지 않고 보류**하며, 블록 finalize(spine-finalize)에서 검증자에게 재분배된다(geth라면 소각, StableNet은 비소각). 시뮬레이션(NoBaseFee)이면 적립을 건너뛴다.

---

## 이 flow가 보존하는 약속 (요약)
- 한 tx의 대차: 결제자 −(사용가스×GP), 발신자 −value, 수신자 +value, coinbase +사용가스×팁,
  (finalize에서) 검증자 +사용가스×BF. `GP=팁+BF`이라 합은 0 — mint/burn 없으면 총공급 보존(INV-LEDGER-01).
- 블랙리스트는 txpool(사전)과 이 실행 계층(최종) **두 곳**에서 막힌다(INV-SEC-01).
- 생성과 import가 이 동일 코드를 타므로 상태 루트가 일치한다(INV-STATE-01/02).

## prose 렌더 (분기/실패 포함)
> `TransitionDb`는 먼저 `preCheck`로 논스·fee cap·blob·SetCode 형식을 보고 어기면 중단한다. 이어
> `buyGas`가 결제자(일반=발신자, 위임=FeePayer)에게서 가스 전액을 선결제하는데 잔액이 모자라면
> 가스 부족으로 끝난다. 그다음 intrinsic gas를 빼고 송금 가능 잔액·initcode 크기를 확인하며,
> Anzeon이면 발신자·수신자 블랙리스트를 막는다(txpool에서 막혔어도 여기서 최종 차단). 통과하면
> `evm.Call`로 실행하는데, value가 있으면 발신자→수신자로 같은 액을 옮기고(합 불변) 코드를
> 실행한다 — revert/out-of-gas면 상태는 롤백되나 가스는 소비되고 합의엔 영향이 없다. 실행 후
> 안 쓴 가스를 같은 결제자에게 환급하고(위임이면 FeePayer, 상한 gasUsed/5), 실효 팁만큼을 coinbase에
> 적립한다. base fee 몫은 여기서 주지 않고 보류해 finalize에서 검증자에게 재분배한다.
