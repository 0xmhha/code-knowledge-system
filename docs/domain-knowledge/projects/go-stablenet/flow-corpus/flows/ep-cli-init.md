---
flow: ep-cli-init
entry_point: EP-CLI-INIT   # + EP-FILE-GENESIS
trigger: "gstable init <genesis.json>"
root_symbol: "main.initGenesis"
summary: "제네시스 JSON을 읽어 WBFT 헤더·시스템 컨트랙트가 박힌 0번 블록을 DB에 기록. 검증·주소예약에서 갈린다."
links: []
called_by: []
---

# Flow: EP-CLI-INIT — 제네시스 초기화 (genesis file load)

> 제네시스 JSON 로드(EP-FILE-GENESIS)가 이 경로의 시작이다. 노드 첫 부팅이 빈 DB를 만났을 때도
> step init-04부터(SetupGenesisBlock) 공통으로 탄다.

### STEP init-01
- symbol: `main.initGenesis`
- at: `cmd/gstable/chaincmd.go:191`
- kind: stablenet
- calls: [init-02]
- reads: `genesis.json` 파일(EP-FILE-GENESIS)
- writes: `core.Genesis` 구조체
- emits: —
- branches:
  - when: "경로 인자 없음/파일 열기 실패" → then: "Fatalf 종료" at: `chaincmd.go:199`
  - when: "JSON 디코드 실패" → then: "Fatalf 종료" at: `chaincmd.go:206`
- invariant: —
- prose: 인자로 받은 경로의 파일을 열어 JSON을 `core.Genesis`로 디코드한다. 파일이 없거나 JSON이 깨지면 종료한다.

### STEP init-02
- symbol: `main.initGenesis` (Anzeon 정합성 검사 + 주소 예약)
- at: `cmd/gstable/chaincmd.go:224`
- kind: stablenet
- calls: [init-03]
- reads: `genesis.Config.Anzeon`, `genesis.Alloc`
- writes: —
- emits: —
- branches:
  - when: "AnzeonEnabled & `Anzeon.CheckValidity()` 실패" → then: "거부(검증자/시스템컨트랙트/WBFT 파라미터 누락)" at: `chaincmd.go:224`
  - when: "사용자 alloc에 시스템 컨트랙트 주소(0x1000~0x1004)" → then: "거부(`checkAllocAddress`)" at: `chaincmd.go:228`
- invariant: [INV-GENESIS-01]
- prose: Anzeon 설정이 유효한지(검증자·시스템컨트랙트·WBFT 파라미터) 확인하고, 사용자가 시스템 컨트랙트 예약 주소에 임의 alloc을 넣었으면 거부한다 — 그 자리는 코드가 주입한다.

### STEP init-03
- symbol: `core.SetupGenesisBlockWithOverride`
- at: `cmd/gstable/chaincmd.go:243` → `core/genesis.go:282`
- kind: geth
- calls: [init-04]
- reads: DB 기존 제네시스 여부, override 플래그
- writes: —
- emits: —
- branches:
  - when: "DB에 이미 제네시스 존재 & 호환" → then: "기존 사용(재초기화 안 함)" at: `core/genesis.go:287`
  - when: "DB 제네시스와 불일치" → then: "GenesisMismatch 에러" at: `core/genesis.go`
  - when: "genesis 인자 nil" → then: "기본 StableNet 제네시스 사용" at: `core/genesis.go:289`
- invariant: —
- prose: DB에 제네시스가 이미 있고 호환되면 그대로 쓰고, 충돌하면 mismatch 에러를 낸다. 새 체인이면 초기화로 넘어간다.

### STEP init-04
- symbol: `core.initializeAnzeonGenesis` (WBFT extra + 시스템 컨트랙트 주입)
- at: `core/genesis.go:242` (→ `:247` extra, `:262` inject)
- kind: stablenet
- calls: [init-05]
- reads: `Anzeon` 검증자·BLS키, 임베드 아티팩트(`systemcontracts/contracts.go:37`)
- writes: 헤더 ExtraData, `genesis.Alloc`(시스템 컨트랙트 코드·스토리지)
- emits: —
- branches:
  - when: "!AnzeonEnabled" → then: "건너뜀(일반 제네시스)" at: `core/genesis.go:243`
  - when: "BohoBlock==0(메인넷)" → then: "GovMinter v2 오버레이를 제네시스에 적용(`applyUpgradeOverlay`)" at: `core/genesis.go:760`
  - when: "BohoBlock>0(테스트넷)" → then: "오버레이 미적용(런타임 finalize에서, spine-finalize)" at: `params/config.go:1114`
- invariant: [INV-GENESIS-02, INV-GENESIS-01]
- prose: 초기 검증자·BLS공개키를 RLP로 인코딩해 헤더 ExtraData에 박고(`CreateInitialExtraData`), 임베드된 v1 시스템 컨트랙트 바이트코드·초기 스토리지를 alloc에 주입한다(`InjectContracts`). 메인넷처럼 Boho가 블록 0이면 GovMinter v2 코드 오버레이를 제네시스에 적용하고, 테스트넷이면 런타임 finalize로 미룬다.

### STEP init-05
- symbol: `core.(*Genesis).Commit`
- at: `core/genesis.go:303` → `:575`
- kind: geth
- calls: []
- reads: 완성된 `genesis.Alloc`(시스템 컨트랙트 포함)
- writes: 상태 트리(`flushAlloc`), 블록 0, canonical 해시, **chain config**(`WriteChainConfig` `:603`)
- emits: —
- branches:
  - when: "alloc 해싱/커밋 실패" → then: "에러" at: `core/genesis.go:511`
- invariant: [INV-GENESIS-02]
- prose: 주입된 시스템 컨트랙트를 포함해 alloc을 해싱해 상태 루트를 구하고, WBFT ExtraData를 담은 0번 블록을 만들어 상태·블록·**chain config**를 DB에 영속화한다. 이 chain config 덕분에 다음 부팅(ep-main)에서 Anzeon/Boho 규칙이 블록 0부터 켜진다.

---

## prose 렌더 (분기/실패 포함)
> `gstable init`은 제네시스 JSON을 열어 디코드하고(파일/JSON 오류면 종료), Anzeon 설정 유효성과
> 시스템 컨트랙트 주소 예약을 검사한다(예약 주소에 사용자 alloc이 있으면 거부). `SetupGenesisBlock`은
> DB에 기존 제네시스가 있으면 호환 여부를 보고(불일치면 mismatch 에러), 새 체인이면
> `initializeAnzeonGenesis`로 검증자·BLS키를 헤더 Extra에 박고 v1 시스템 컨트랙트를 alloc에 주입한다 —
> 메인넷은 Boho v2 오버레이를 제네시스에 적용하고 테스트넷은 런타임으로 미룬다. 끝으로 `Commit`이
> alloc을 해싱해 0번 블록을 만들고 상태·블록·chain config를 DB에 쓴다.
