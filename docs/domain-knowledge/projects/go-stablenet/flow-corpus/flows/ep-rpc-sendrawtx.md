---
flow: ep-rpc-sendrawtx
entry_point: EP-RPC-HTTP   # (WS/IPC 도 transport만 다르고 step 2부터 동일)
trigger: "JSON-RPC 요청 {method: eth_sendRawTransaction, params:[rawTxHex]}"
root_symbol: "node.(*httpServer).ServeHTTP"
summary: "서명된 원시 트랜잭션이 RPC로 들어와 디코드·검증을 거쳐 txpool에 적재되기까지. 검증 단계마다 거부 조건이 있다."
links: [spine-statetransition, ep-loop-miner]   # 이 flow 끝에서 이어지는 경로
---

# Flow: EP-RPC-HTTP — eth_sendRawTransaction

> 형식 견본. 각 `### STEP`은 코퍼스의 최소 단위이며, 필드는 `README.md`의 unit 형식을 따른다.
> `branches`는 그 step에서 검사하는 조건(`when`)과 결과(`then`)·위치(`at`)를 적는다 —
> happy path뿐 아니라 **언제 어떻게 거부/분기되는지**를 담는다.

### STEP rpc-sendrawtx-01
- symbol: `node.(*httpServer).ServeHTTP`
- at: `node/rpcstack.go:198`
- kind: geth
- calls: [rpc-sendrawtx-02]
- reads: HTTP 요청 본문
- writes: —
- emits: —
- branches:
  - when: "요청이 WebSocket 업그레이드" → then: "WS 핸들러로 분기(이후 step 동일)" at: `node/rpcstack.go:201`
  - when: "경로가 설정된 RPC prefix와 불일치" → then: "404, 처리 안 함" at: `node/rpcstack.go:209`
  - when: "HTTP body가 max 크기 초과" → then: "413/거부" at: `node/rpcstack.go` (size 제한)
- invariant: —
- prose: HTTP 요청을 받아, WebSocket 업그레이드면 WS 핸들러로, 경로가 RPC prefix와 맞으면 HTTP RPC 핸들러로 보낸다. prefix 불일치나 과대 본문이면 여기서 거부된다.

### STEP rpc-sendrawtx-02
- symbol: `rpc.(*Server).serveSingleRequest`
- at: `rpc/server.go:142`
- kind: geth
- calls: [rpc-sendrawtx-03]
- reads: JSON-RPC 메시지(배치)
- writes: —
- emits: —
- branches:
  - when: "JSON 파싱 실패(`codec.readBatch` 오류)" → then: "-32700 parse error 응답, 종료" at: `rpc/server.go:152`
  - when: "배치 메시지" → then: "`handleBatch`로 분기(여기선 단건 가정)" at: `rpc/server.go:155`
  - when: "배치 항목 수가 batchItemLimit 초과" → then: "거부" at: `rpc/handler.go` (batch limit)
- invariant: —
- prose: 요청 JSON을 읽어 단건/배치를 가른다. JSON이 깨졌으면 parse error로 끝나고, 배치면 batch 처리로 분기하며, 배치 한도를 넘으면 거부한다.

### STEP rpc-sendrawtx-03
- symbol: `rpc.(*handler).handleCall`
- at: `rpc/handler.go:492`
- kind: geth
- calls: [rpc-sendrawtx-04]
- reads: `msg.Method` ("eth_sendRawTransaction"), `msg.Params`
- writes: —
- emits: —
- branches:
  - when: "콜백을 못 찾음(`callb == nil`)" → then: "-32601 methodNotFound 응답" at: `rpc/handler.go:501`
  - when: "인자 파싱 실패(`parsePositionalArguments`)" → then: "-32602 invalidParams 응답" at: `rpc/handler.go:507`
- invariant: —
- prose: 메서드 이름으로 콜백을 찾고 파라미터를 파싱한다. 메서드가 없으면 methodNotFound, 인자가 안 맞으면 invalidParams로 거부한다.

### STEP rpc-sendrawtx-04
- symbol: `rpc.(*serviceRegistry).callback`
- at: `rpc/service.go:95`
- kind: geth
- calls: [rpc-sendrawtx-05]
- reads: 메서드명 → ("eth","sendRawTransaction") 분해
- writes: —
- emits: —
- branches:
  - when: "메서드명에 `_` 구분자 없음" → then: "nil 반환 → 호출부에서 methodNotFound" at: `rpc/service.go:97`
  - when: "네임스페이스가 등록 안 됨(모듈 미허용)" → then: "nil 반환" at: `rpc/service.go:102`
- invariant: —
- prose: "eth_sendRawTransaction"을 `_`로 잘라 해당 네임스페이스/메서드 콜백을 돌려준다. 구분자가 없거나 네임스페이스가 등록/허용되지 않았으면 nil이라 methodNotFound가 된다.

### STEP rpc-sendrawtx-05
- symbol: `rpc.(*callback).call`
- at: `rpc/service.go:183`
- kind: geth
- calls: [rpc-sendrawtx-06]
- reads: 리시버·컨텍스트·파싱된 인자
- writes: —
- emits: —
- branches:
  - when: "메서드 내부 panic" → then: "recover 후 internal error 응답" at: `rpc/service.go` (crit recover)
- invariant: —
- prose: 리플렉션으로 `TransactionAPI.SendRawTransaction`을 호출한다. 메서드가 panic하면 recover되어 internal error로 응답한다.

### STEP rpc-sendrawtx-06
- symbol: `internal/ethapi.(*TransactionAPI).SendRawTransaction`
- at: `internal/ethapi/api.go:2015`
- kind: geth
- calls: [rpc-sendrawtx-07]
- reads: 원시 트랜잭션 바이트(input)
- writes: —
- emits: —
- branches:
  - when: "`tx.UnmarshalBinary(input)` 실패(RLP/타입 오류)" → then: "에러 반환, txpool 진입 안 함" at: `internal/ethapi/api.go:2018`
- invariant: —
- prose: 입력 바이트를 `types.Transaction`으로 언마샬한다. 바이트가 깨졌거나 알 수 없는 타입이면 여기서 에러로 끝나고 txpool에 닿지 않는다. 성공하면 `SubmitTransaction`을 호출한다.

### STEP rpc-sendrawtx-07
- symbol: `internal/ethapi.SubmitTransaction` → `eth.(*EthAPIBackend).SendTx`
- at: `internal/ethapi/api.go:2022` → `eth/api_backend.go`
- kind: geth
- calls: [rpc-sendrawtx-08]
- reads: 트랜잭션 객체
- writes: —
- emits: —
- branches:
  - when: "`SendTx`(=txpool.Add)가 에러 반환" → then: "그 에러를 RPC 응답으로 전파(아래 step들의 거부가 여기로 올라옴)" at: `internal/ethapi/api.go` (SubmitTransaction)
  - when: "성공" → then: "tx 해시를 응답으로 반환" at: `internal/ethapi/api.go`
- invariant: —
- prose: 제출 헬퍼가 백엔드의 `SendTx`를 거쳐 트랜잭션을 txpool로 보낸다. 아래 검증 step들의 거부 에러는 모두 이 경로로 되올라와 RPC 에러 응답이 되고, 성공하면 tx 해시가 반환된다.

### STEP rpc-sendrawtx-08
- symbol: `core/txpool.(*TxPool).Add`
- at: `core/txpool/txpool.go:250`
- kind: stablenet   # legacypool.Filter가 FeeDelegateDynamicFeeTxType를 허용하도록 확장됨
- calls: [rpc-sendrawtx-09]
- reads: `tx.Type()`
- writes: 서브풀 입력 버퍼
- emits: —
- branches:
  - when: "어떤 서브풀의 `Filter`도 통과 못 함(미지원 타입)" → then: "tx 드롭(해당 서브풀 없음)" at: `core/txpool/txpool.go:266`
  - when: "타입이 blob tx" → then: "blobpool로 분기(이 flow는 legacypool 가정)" at: `core/txpool/txpool.go:266`
- invariant: —
- prose: 타입별 `subpool.Filter`로 알맞은 서브풀을 고른다. 수수료 위임 타입(0x16)·SetCode 타입은 legacypool이 받고(`legacypool.go:313`, geth에 없는 허용), blob은 blobpool로 가며, 어느 풀도 안 받는 타입이면 드롭된다.

### STEP rpc-sendrawtx-09
- symbol: `core/txpool.ValidateTransaction`
- at: `core/txpool/validation.go:49`
- kind: stablenet   # FeePayer 서명 검증 + Applepie/Anzeon 게이트가 추가됨
- calls: [rpc-sendrawtx-10]
- reads: tx 필드(타입·크기·서명·fee cap·gas), `head.Number`, `head.GasLimit`, `opts.MinTip`
- writes: —
- emits: —
- branches:
  - when: "타입이 풀 Accept 비트에 없음" → then: "ErrTxTypeNotSupported" at: `validation.go:57`
  - when: "`tx.Size() > MaxSize`(256KB)" → then: "ErrOversizedData" at: `validation.go:62`
  - when: "수수료 위임 타입인데 `!IsApplepie`" → then: "거부(포크 미도달)" at: `validation.go:72`
  - when: "SetCode 타입인데 `!AnzeonEnabled`" → then: "거부(포크 미도달)" at: `validation.go:78`
  - when: "`tx.Value() < 0`" → then: "ErrNegativeValue" at: `validation.go:87`
  - when: "`head.GasLimit < tx.Gas()`" → then: "ErrGasLimit" at: `validation.go:91`
  - when: "feeCap/tipCap 비트 길이 > 256" → then: "ErrFeeCapVeryHigh / ErrTipVeryHigh" at: `validation.go:95`
  - when: "`gasFeeCap < gasTipCap`" → then: "ErrTipAboveFeeCap" at: `validation.go:102`
  - when: "발신자 서명 복원 실패" → then: "ErrInvalidSender" at: `validation.go:106`
  - when: "`tx.Gas() < intrinsicGas`" → then: "ErrIntrinsicGas" at: `validation.go:111`
  - when: "`gasTipCap < MinTip`(원격 tx)" → then: "ErrUnderpriced" at: `validation.go:119`
  - when: "Anzeon+London인데 `gasFeeCap < minBaseFee+minTip`" → then: "ErrUnderpriced" at: `validation.go:122`
  - when: "위임 tx인데 FeePayer가 nil이거나 서명 복원 주소 불일치" → then: "ErrInvalidFeePayer" at: `validation.go:133`
  - when: "SetCode tx인데 authorization 0개" → then: "거부" at: `validation.go:144`
- invariant: [INV-TX-01, INV-TX-03]
- prose: 무상태 검증. 타입 허용·크기(256KB)·값 음수·블록 gas 한도·fee cap 비트·`feeCap≥tipCap`·발신자 서명·intrinsic gas·최소 tip·(Anzeon)최소 base fee+tip을 차례로 보고 하나라도 어기면 해당 에러로 거부한다. 수수료 위임 타입은 Applepie 이후만 허용하며 FeePayer 서명을 복원·일치 검사하고(불일치 ErrInvalidFeePayer), SetCode는 Anzeon 이후 + authorization 1개 이상이어야 한다.

### STEP rpc-sendrawtx-10
- symbol: `core/txpool.ValidateTransactionWithState`
- at: `core/txpool/validation.go:236`
- kind: stablenet   # 블랙리스트 + 위임 비용 분리 검증
- calls: [rpc-sendrawtx-11]
- reads: `state.GetNonce`, `state.GetBalance(from)`, `state.GetBalance(feePayer)`, `state.IsBlacklisted`
- writes: —
- emits: —
- branches:
  - when: "Anzeon이고 발신자가 블랙리스트" → then: "ErrBlacklistedAccount" at: `validation.go:250`
  - when: "Anzeon이고 수신자(To)가 블랙리스트" → then: "ErrBlacklistedAccount" at: `validation.go:253`
  - when: "`state nonce > tx.Nonce`" → then: "ErrNonceTooLow" at: `validation.go:259`
  - when: "논스 갭이 허용 범위 초과" → then: "ErrNonceTooHigh" at: `validation.go:265`
  - when: "위임 tx이고 Anzeon이며 FeePayer가 블랙리스트" → then: "ErrBlacklistedAccount" at: `validation.go:281`
  - when: "위임 tx인데 발신자 잔액 < value" → then: "ErrSenderInsufficientFunds" at: `validation.go:284`
  - when: "위임 tx인데 FeePayer 잔액 < feeCost" → then: "ErrFeePayerInsufficientFunds" at: `validation.go:287`
  - when: "일반 tx인데 잔액 < cost(value+gas)" → then: "ErrInsufficientFunds" at: `validation.go:291`
  - when: "계정 슬롯 한도 초과(EIP-7702)" → then: "ErrAccountLimitExceeded" at: `validation.go:299`
- invariant: [INV-SEC-01, INV-TX-02]
- prose: 상태 검증. Anzeon이면 발신자·수신자·(위임)FeePayer 블랙리스트를 먼저 막고, 논스가 너무 낮거나(이미 쓴 논스) 갭이 크면 거부한다. 잔액은 종류별로 갈려, 일반 tx는 `잔액≥value+가스`를, 위임 tx는 발신자 `잔액≥value`·FeePayer `잔액≥가스비`를 각각 확인하고 못 미치면 해당 부족 에러로 거부한다.

### STEP rpc-sendrawtx-11
- symbol: `core/txpool/legacypool.(*LegacyPool).add` → `enqueueTx` / `txFeed.Send`
- at: `core/txpool/legacypool/legacypool.go:774` → `:1440`
- kind: geth
- calls: [ep-loop-miner]   # cross-flow: 마이너가 NewTxsEvent를 구독해 블록을 만든다
- reads: pending/queue 상태, 풀 용량, `priced`
- writes: pending/queue 리스트, `all` 맵, journal
- emits: `core.NewTxsEvent`
- branches:
  - when: "이미 아는 해시" → then: "ErrAlreadyKnown" at: `legacypool.go:777`
  - when: "계정 첫 tx인데 `reserver.Hold` 실패" → then: "거부(다른 서브풀이 점유)" at: `legacypool.go:807`
  - when: "풀이 가득 + 원격 tx가 underpriced" → then: "ErrUnderpriced" at: `legacypool.go:819`
  - when: "풀이 가득" → then: "저가 tx 축출(eviction) 후 삽입 시도" at: `legacypool.go:818`
  - when: "같은 논스 교체인데 가격 인상 부족" → then: "ErrReplaceUnderpriced" at: `legacypool.go:884`
  - when: "논스가 당장 실행 불가" → then: "queued로 보류(pending 아님)" at: `legacypool.go:904`
- invariant: —
- prose: 검증을 통과한 트랜잭션을 삽입한다. 이미 알거나, 계정 점유 충돌이거나, 풀이 가득 차 underpriced면 거부/축출되고, 같은 논스 교체는 가격을 충분히 올려야 한다. 논스가 당장 실행 가능하면 pending, 아니면 queued로 들어가고, pending이 되면 `NewTxsEvent`를 브로드캐스트해 마이너 워크 루프(EP-LOOP-MINER)가 이어받는다.

---

## 이 flow가 보존하는 약속 (요약)
- 서명·크기·fee cap·잔액·블랙리스트·FeePayer·논스·풀용량 — 이 중 하나라도 어기면 트랜잭션은
  txpool에 들어가지 못하고 그 거부 에러가 RPC 응답으로 되올라간다. 단 이는 *사전 거름*이며 최종
  보증은 `spine-statetransition`의 실행 단계가 한다(다층 방어, INV-SEC-01).
- 이 flow의 끝(step 11)은 EP-LOOP-MINER로 이어지고, 거기서 트랜잭션이 실행되어 잔액을 움직인다
  (`spine-statetransition`, INV-LEDGER-*).

## prose 렌더 (사람 읽기용 — 분기/실패 조건 포함)
> HTTP로 들어온 RPC 요청을 `ServeHTTP`가 받는다. WebSocket이면 WS 핸들러로 분기하고, 경로가 RPC
> prefix와 안 맞거나 본문이 너무 크면 여기서 거부된다. 통과하면 `serveSingleRequest`가 JSON을 읽어
> — 깨졌으면 parse error, 배치면 batch 처리로 갈라 — `handleCall`로 넘긴다. `handleCall`은 메서드명을
> `callback`으로 찾는데, "eth_sendRawTransaction"을 `_`로 쪼개 네임스페이스/메서드를 얻으며,
> 메서드가 없거나 네임스페이스가 허용 안 됐으면 methodNotFound, 인자가 안 맞으면 invalidParams로
> 거부한다. 콜백을 찾으면 리플렉션으로 `SendRawTransaction`을 호출한다(메서드가 panic하면 recover되어
> internal error).
>
> `SendRawTransaction`은 바이트를 트랜잭션으로 언마샬하는데, RLP가 깨졌거나 알 수 없는 타입이면
> 여기서 끝나고 txpool에 닿지 않는다. 성공하면 `SubmitTransaction`→`SendTx`로 txpool에 보내고,
> 이후 검증 거부는 모두 이 경로로 되올라와 RPC 에러가 된다.
>
> `TxPool.Add`는 타입으로 서브풀을 고른다 — 위임·SetCode는 legacypool, blob은 blobpool, 어느 풀도
> 안 받으면 드롭. legacypool의 무상태 검증(`ValidateTransaction`)은 타입 허용·크기(256KB)·값 음수·
> 블록 gas 한도·fee cap 비트·`feeCap≥tipCap`·발신자 서명·intrinsic gas·최소 tip·(Anzeon)최소
> base fee+tip을 차례로 보고 하나라도 어기면 그에 맞는 에러로 거부하며, 위임 타입은 Applepie 이후만
> 허용하고 FeePayer 서명이 일치해야 하고(아니면 ErrInvalidFeePayer), SetCode는 Anzeon 이후 +
> authorization이 있어야 한다. 이어 상태 검증(`ValidateTransactionWithState`)은 Anzeon이면 발신자·
> 수신자·FeePayer 블랙리스트를 막고, 논스가 낮거나 갭이 크면 거부하며, 잔액은 일반 tx면 `value+가스`를
> 한 계정에서, 위임 tx면 발신자 `value`·FeePayer `가스비`로 나눠 확인해 부족하면 거부한다.
>
> 검증을 통과해도 삽입 단계(`add`)에서 다시 갈린다 — 이미 아는 해시면 ErrAlreadyKnown, 계정 점유
> 충돌이면 거부, 풀이 가득 차면 저가 tx를 축출하거나 underpriced로 거부하고, 같은 논스 교체는 가격을
> 충분히 올려야 한다. 최종적으로 논스가 실행 가능하면 pending, 아니면 queued에 들어가고, pending이
> 되면 `NewTxsEvent`를 쏘아 마이너가 블록 생성으로 이어받는다.
