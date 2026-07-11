---
flow: spine-finalize
entry_point: SPINE
trigger: "블록의 모든 tx 실행 후 블록 마감 — engine.Finalize / FinalizeAndAssemble"
root_symbol: "consensus/wbft/engine.(*Engine).processFinalize"
summary: "블록 단위 마감 — 시스템 컨트랙트 전이, base fee 검증자 재분배, 에폭 기록, 상태 루트 확정. 데이터 정합성에서 갈린다."
links: []
called_by: [ep-loop-miner, spine-blockimport]   # 생성=FinalizeAndAssemble, import=Finalize 둘 다 여기로 수렴
---

# Flow: SPINE — finalize (블록 마감)

> 마이너의 `FinalizeAndAssemble`(`engine/engine.go:1053`)와 import의 `Finalize`(`:919`, state_processor에서
> 호출)가 **동일한** `processFinalize`로 수렴한다 — 그래서 생성·검증 상태 루트가 일치한다(INV-STATE-02).

### STEP finalize-01
- symbol: `consensus/wbft/engine.(*Engine).processFinalize` (시스템 컨트랙트 전이)
- at: `consensus/wbft/engine/engine.go:929`
- kind: stablenet
- calls: [finalize-02]
- reads: chain config 업그레이드 목록, 현재 블록 번호
- writes: 시스템 컨트랙트 코드/스토리지(하드포크 활성 블록에서)
- emits: —
- branches:
  - when: "이 블록 번호에 활성화되는 업그레이드 있음(예: Boho)" → then: "GovMinter 등 코드 교체 적용" at: `engine.go:930`
  - when: "업그레이드 없음" → then: "건너뜀" at: `engine.go:930`
- invariant: [INV-STATE-02, INV-GENESIS-01]
- prose: 이 블록 번호에 하드포크 업그레이드가 걸려 있으면(예: Boho에서 GovMinter v1→v2) 시스템 컨트랙트 코드를 교체한다(잔액·스토리지는 보존). 없으면 건너뛴다.

### STEP finalize-02
- symbol: `consensus/wbft/engine.(*Engine).distributeBaseFee`
- at: `consensus/wbft/engine/engine.go:972` (호출 조건 `:941` London)
- kind: stablenet   # base fee 소각 대신 검증자 재분배
- calls: [finalize-03]
- reads: `header.BaseFee`, `header.GasUsed`, 에폭 정보(검증자·candidate·diligence)
- writes: `AddBalance(validator, share)` `:1033`, `AddBalance(coinbase, dust)` `:1045`
- emits: —
- branches:
  - when: "gasUsed==0 또는 블록 0" → then: "분배 건너뜀(분배할 base fee 없음)" at: `engine.go:973`
  - when: "header.BaseFee == nil" → then: "에러 반환(블록 무효)" at: `engine.go:976`
  - when: "validator candidate 인덱스 범위 초과" → then: "에러(블록 무효)" at: `engine.go:994`
  - when: "diligenceSum == 0" → then: "검증자 분배 0, 전액 dust로 coinbase" at: `engine.go:1008`
  - when: "share가 uint256 오버플로" → then: "에러" at: `engine.go:1030`
  - when: "분배 후 dust < 0" → then: "에러(분배 합 > 총액, 블록 무효)" at: `engine.go:1037`
- invariant: [INV-LEDGER-02]
- prose: 이 블록이 거둔 `baseFee×gasUsed`를 검증자들에게 성실도(diligence) 비례로 나눠 주고(`:1033`) 나누어떨어지지 않은 잔돈은 coinbase에 준다(`:1045`). base fee가 없거나 candidate 인덱스가 어긋나거나 분배 합이 총액을 넘어 dust가 음수가 되면 블록을 무효로 만든다. **소각하지 않으므로 공급은 보존**된다(`Σshare+dust==totalBaseFee`).

### STEP finalize-03
- symbol: `consensus/wbft/engine.(*Engine).processFinalize` (에폭 기록 + gasTip + 상태 루트)
- at: `consensus/wbft/engine/engine.go:948`
- kind: stablenet
- calls: []
- reads: 에폭 경계 여부, GovValidator gasTip
- writes: 다음 에폭 검증자 집합 기록(에폭 블록), `header.Root = state.IntermediateRoot(...)` `:967`
- emits: —
- branches:
  - when: "에폭 경계 블록" → then: "다음 에폭 검증자 목록 기록(writeEpoch)" at: `engine.go:948`
  - when: "에폭 중간 블록" → then: "검증자 집합 불변" at: `engine.go:948`
  - when: "verifyGasTip 불일치" → then: "에러(블록 무효)" at: `engine.go:963`
- invariant: [INV-VALIDATOR-01, INV-STATE-01, INV-STATE-02]
- prose: 에폭 경계면 다음 에폭의 검증자 집합을 기록하고(에폭 중간엔 불변), 가스팁이 GovValidator 컨트랙트와 맞는지 확인한 뒤, 모든 마감 상태 변화를 반영한 **상태 루트를 계산해 헤더에 박는다**(`:967`). 이 루트가 import 검증의 기준이 된다.

---

## 이 flow가 보존하는 약속 (요약)
- base fee는 소각되지 않고 검증자+coinbase에 전액 재분배되어 공급이 보존된다(INV-LEDGER-02);
  분배 합 검사(`dust<0` 거부)가 누수를 막는다.
- 검증자 집합은 에폭 경계에서만 바뀐다(INV-VALIDATOR-01) → 정족수 기준 N이 에폭 내 고정(INV-CONSENSUS-01).
- 생성·import가 동일 `processFinalize`를 타 상태 루트가 일치한다(INV-STATE-02).

## prose 렌더 (분기/실패 포함)
> 블록의 모든 트랜잭션 실행이 끝나면 `processFinalize`가 마감한다. 먼저 이 블록 번호에 걸린
> 하드포크가 있으면 시스템 컨트랙트 코드를 교체하고(없으면 건너뜀), London이면 `distributeBaseFee`가
> `baseFee×gasUsed`를 검증자에게 성실도 비례로 분배하고 잔돈은 coinbase에 준다 — base fee가 nil이거나
> candidate 인덱스가 범위를 벗어나거나 분배 합이 총액을 넘어 dust가 음수가 되면 블록을 무효로 만든다.
> 이어 에폭 경계면 다음 검증자 집합을 기록하고(중간 블록은 불변), 가스팁 정합을 확인한 뒤 마감
> 상태의 루트를 계산해 헤더에 박는다. 블록 0이나 gasUsed 0이면 분배 자체를 건너뛴다.
