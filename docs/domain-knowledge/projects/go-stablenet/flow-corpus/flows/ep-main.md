---
flow: ep-main
entry_point: EP-MAIN   # + EP-FILE-CONFIG + EP-FILE-CHAINDATA (기동 중 파일 로드 포함)
trigger: "프로세스 실행 ./gstable (서브커맨드 없음)"
root_symbol: "main.main"
summary: "프로세스 시작부터 P2P·RPC·합의가 모두 켜진 노드까지. 설정 파일·DB·합의엔진 선택에서 갈린다."
links: [ep-loop-consensus, ep-loop-miner, ep-p2p-eth, ep-rpc-sendrawtx]
called_by: []
---

# Flow: EP-MAIN — 기동 (process start)

> 설정 TOML 로드(EP-FILE-CONFIG)와 chaindata 오픈(EP-FILE-CHAINDATA)도 이 경로에 포함된다.

### STEP main-01
- symbol: `main.main`
- at: `cmd/gstable/main.go:271`
- kind: stablenet
- calls: [main-02]
- reads: `os.Args`
- writes: —
- emits: —
- branches:
  - when: "`app.Run` 에러" → then: "stderr 출력 + os.Exit(1)" at: `cmd/gstable/main.go:272`
  - when: "서브커맨드 지정됨(init/account/console/db...)" → then: "해당 커맨드로 분기(이 flow는 무인자=기본동작)" at: `main.go:208`
- invariant: —
- prose: 미리 구성된 CLI 앱에 명령줄 인자를 넘긴다. 서브커맨드가 없으면 기본 동작 `gstable(ctx)`가, 있으면 해당 커맨드가 실행된다. 앱이 에러를 반환하면 종료 코드 1로 끝난다.

### STEP main-02
- symbol: `main.gstable` → `main.makeFullNode` → `main.makeConfigNode` → `main.loadConfig`
- at: `cmd/gstable/main.go:333` → `config.go:172` → `:151` → `:101`
- kind: geth
- calls: [main-03]
- reads: `--config` TOML 파일(있으면), CLI 플래그
- writes: 노드 설정 구조체, 빈 `*node.Node`
- emits: —
- branches:
  - when: "무인자 아님(`gstable` 뒤에 인자)" → then: "invalid command 에러" at: `main.go:335`
  - when: "`--config` 지정됨" → then: "TOML 파일 디코드(EP-FILE-CONFIG)" at: `config.go:139`
  - when: "TOML 디코드 실패" → then: "Fatalf 종료" at: `config.go:108`
  - when: "`node.New` 실패(datadir 락 등)" → then: "Fatalf 종료" at: `config.go:153`
- invariant: —
- prose: 기본값에 `--config` TOML(있으면)을 덮어쓰고, `node.New`로 datadir·DB핸들·P2P서버·계정매니저를 갖춘 빈 노드를 만든다. 설정 파일이 깨졌거나 datadir 락에 실패하면 즉시 종료한다.

### STEP main-03
- symbol: `eth.New`
- at: `cmd/utils/flags.go:1895` → `eth/backend.go:110`
- kind: stablenet
- calls: [main-04]
- reads: chaindata DB(EP-FILE-CHAINDATA), DB의 chain config
- writes: blockchain·txpool·handler·miner·consensus engine 인스턴스
- emits: —
- branches:
  - when: "`OpenDatabaseWithFreezer` 실패" → then: "에러 반환" at: `eth/backend.go:134`
  - when: "chain config 로드 실패" → then: "에러 반환" at: `eth/backend.go:159`
  - when: "`config.AnzeonEnabled()` 참" → then: "WBFT 백엔드 생성(`wbftBackend.New`)" at: `eth/ethconfig/config.go:192`
  - when: "Clique 설정 존재" → then: "Clique 엔진" at: `eth/ethconfig/config.go` (clique)
  - when: "둘 다 아님" → then: "Beacon/ethash 폴백" at: `eth/ethconfig/config.go`
  - when: "`NewBlockChain` 실패(상태 손상 등)" → then: "에러 반환" at: `eth/backend.go:227`
- invariant: [INV-GENESIS-02]
- prose: chaindata DB를 열고 거기 기록된 chain config를 읽는다. `CreateConsensusEngine`이 `AnzeonEnabled()`를 보고 WBFT 백엔드를 골라(BLS 키 파생) 블록체인에 주입하고, txpool·핸들러·마이너를 만들어 eth 서비스를 lifecycle로 등록한다. DB 오픈·상태 로드 실패면 기동이 중단된다.

### STEP main-04
- symbol: `node.(*Node).Start` → `openEndpoints` (p2p + RPC)
- at: `cmd/utils/cmd.go:77` → `node/node.go:161` → `:264`
- kind: geth
- calls: [main-05]
- reads: —
- writes: P2P 리스너, RPC(HTTP/WS/IPC) 리스너
- emits: —
- branches:
  - when: "`p2p server.Start` 실패(포트 점유 등)" → then: "기동 실패" at: `node/node.go:268`
  - when: "`startRPC` 실패" → then: "RPC 정리 + p2p 정지 + 실패" at: `node/node.go:272`
- invariant: —
- prose: P2P 서버를 켜 피어를 받기 시작하고(이후 ep-p2p-*), RPC 스택(HTTP/WS/IPC)을 켜 요청을 받기 시작한다(이후 ep-rpc-*/ep-graphql). 포트 점유 등으로 어느 하나라도 실패하면 정리 후 기동을 멈춘다.

### STEP main-05
- symbol: `eth.(*Ethereum).Start` → `handler.Start`; `wbft backend.(*Backend).Start` → `startWBFT` → `core.Start`
- at: `eth/backend.go:528` → `eth/handler.go:559`; `consensus/wbft/backend/engine.go:241` → `backend/backend.go:355`
- kind: stablenet
- calls: [ep-loop-consensus, ep-loop-miner]
- reads: maxPeers
- writes: 핸들러 고루틴들, WBFT 코어
- emits: —
- branches:
  - when: "WBFT 코어 이미 시작됨" → then: "ErrStartedEngine" at: `backend/engine.go:248`
  - when: "`core.Start` 실패" → then: "에러 반환(합의 미가동)" at: `backend/backend.go:361`
- invariant: —
- prose: eth 서비스가 핸들러 고루틴(tx/블록 브로드캐스트, 동기화, 추적기)을 띄우고, 합의 백엔드가 WBFT 코어를 깨워 합의 이벤트 루프(EP-LOOP-CONSENSUS)와 라운드 타이머를 돌린다. 이 시점부터 노드는 블록을 만들고(EP-LOOP-MINER) 확정할 수 있다.

---

## prose 렌더 (분기/실패 포함)
> `main()`이 CLI 앱을 돌린다. 서브커맨드가 없으면 `gstable`이 노드를 켜는데, `makeFullNode`가 먼저
> `--config` TOML(있으면)을 읽어 빈 노드를 만들고(파일이 깨졌거나 datadir 락 실패면 종료), `eth.New`로
> chaindata DB를 열어 chain config를 읽은 뒤 `AnzeonEnabled`면 WBFT 백엔드를(아니면 Clique/Beacon)
> 골라 블록체인·txpool·핸들러·마이너를 조립한다. 그다음 `node.Start`가 P2P와 RPC를 먼저 열고(포트
> 점유 등 실패 시 정리 후 중단), eth 서비스의 `Start`가 핸들러 고루틴들을, 합의 백엔드가 WBFT 코어를
> 깨워 합의 루프를 돌린다. 이렇게 P2P·RPC·합의가 모두 켜진 노드가 된다.
