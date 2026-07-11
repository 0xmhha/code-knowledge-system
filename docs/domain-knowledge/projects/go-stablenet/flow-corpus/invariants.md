# go-stablenet 불변식 레이어 (INV-*)

> **이 문서의 역할**
> `flows/*.md`의 각 step은 `invariant` 필드로 자신이 지키는 불변식을 가리킨다. 이 문서는 그
> 반대 방향 — **각 불변식이 어느 step들에서 강제되는가** — 을 모은 역인덱스이자, 코퍼스의
> "안전성 레이어"다. graph DB에서 이 레이어가 코드 노드와 성질을 잇는다: "이 step을 바꾸면 어떤
> 불변식이 위험한가"를 양방향으로 끌어낼 수 있다.
>
> 각 INV는 다음을 갖는다:
> - **statement** — 항상 참인 성질
> - **assumes** — 이 성질이 참이 되기 위한 전제 (전제 없이는 거짓일 수 있다)
> - **enforced_at** — 강제하는 flow step 목록 `flow:step-id (파일:라인)` ← `flows/*.md`와 양방향
> - **defense_layers** — 다층 방어면 어느 계층들에서 중복 강제되는지
> - **check** — 위반 가능성 / 회귀 테스트에서 볼 것
>
> 도메인: CONSENSUS / LEDGER / STATE / TX / SEC / GENESIS / VALIDATOR.

---

## 도메인 A: 합의 안전성 (CONSENSUS)

### INV-CONSENSUS-01 — 정족수 확정
- statement: 블록은 같은 (높이,라운드)에 대한 COMMIT이 `QuorumSize()=ceil(N − (N−1)/3)` 이상일 때만 확정된다.
- assumes: `N≥3f+1`, 비잔틴 ≤ f. COMMIT은 digest·서명 검증을 통과한 것만 집계.
- enforced_at:
  - `spine-consensus-rounds:rounds-04` (`commit.go:123` 정족수 게이트)
  - `spine-consensus-rounds:rounds-03` (`prepare.go:121` PREPARE 정족수 lock)
  - `ep-p2p-consensus:p2pcons-04` (`core/handler.go` 서명 검증 후 집계)
  - `ep-loop-consensus:loopcons-01` (`core/handler.go:102` 메시지 라우팅)
  - 정족수 정의: `validator/default.go:226`
- check: 검증자 수 경계(N=3,6,9 — N이 3의 배수일 때 옛 오기 `ceil(2N/3)`과 값이 1 갈리는 지점)에서 QuorumSize 값, 정족수 미달 시 미확정.

### INV-CONSENSUS-02 — 충돌 블록 비양립 (safety)
- statement: 동일 높이에서 서로 다른 두 블록이 동시에 확정될 수 없다.
- assumes: 정직 검증자는 한 (높이,라운드)에서 한 블록에만 서명. `N≥3f+1`. (두 정족수 합 `>N` → 교집합 ∋ 정직자 → 모순)
- enforced_at:
  - `spine-consensus-rounds:rounds-04` (`commit.go:96` digest 일치 + `:123` 정족수)
  - `ep-p2p-consensus:p2pcons-04` (유효 검증자 메시지만 집계)
- check: 한 검증자가 같은 높이·라운드 두 블록에 서명(equivocation) 시 거부되는지.

### INV-CONSENSUS-03 — 라운드 계승 (round-change safety)
- statement: 라운드 r에서 prepare된 블록 B는, r보다 높은 라운드의 새 제안에서 계승되어야 하며 충돌 블록을 제안할 수 없다.
- assumes: prepare/commit/round-change의 정족수(quorum)는 모두 `QuorumSize()=ceil(N − (N−1)/3)` (`validator/default.go:226`) → 두 정족수 합 >N → 교집합에 정직자 존재. (round-change에는 이와 별개로 낮은 `F+1` catch-up 트리거 `roundchange.go:161`가 있으나, 이는 라운드 조기 전환용이지 정족수가 아니다.)
- enforced_at:
  - `spine-consensus-rounds:rounds-02` (`preprepare.go:135` isJustified, 라운드>0)
  - `spine-consensus-rounds:rounds-03` (`prepare.go:121` preparedRound/Block lock)
  - `spine-consensus-rounds:rounds-TO` (`roundchange.go` highestPrepared 계승 + `justification.go:95`)
  - `ep-loop-consensus:loopcons-02` (`core/handler.go:159` 타임아웃→라운드체인지)
- check: 라운드 변경 시나리오에서 prepared 블록 계승 강제, justification 약화 시 fork 통과 여부.

### INV-CONSENSUS-04 — import 시 seal 재검증
- statement: 정족수만큼의 유효 committed BLS seal이 없는 블록은 체인에 import될 수 없다.
- assumes: 검증자 BLS 공개키가 헤더/에폭 정보에서 정확히 복원됨([INV-GENESIS-02], [INV-VALIDATOR-01]).
- enforced_at:
  - `spine-blockimport:import-02` (`engine.go:1341` seal 수 < quorum 거부, `:1364` BLS 검증)
  - `spine-consensus-rounds:rounds-05` (`backend.go:223` CommitHeader로 seal 박음)
  - `ep-p2p-eth:p2peth-03b` (네트워크 블록 → InsertChain)
  - `ep-loop-chainsync:loopsync-03` (동기화 블록 → InsertChain)
- defense_layers: 생성(seal 부착) + import(seal 검증) + 네트워크/동기화 진입 모두 동일 InsertChain 검증.
- check: seal 부족/위조 블록을 전파/동기화로 주입 시 거부되는지. **import 계층을 노릴 것.**

---

## 도메인 B: 원장 보존 (LEDGER)

### INV-LEDGER-01 — 총공급량 보존 (modulo mint/burn)
- statement: tx·블록 전후로 `Δ(총공급) = Σmint − Σburn`. mint/burn 없으면 0.
- assumes: mint/burn은 governance minter 경로(`native_manager.go:214/246`)로만. base fee 비소각([INV-LEDGER-02]).
- enforced_at:
  - `spine-statetransition:statetx-04` (`evm.go:140-141` 이체 Sub=Add)
  - `spine-statetransition:statetx-06` (`:572` 팁 적립; base fee 보류)
  - `spine-blockimport:import-04` (재실행도 동일 → 공급 보존 유지)
  - `ep-loop-miner:loopminer-02` (생성 시 tx 실행)
- check: tx 전후 전 계정 잔액 합 보존, mint/burn만 합을 바꾸는지.

### INV-LEDGER-02 — base fee 비소각·재분배 (supply-neutral finalize)
- statement: `BaseFee×GasUsed`는 소각되지 않고 검증자(성실도 비례)+coinbase(dust)에 전액 재분배된다.
- assumes: diligenceSum·candidate 인덱스가 에폭 정보와 정합. London 활성.
- enforced_at:
  - `spine-statetransition:statetx-06` (`:572` base fee 몫을 coinbase에 안 줌, 보류)
  - `spine-finalize:finalize-02` (`engine.go:1033` 검증자 분배, `:1045` dust, `:1037` dust<0 거부)
  - `ep-loop-miner:loopminer-03` (FinalizeAndAssemble)
- check: `Σshare + dust == totalBaseFee` 불변, 반올림 누수/dust 부호.

### INV-LEDGER-03 — 가스 결제자 단일성
- statement: 한 tx의 가스는 정확히 한 계정(일반=발신자, 위임=FeePayer)에서 선지불·환급된다.
- assumes: FeePayer 서명 검증됨([INV-TX-01]).
- enforced_at:
  - `spine-statetransition:statetx-02` (`:263-270` payer 선택, `:341` 선지불)
  - `spine-statetransition:statetx-05` (`:677/679` 같은 payer 환급)
- check: 위임 tx에서 선지불=환급 계정 동일성.

---

## 도메인 C: 상태 결정론 (STATE)

### INV-STATE-01 — import 재실행 일치
- statement: 블록 import 시 부모 상태에서 같은 tx·같은 finalize를 재실행한 상태 루트가 헤더 Root와 일치해야 하며, 어긋나면 거부된다.
- assumes: 상태 전이가 결정론적(난수는 봉인된 RANDAO, 시간/난이도 고정). 생성·import가 같은 chain config.
- enforced_at:
  - `spine-blockimport:import-05` (`block_validator.go:142` 상태 루트 비교 — 핵심 게이트)
  - `spine-blockimport:import-04` (`state_processor.go:60` 동일 코드 재실행)
  - `spine-finalize:finalize-03` (`engine.go:967` 상태 루트 계산)
  - `ep-p2p-eth:p2peth-03b`, `ep-loop-chainsync:loopsync-03` (네트워크/동기화 블록도 동일)
  - `ep-p2p-snap:p2psnap-03` (상태 스냅샷 Merkle 증명 검증)
- check: 동일 블록 재실행 시 루트 재현성; 비결정 요소(맵 순회·시계·비봉인 난수) 누출.

### INV-STATE-02 — 생성-검증 finalize 동일성
- statement: 제안자 생성 시 finalize와 import 검증 시 finalize는 동일 함수·동일 결과여야 한다.
- assumes: 에폭 정보·검증자 집합이 양쪽에서 동일([INV-VALIDATOR-01]).
- enforced_at:
  - `spine-finalize:finalize-01` (시스템 컨트랙트 전이, 생성·import 공통 `processFinalize`)
  - `spine-finalize:finalize-03` (상태 루트 계산)
  - `ep-loop-miner:loopminer-03` (생성=FinalizeAndAssemble) ↔ `spine-blockimport:import-04` (import=Finalize)
- check: 하드포크 전이 블록에서 생성·검증 루트 일치.

---

## 도메인 D: 트랜잭션 검증 (TX)

### INV-TX-01 — 수수료 위임 서명 무결성
- statement: 위임 tx는 선언된 FeePayer가 실제 서명한 경우에만 유효하다.
- assumes: 체인 ID가 서명에 묶여 재생 공격 방지.
- enforced_at:
  - `ep-rpc-sendrawtx:rpc-sendrawtx-09` (`validation.go:133` FeePayer 서명 복원·일치, ErrInvalidFeePayer)
- check: 잘못된/누락 FeePayer 서명 거부.

### INV-TX-02 — 수수료 위임 비용 분리
- statement: 위임 tx에서 발신자는 value를, FeePayer는 가스비(FeeCost)를 각각 감당해야 한다.
- assumes: `FeeCost = Cost − Value` 정확.
- enforced_at:
  - `ep-rpc-sendrawtx:rpc-sendrawtx-10` (`validation.go:284/287` 분리 잔액 검사)
  - `ep-loop-txpool:looptxpool-02` (재정렬 시 재평가)
  - 실행 측: `spine-statetransition:statetx-02` (결제자 선택, [INV-LEDGER-03])
- check: 발신자/FeePayer 잔액 경계; 합산 검증으로 회귀 시 정상 위임 tx 거부 여부.

### INV-TX-03 — 하드포크 게이트
- statement: 위임 타입은 Applepie 이후, SetCode 타입은 Anzeon 이후에만 풀에 수용된다.
- assumes: 헤더 번호로 포크 활성 판정 정확.
- enforced_at:
  - `ep-rpc-sendrawtx:rpc-sendrawtx-09` (`validation.go:72` Applepie, `:78` Anzeon)
- check: 활성 블록 경계 직전/직후 타입 수용 여부.

---

## 도메인 E: 보안 — 블랙리스트 (SEC)

### INV-SEC-01 — 블랙리스트 다층 차단
- statement: Anzeon에서 블랙리스트 계정은 발신자·수신자·FeePayer 어느 위치로도 가치를 이동할 수 없다.
- assumes: 블랙리스트 플래그는 state account extra 비트(`state_account_extra.go`)에서 읽힘. Anzeon 활성.
- enforced_at:
  - `ep-rpc-sendrawtx:rpc-sendrawtx-10` (txpool: `validation.go:250/253/281`) — 사전 거름
  - `spine-statetransition:statetx-03` (실행: `state_transition.go:504/509` 발신/수신) — 최종 차단
  - `spine-statetransition:statetx-06` (실행: `:577` FeePayer)
- defense_layers: txpool(사전) + 상태 전이 실행(최종). **txpool 우회 경로(네트워크 직접/위임)는 실행 계층이 막는다.**
- check: txpool만 막고 실행에서 안 막으면 우회 가능 → **실행 계층 차단을 반드시 테스트.**

---

## 도메인 F: 제네시스·검증자 (GENESIS / VALIDATOR)

### INV-GENESIS-01 — 시스템 컨트랙트 주소 예약
- statement: 사용자는 제네시스 alloc에서 시스템 컨트랙트 주소(0x1000~0x1004)에 임의 상태를 넣을 수 없다.
- assumes: 시스템 컨트랙트 주소 목록이 설정과 일치.
- enforced_at:
  - `ep-cli-init:init-02` (`chaincmd.go:228` checkAllocAddress)
  - `ep-cli-init:init-04` (`genesis.go:262` InjectContracts가 코드 주입)
  - `spine-finalize:finalize-01` (런타임 업그레이드 시 코드 교체도 동일 권한 경로)
- check: 예약 주소 alloc 거부.

### INV-GENESIS-02 — 제네시스가 검증자 집합을 봉인
- statement: 0번 블록 헤더 WBFT Extra에 초기 검증자·BLS 공개키가 인코딩되고, 상태 루트는 주입된 시스템 컨트랙트를 포함한다.
- assumes: 설정 validators/BLSPublicKeys 길이·정합 검증(`Anzeon.CheckValidity`).
- enforced_at:
  - `ep-cli-init:init-04` (`genesis.go:247` CreateInitialExtraData)
  - `ep-cli-init:init-05` (`genesis.go:511` hashAlloc — 시스템 컨트랙트 포함 상태 루트)
  - `ep-main:main-03` (부팅 시 chain config/검증자 로드)
- check: 제네시스 Extra ↔ `istanbul_getValidators(0)` 일치 → 이게 어긋나면 [INV-CONSENSUS-04]가 깨짐.

### INV-VALIDATOR-01 — 검증자 집합은 에폭 경계에서만 전이
- statement: 활성 검증자 집합은 에폭 경계 블록의 finalize에서만 갱신되고 에폭 내에서는 고정이다.
- assumes: 에폭 길이·경계 판정이 생성·검증에서 동일.
- enforced_at:
  - `spine-finalize:finalize-03` (`engine.go:948` 에폭 블록에서 다음 검증자 기록)
  - 검증: `spine-blockimport:import-04` (동일 finalize 재실행, [INV-STATE-02])
- check: 에폭 중간 집합 변경 시 정족수 기준 N이 흔들림([INV-CONSENSUS-01]) → 에폭 경계에서만 변경되는지.

---

## 부록: 다층 방어 불변식 (회귀 테스트 우선순위)
검색·생성 품질과 무관하게 항상 켜져 있어야 하는 backstop. 한 계층만 보고 "충분"이라 판단하면 안 된다.

| 불변식 | 계층 1 (사전) | 계층 2 (최종) |
|--------|---------------|----------------|
| INV-SEC-01 (블랙리스트) | txpool `validation.go` | **실행 `state_transition.go`** |
| INV-CONSENSUS-04 (seal) | 생성 시 부착 | **import `engine.go` 재검증** |
| INV-STATE-01 (결정론) | — | **import 상태 루트 비교** |
| INV-LEDGER-02 (base fee) | — | **finalize dust<0 거부** |

> 회귀 테스트는 *계층 2(굵게)* 를 노려야 한다 — 계층 1(사전 거름)을 우회하는 입력 경로
> (네트워크 직접 주입·동기화·위임)가 실제 공격면이기 때문이다.
