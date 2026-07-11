---
flow: ep-graphql
entry_point: EP-GRAPHQL
trigger: "HTTP POST /graphql {query, variables}"
root_symbol: "graphql.handler.ServeHTTP"
summary: "GraphQL 질의를 파싱·실행해 같은 백엔드(상태·블록)를 읽어 응답. 파싱·타임아웃에서 갈린다. 읽기 전용."
links: []
called_by: []
---

# Flow: EP-GRAPHQL — GraphQL 읽기 질의 (REST에 가장 가까운 면)

> 상태를 바꾸지 않는 읽기 창구. 결국 `ethapi.Backend`(ep-rpc-*와 같은 백엔드)로 위임한다.

### STEP graphql-01
- symbol: `graphql.New` → `RegisterHandler("/graphql")`
- at: `graphql/service.go:109` → `:128`
- kind: geth
- calls: [graphql-02]
- reads: 스키마 정의
- writes: `/graphql` HTTP 핸들러 등록
- emits: —
- branches:
  - when: "스키마 파싱 실패" → then: "기동 시 에러" at: `graphql/service.go` (ParseSchema)
  - when: "GraphQL 비활성(미설정)" → then: "엔드포인트 미등록" at: `node/node.go` (graphql opt-in)
- invariant: —
- prose: 기동 시 스키마를 파싱해 `/graphql` 경로에 핸들러를 등록한다(설정으로 켰을 때만). 스키마가 깨지면 기동 에러.

### STEP graphql-02
- symbol: `graphql.handler.ServeHTTP`
- at: `graphql/service.go:35`
- kind: geth
- calls: [graphql-03]
- reads: POST 본문(`query`, `operationName`, `variables`)
- writes: —
- emits: —
- branches:
  - when: "본문 JSON 디코드 실패" → then: "400 Bad Request" at: `graphql/service.go` (Decode)
  - when: "요청 컨텍스트 타임아웃" → then: "타임아웃 응답" at: `graphql/service.go`
- invariant: —
- prose: POST 본문에서 질의·변수를 꺼낸다. JSON이 깨지면 400, 시간 초과면 타임아웃으로 응답한다.

### STEP graphql-03
- symbol: `Schema.Exec` → `graphql.Resolver` 메서드 → `ethapi.Backend`
- at: `graphql/service.go:90` → `graphql/graphql.go`
- kind: geth
- calls: []
- reads: `Backend`를 통한 상태/블록(`Account.Balance`, `Block.Number` 등)
- writes: —
- emits: —
- branches:
  - when: "질의 검증 실패(스키마 불일치)" → then: "GraphQL 에러 응답" at: `graphql/service.go:90`
  - when: "리졸버에서 백엔드 읽기 실패(없는 블록 등)" → then: "필드 에러" at: `graphql/graphql.go`
- invariant: —
- prose: 스키마에 맞춰 질의를 실행하고, 각 리졸버가 `ethapi.Backend`로 계정 잔액·블록 등을 읽어 채운다. 질의가 스키마와 안 맞거나 없는 데이터를 요청하면 GraphQL 에러로 응답한다. 상태를 바꾸는 경로는 없다.

---

## prose 렌더 (분기/실패 포함)
> GraphQL이 켜져 있으면 기동 시 스키마를 파싱해 `/graphql`에 핸들러를 단다(스키마 오류면 기동 실패).
> 요청이 오면 `ServeHTTP`가 POST 본문에서 질의·변수를 꺼내고(JSON 깨지면 400, 시간 초과면 타임아웃),
> `Schema.Exec`가 리졸버를 통해 `ethapi.Backend`에서 계정·블록을 읽어 채운다. 스키마 불일치나 없는
> 데이터는 GraphQL 에러가 되며, 이 경로는 읽기 전용이라 상태를 바꾸지 않는다.
