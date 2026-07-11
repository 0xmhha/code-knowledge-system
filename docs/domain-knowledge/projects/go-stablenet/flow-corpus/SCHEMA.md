# 코퍼스 JSONL 스키마 + cks(vector/graph) 적재 매핑

> `flows/*.md`와 `invariants.md`는 사람이 쓰고 읽는 1차 출처다. 기계 적재용으로는 이들을
> `corpus.jsonl`로 변환한다. 변환은 `tools/build_corpus.py`가 수행하며, flow가 늘거나 바뀌면
> 다시 돌리면 된다(재현 가능). 이 문서는 그 JSONL의 레코드 스키마와 vector/graph DB 매핑을 정의한다.

## 레코드 종류 (JSONL 한 줄 = 한 레코드, `type`로 구분)

### 1) `flow` — entry point flow 메타
```json
{"type":"flow","id":"ep-rpc-sendrawtx","entry_point":"EP-RPC-HTTP",
 "trigger":"...","summary":"...","root_symbol":"node.(*httpServer).ServeHTTP",
 "links":["spine-statetransition","ep-loop-miner"],"called_by":[]}
```

### 2) `step` — flow의 한 실행 단계 (코퍼스 최소 단위)
```json
{"type":"step","id":"rpc-sendrawtx-10","flow":"ep-rpc-sendrawtx",
 "symbol":"core/txpool.ValidateTransactionWithState","file":"core/txpool/validation.go","line":236,
 "kind":"stablenet","calls":["rpc-sendrawtx-11"],
 "reads":"state.GetNonce, state.GetBalance(from), ...","writes":"—","emits":"—",
 "branches":[{"when":"Anzeon이고 발신자가 블랙리스트","then":"ErrBlacklistedAccount","at":"validation.go:250"}, ...],
 "invariants":["INV-SEC-01","INV-TX-02"],
 "prose":"상태 검증. Anzeon이면 ..."}
```

### 3) `invariant` — 안전성 성질
```json
{"type":"invariant","id":"INV-SEC-01","domain":"SEC","statement":"...","assumes":"...",
 "enforced_at":[{"flow":"ep-rpc-sendrawtx","step":"rpc-sendrawtx-10","loc":"validation.go:250/253/281"}, ...],
 "check":"..."}
```

### 4) `edge` — graph DB용 명시적 엣지 (step/invariant 레코드에서 파생)
```json
{"type":"edge","rel":"calls","from":"ep-rpc-sendrawtx:rpc-sendrawtx-10","to":"ep-rpc-sendrawtx:rpc-sendrawtx-11"}
{"type":"edge","rel":"calls_flow","from":"ep-rpc-sendrawtx:rpc-sendrawtx-11","to":"ep-loop-miner"}
{"type":"edge","rel":"enforces","from":"INV-SEC-01","to":"ep-rpc-sendrawtx:rpc-sendrawtx-10"}
```
`rel` 종류: `calls`(step→step, 같은 flow), `calls_flow`(step→다른 flow), `enforces`(invariant→step).

## DB 적재 매핑

### Vector DB (의미 검색)
- 임베딩 청크 = `step.prose` (+ `step.symbol`·`branches[].when`을 메타/본문에 포함하면 실패조건도 검색됨),
  그리고 `invariant.statement`/`assumes`/`check`.
- 메타데이터 = `id`, `flow`, `symbol`, `file:line`, `kind`, `invariants`, `entry_point`.
- 용도: "수수료 위임이 어디서 검증되나?" → prose+symbol 임베딩이 해당 step을 회수.

### Graph DB (영향 분석)
- 노드: `step`(키=`flow:id`, 속성 symbol/file/line/kind), `invariant`(키=INV-id), `flow`(키=flow-id).
- 엣지: `calls`/`calls_flow`(실행 흐름), `enforces`(불변식↔코드), `branches`(조건부 분기/실패 — 노드 속성으로 유지하거나 outcome 노드로 확장 가능).
- cks가 이미 코드에서 추출하는 호출그래프와는 **`symbol`로 조인**한다 — 이 코퍼스가 더하는 것은
  (a) entry-point 단위 그룹핑, (b) prose 설명, (c) 분기/실패 조건, (d) 불변식 링크.
- 용도: "`distributeBaseFee`를 바꾸면?" → `enforces` 역방향으로 INV-LEDGER-02·INV-STATE-02가 위험,
  `calls`/`called_by`로 ep-loop-miner·spine-blockimport(생성·import 양쪽)가 영향받음.

### 벤치마크 정답 (cks 품질 검증)
- 질의 "X 작업의 코드 경로/영향"에 대해, 코퍼스의 `calls`·`enforces` 폐포(closure)가 기대 집합.
- cks의 `impact_analysis`/`get_subgraph` 결과를 이 폐포와 비교해 precision/recall 채점.
- 음성(negative) 케이스 = `branches`의 실패 조건(예: "FeePayer 서명 불일치→거부")을 cks가 아는지.

## 한계 / 주의
- `build_corpus.py`는 사람이 쓴 markdown을 best-effort 파싱한다. 형식을 벗어난 step은 경고로 표시되며
  스킵될 수 있으니, 변환 후 카운트를 확인할 것.
- `symbol`은 정규명을 의도하지만 일부는 약식(`pkg.Func`)이다. cks 조인 시 패키지 경로 정규화가 필요할 수 있다.
- 실제 cks 적재 API/스키마 매핑은 별도 통합 작업이다. 이 JSONL은 그 입력 포맷이며, cks가 코드에서
  뽑는 심볼/호출그래프 위에 얹는 레이어다.
