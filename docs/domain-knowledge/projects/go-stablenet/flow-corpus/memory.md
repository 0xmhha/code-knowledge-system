# 세션 메모리 (이 코퍼스를 만든 작업 중 저장된 사실)

> 이 코퍼스를 구축한 작업 세션 동안 영구 메모리에 저장한 내용을 옮긴 것. 두 항목 — cks 아키텍처
> (이 연구의 대상 시스템)와 커밋 컨벤션(이 사용자의 git 규칙) — 이다.
> 아키텍처 항목의 query-흐름은 세션 중 `intent.go` 권위 문서로 정정한 최신 내용을 반영했다.

---

## 1. cks 아키텍처 (reference)

> cks = Code Knowledge System. 이 연구(검색 알고리즘 실험)의 대상 시스템.

cks는 별도 리포 `~/Work/github/code-knowledge-system`(module `github.com/0xmhha/code-knowledge-system`)에
구현돼 있다. go-stablenet 리포 안에는 없다 — cks 구조를 찾을 땐 그 리포를 봐야 한다.

**구조** (출처: 해당 리포 README + `internal/` + `pkg/contract/intent.go`):

- **cks = Code Knowledge System**: Claude Code와 연동되는 통합 레이어. `cmd/cks-mcp`가 stdio
  JSON-RPC MCP 서버로 `cks.context.*`·`cks.ops.*`(13개 도구)를 노출. coding-agent 플러그인의
  `.mcp.json`에 `"cks": {command: ${CKS_MCP_BIN}, args:["-config", ${CKS_CONFIG}]}`로 등록.
  빌드 시 CGO 필요(sqlite-vec).

- 내부에서 **두 엔진을 in-process로 조합**:
  - **ckv** (`code-knowledge-vector`, `internal/ckvclient`) = 벡터/의미 검색. `SemanticSearch`.
    데이터 `ckv-stablenet/`.
  - **ckg** (`code-knowledge-graph`, `internal/ckgclient`) = 그래프 + BM25/키워드.
    `FindSymbol / GetSubgraph / ImpactOfChange / Neighbors / BM25Search`. 데이터 `data/ckg-stablenet/`.

- **query 흐름 (composer) — 순차 2단계 funnel** (`pkg/contract/intent.go`가 *"Pipeline model
  (not fan-out)"* 라고 명시):
  1. **intent 분류**
  2. **stage1**: ckv 의미검색 recall → 키워드 추출 → ckg BM25 rerank (신뢰도 미달 시 query를
     augment 하고 반복)
  3. **stage2**: stage1 키워드로 ckg BM25 + FindSymbol, 그리고 **ckv hits + ckg BM25 + ckg symbol**
     세 소스를 **가중 RRF(역순위 융합, k=60, Cormack 2009)**로 합침
  4. **stage3**: graph 1-hop 확장
  5. 토큰 예산 → sanitize → `EvidencePack` → MCP
  > RRF는 *병렬 팬아웃*이 아니라 stage2 내부의 다중-소스 융합이다. 전체 흐름은 ckv(의미) →
  > ckg(정확) 순차다.

- 도구별 주력 백엔드: `semantic_search`→ckv, `find_symbol/get_subgraph/impact_analysis`→ckg,
  `get_for_task`→둘 다(Composer 전체 funnel). 키워드/BM25는 별도가 아니라 ckg 안에 있음.

- **주의**: cks MCP 도구는 planner 등 특정 에이전트에만 노출되어 메인 루프에서 직접 호출 불가.

- **lexical gap**: 사용자의 암묵지 한글 용어는 코드 토큰과 안 겹쳐 ckg(BM25/symbol)로 못 잡는다.
  ckv(임베딩)가 그 의미 격차를 메워 ckg가 매칭할 키워드/심볼로 번역하는 것이 funnel의 존재 이유.

---

## 2. 커밋 컨벤션 (이 사용자의 git 규칙, feedback)

이 사용자가 작업하는 **모든 리포**의 git 커밋은 오픈소스 컨벤션을 따른다:

- **영어** 요약·본문. 한국어 금지.
- 메시지에 **개발 단계 용어 금지** ("Step 1/2", "WIP", "phase", 내부 파이프라인 jargon 등).
  완성된, 외부에 의미 있는 문장으로 기술.
- **`Co-Authored-By` 트레일러 완전 제거.** Claude/AI co-author 줄을 절대 추가하지 않는다
  (하니스 기본값이 제안해도). 이 규칙이 기본 트레일러 지침을 override 한다.

**이유:** 이 리포들은 오픈소스라 커밋 히스토리가 공개되며, 전문적·영어·도구 비의존·단일 저자로
읽혀야 한다.

**적용:** 커밋 시 Conventional Commit(`feat:`/`fix:`/`chore:` 등)을 영어 완성형으로 쓰고
co-author 트레일러를 생략한다. 잘못 만든 커밋(한국어 본문/co-author 트레일러)은 push 전에
amend/reword 한다.
