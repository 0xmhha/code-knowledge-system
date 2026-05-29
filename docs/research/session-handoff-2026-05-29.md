# Session Handoff — cks MCP 확장 + D-1 인벤토리 + 도구 정비 (2026-05-23 → 05-29)

> **목적**: 2026-05-23 핸드오프 이후 cks repo 단독으로 진행된 작업 14 commit + uncommitted 1 의 *결정·산출물·미완·다음 액션* 을 한 곳에 묶어, 다른 머신에서 같은 세션을 cold start 로 이어 받을 수 있게 한다.
>
> **전제 문서**: `docs/research/session-handoff-2026-05-23.md` (3-repo 통합 점검). 본 문서는 그 다음 단계 — *cks 단독 진척* 위주이며, ckv/ckg/Atlassian/Ollama 외부 의존은 23 일자 상태와 동일.

---

## 0. TL;DR (60 초 핸드오프)

1. **MCP surface 확장 완료**: 2 tools → **11 tools** (`cks.context.get_for_task`, `semantic_search`, `search_text`, `find_symbol`, `find_callers`, `find_callees`, `get_subgraph`, `impact_analysis`, `change_history`, `cks.ops.health`, `cks.ops.freshness`). coding-agent HLD 의 가상 `ckv_search` / `ckg_query` 와의 매핑 문서까지 작성.

2. **Smart Dummy 백엔드 확정**: `ckvclient.NewDummy()` / `ckgclient.NewDummy()` 가 실제 backend 부재 시 *fake data 가 아니라 LLM-actionable instruction* 을 반환. 호출 사용자 (코드 분석 LLM) 가 `/Users/wm-it-22-00661/Work/github/stable-net/go-stablenet-latest/.claude` 스킬을 직접 실행해 응답 생성.

3. **RRF Stage 2 + Vocabulary Stage 1**: score-sum fusion 을 *Reciprocal Rank Fusion (Cormack 2009, k=60)* 으로 교체. 한국어/모호 prompt 를 한 단계 앞에서 코드 키워드로 확장하는 `internal/vocab` 패키지 추가, `cks.yaml` 의 `vocab.glossary_path` 로 opt-in.

4. **D-1 도메인 인벤토리 23 entries 작성**: `docs/domain-knowledge/projects/go-stablenet/` 아래 schema + 23 yaml entries (P0 9 / P1 11 / P2 3), 11 subsystem · B1–B7 knowledge type 모두 커버. **모두 `status: needs_verification`**. 검증 단계 시작 전.

5. **검증·도구 트리오 완성**: `cmd/cks-glossary-gen` (entries → glossary.yaml), `cmd/cks-inventory-check` (mechanical validator), `cmd/cks-entry-verify` (verified 전이 + inventory.md auto-sync). 사용자 검증 워크플로우 가속을 위한 P0 도구.

6. **현재 uncommitted**: `docs/coding-agent-mcp-mapping.md` (235 lines) — coding-agent ↔ CKS 11 tool 매핑 문서. 본 handoff 와 같이 commit + push 권장.

7. **다음 작업 후보**: ① 사용자 검증 시작 (verify 도구 사용), ② D1 합본 (`cks.ops.health` vocab status + Makefile inventory-check target + cks-glossary-gen 리팩토링), ③ CKS eval 시나리오 11 tool 확장, ④ A4 GovCouncil/MasterMinter + A11 tx taxonomy entries 추가.

---

## 1. 세션 흐름 (commit 시간순, 2026-05-28 → 05-29)

| commit | 일자 | 의미 |
|---|---|---|
| `b6fdf04 docs(planning): system review, technical analysis, and integrated workplan for coding-agent integration` | 05-28 | 3 문서 (system-review / technical-analysis / integrated-workplan) — coding-agent 통합 관점에서 cks 의 격차 + 해소 작업 분해 |
| `fde046d feat(client): extend ckgclient and ckvclient interfaces for upcoming CKS MCP tools` | 05-28 | 9 신규 method (ImpactOfChange, EvidenceForIntent, GetNodePRs, GetSubgraph, Freshness, ...). fake/real/dummy 3 adapter 동시 확장 |
| `d4027bc feat(dummy): smart-dummy ckv/ckg backends emit LLM instructions in place of real calls` | 05-28 | `pkg/contract/instruction.go` 의 `InstructionCollector` + ctx-bound 패턴. EvidencePack.Instructions 필드 추가 |
| `9781118 feat(mcp): expand CKS MCP surface from 2 to 11 tools for coding-agent` | 05-28 | `internal/mcp/{graph,search,analysis,freshness}.go` 신규. wire name 명명 규칙 `cks.context.*` / `cks.ops.*` |
| `9f90178 refactor(stage2): replace score-sum fusion with formal RRF over BM25 and FindSymbol lists` | 05-28 | Stage 2 aggregator 재작성. `Config.RRFK=60`, `BMWeight`, `SymbolWeight`. 두 기존 테스트 RRF semantics 로 재작성 |
| `0f5afba feat(vocab): glossary loader + query expansion for Korean and ambiguous prompts` | 05-28 | `internal/vocab` 패키지 + Glossary YAML 로더 + case-insensitive substring matching |
| `e0386b2 docs(d1): bootstrap domain-knowledge inventory — shared schema + go-stablenet project skeleton + 6 sample entries` | 05-29 | `docs/domain-knowledge/shared/` schema + `projects/go-stablenet/` skeleton + 6 P0 entries |
| `9ed11c4 docs(d1): expand go-stablenet inventory to 15 entries — full subsystem and knowledge-type coverage` | 05-29 | 9 entries 추가 → A1–A11 모두 1 entry 이상, B1–B7 모두 exercise |
| `864817a feat(stage1): wire vocab.Resolver into composer Stage 1 prompt expansion` | 05-29 | `stage1.WithVocab(*vocab.Resolver) Option`, Stage1Output.VocabExpanded/Keywords 필드. cks-mcp 가 `cfg.Vocab.GlossaryPath` 에 따라 opt-in |
| `2845e25 feat(cks-glossary-gen): CLI to derive a project's glossary.yaml from inventory entries` | 05-29 | status-gated (default verified) entries → glossary.yaml. round-trip 검증 |
| `74baacc feat(inventory): cks-inventory-check + cks-entry-verify CLIs with shared internal/inventory package` | 05-29 | `internal/inventory` 패키지 (types/load/validate/render/verify) + 2 CLI + 4 unit test 통과 |
| `b2c2e75 docs(d1): add 8 P1 entries (round-change, handlers, ...)` | 05-29 | 인벤토리 15 → **23 entries**. 동시에 A8 anchor + subsystems.yaml 의 깨진 경로 `core/stablenet_genesis.go` → `core/genesis.go::DefaultStableNetMainnetGenesisBlock` 정정 |
| *uncommitted* `docs/coding-agent-mcp-mapping.md` | 05-29 | coding-agent HLD 의 가상 `ckv_search` / `ckg_query` / `ckg_impact` ↔ 11 live tool 매핑 + composition example + 7 가지 구조적 차이 표 |

## 2. 산출물 상세

### 2.1 코드 — 신규 패키지

| 경로 | 역할 |
|---|---|
| `internal/vocab/` | Glossary YAML loader + `Resolver.Resolve(prompt)` → `ResolveResult{Expanded, MatchedKeywords}` |
| `internal/inventory/` | Project / Entry / Subsystem 도메인 타입, schema + cross-ref validator, inventory.md 카운트 in-place 갱신, yaml.Node 기반 `MarkVerified` (주석·순서 보존) |
| `internal/mcp/{graph,search,analysis,freshness}.go` | 9 신규 MCP tool 핸들러 |
| `pkg/contract/instruction.go` | `DummyInstruction` + ctx-bound `InstructionCollector` (Smart Dummy 의 응답 통로) |

### 2.2 코드 — CLI 추가

| 경로 | 용도 |
|---|---|
| `cmd/cks-glossary-gen/` | entries → glossary.yaml. `-status verified` 게이트. `vocab.New` round-trip 검증 |
| `cmd/cks-inventory-check/` | mechanical validator. `-update-inventory` 로 inventory.md 갱신 (errors 있으면 skip) |
| `cmd/cks-entry-verify/` | 단일 entry verified 전이. pre-flight `ValidateEntry` 통과 시에만 파일 write |

Makefile `build-bins` 타겟이 6 binary 모두 빌드 (mcp/agent/eval/glossary-gen/inventory-check/entry-verify).

### 2.3 문서 — D-1 도메인 인벤토리

```
docs/domain-knowledge/
├── shared/                                # 프로젝트 독립 schema
│   ├── README.md
│   ├── SCHEMA.md                          # entry yaml 필드 정의
│   ├── KNOWLEDGE_TYPES.md                 # B1–B7 catalog
│   ├── STATUS_LIFECYCLE.md                # needs_author → draft → needs_verification → verified
│   ├── PROJECT_REGISTRATION.md
│   └── entry.schema.yaml                  # JSON-Schema 형태 (참조용)
└── projects/go-stablenet/
    ├── project.yaml                       # code_root, skill_root, indexing
    ├── subsystems.yaml                    # A1–A11 (11 subsystem)
    ├── inventory.md                       # 카운트 대시보드 + freeform Pending work
    ├── glossary.yaml                      # cks-glossary-gen 산출 (verified 게이트)
    └── entries/                           # 23 yaml (4 P0 + 14 P1 + 5 P2... 실제 수치는 inventory.md 참조)
        ├── A1.wbft_core.consensus_flow_architecture.yaml
        ├── A1.wbft_core.quorum_calc.yaml
        ├── A1.wbft_core.round_change_protocol.yaml
        ├── A1.wbft_core.message_handlers.yaml
        ├── A2.block_encoding.wbft_extra_struct.yaml
        ├── A2.block_encoding.filtered_header_hash.yaml
        ├── A3.validator_set.epoch_transition.yaml
        ├── A3.timing.wbft_config_defaults.yaml
        ├── A4.system_contracts.addresses.yaml
        ├── A4.system_contracts.gov_minter.yaml
        ├── A5.account_extra.bit_layout.yaml
        ├── A5.native_coin.issuance_burn_flow.yaml
        ├── A6.fee_delegation.signing_model.yaml
        ├── A6.anzeon_gas.tip_override.yaml
        ├── A7.hardfork.add_new_fork_procedure.yaml
        ├── A8.genesis.bootstrap_architecture.yaml
        ├── A8.genesis.inject_contracts_two_phase.yaml
        ├── A8.genesis.json_authoring_checklist.yaml
        ├── A9.istanbul_p2p.protocol_architecture.yaml
        ├── A9.istanbul_rpc.api_reference.yaml
        ├── A10.codegen.no_edit_zones.yaml
        ├── A10.codegen.contract_regen_procedure.yaml
        └── A11.state_transition.blacklist_check_points.yaml
```

검증 상태: `cks-inventory-check` → **23 entries, 0 errors, 0 warnings**.

### 2.4 문서 — 그 외

| 경로 | 역할 |
|---|---|
| `docs/system-review-2026-05-27.md` | 통합 시점에서 cks 격차 정성 분석 |
| `docs/technical-analysis-2026-05-27.md` | 격차 기술 분해 |
| `docs/integrated-workplan-2026-05-27.md` | C/D/G/I/P/S/V 트랙 작업 계획 |
| `docs/coding-agent-mcp-mapping.md` *(uncommitted)* | 본 세션 최종 산출. coding-agent ↔ CKS MCP 매핑 |

## 3. 결정 / 발견

### 3.1 아키텍처 결정

- **coding-agent 는 cks 외엔 절대 접근하지 않음**: ckv / ckg 와의 직접 통신 금지. 단일 MCP 서버 (`cks-mcp`) 만 노출. 보안 (caller ≠ index writer) + 인터페이스 안정성.
- **인덱싱은 MCP 표면 밖**: `ckv build` / `ckg build` 는 operator action. coding-agent 가 MCP 로 트리거하지 않음. `cks.ops.freshness` 로 stale 여부만 read-only 노출.
- **Smart Dummy = LLM instruction 반환**: real backend 부재 시에도 "백엔드 미연결" 에러 아닌 *"이 스킬을 이 경로에 대해 실행해 응답 생성하라"* 라는 actionable instruction 반환. 결과는 ctx-bound `InstructionCollector` 가 모아 `EvidencePack.Instructions` 로 노출.

### 3.2 검색 알고리즘 결정

- **RRF (Reciprocal Rank Fusion) k=60**: BM25 hit list + FindSymbol hit list 의 *rank* 만 사용해 융합. score-sum (heuristic) 폐기. 두 리스트의 score 스케일이 다른 문제 해결.
- **Vocabulary resolver = Stage 0.5**: prompt → glossary 매칭 → 코드 키워드 append → Stage 1 다른 검색기에 들어가는 query 가 코드용 어휘로 보강. nil-safe (resolver=nil 이면 verbatim prompt).

### 3.3 검증 워크플로우 결정

- **status lifecycle**: `needs_author` → `draft` → `needs_verification` → `verified`. `ckv build` 는 verified 만 indexing. 현재 모든 entry needs_verification.
- **mechanical 자동 + substantive 사용자**: `cks-inventory-check` 가 schema / filename / cross-ref / FS ref 검증. invariants 가 사실인지, alias 가 실제 도메인 용어인지는 사람만 판단.
- **inventory.md 카운트 자동 sync, freeform 보존**: 4 카운트 테이블은 generator 가 재생성, "Conventions" / "Pending work" 같은 prose 는 byte-for-byte 보존.

### 3.4 발견된 데이터 정합성 이슈

- `entries/A8.genesis.bootstrap_architecture.yaml` anchor 6 + `subsystems.yaml` A8 code_paths 가 *존재하지 않는* `core/stablenet_genesis.go` 를 가리킴 → 실재하는 `core/genesis.go` + `DefaultStableNetMainnetGenesisBlock` 로 정정 (commit `b2c2e75`).
- 향후 검증 단계에서 유사한 이슈가 더 나올 수 있음. `cks-inventory-check` 가 즉시 잡아냄.

## 4. Working tree 상태

```
branch: main
ahead of origin/main: 0   (origin 이 이미 b2c2e75 까지 sync 됨)
uncommitted:
  ?? docs/coding-agent-mcp-mapping.md   (235 lines)
```

`go build ./...` / `go test ./...` / `go vet ./...` 모두 클린 (cached pass 확인됨).

## 5. 다음 머신/세션을 위한 액션 (cold start)

### 5.1 첫 5 분 셋업

```bash
# Clone
git clone <repo-url>/code-knowledge-system.git
cd code-knowledge-system

# uncommitted 가 본 머신에 안 푸시된 상태면, 본 문서 합본 commit + push 먼저:
git status                                            # docs/coding-agent-mcp-mapping.md 등 확인
# (push 완료된 상태라면) 그대로 진행

# 빌드 / 테스트 확인
go build ./...
go test ./...
go vet ./...

# 인벤토리 검증
go run ./cmd/cks-inventory-check \
    -project docs/domain-knowledge/projects/go-stablenet
# 기대: 23 entries, 0 errors, 0 warnings
```

### 5.2 정독 순서 (10 분)

1. **본 문서 §0–§3** — 누적 결정 / 산출물.
2. **`docs/research/session-handoff-2026-05-23.md`** — 3-repo 통합 점검 (ckv/ckg/Atlassian/Ollama 외부 의존 상황). 본 문서는 이 위에 *cks 단독 진척* 만 얹은 것.
3. **`docs/coding-agent-mcp-mapping.md`** — coding-agent 작업자가 cks 와 어떻게 대화하는지. 다른 세션 (coding-agent repo) 작업자도 이 문서 1 부 받아가야 함.
4. **`docs/integrated-workplan-2026-05-27.md`** — C/D/G/I/P/S/V 트랙 작업 분해. 본 문서 §6 의 "다음 후보" 가 이 분해 안에 매핑됨.
5. **`docs/domain-knowledge/projects/go-stablenet/inventory.md`** — 인벤토리 현황 대시보드.

### 5.3 시나리오별 진입점

| 다음 작업 | 정독 + 시작점 |
|---|---|
| 사용자 검증 (verify 도구 사용) | `docs/domain-knowledge/shared/STATUS_LIFECYCLE.md` §3 substantive checklist + `cmd/cks-entry-verify/README.md` |
| C-시리즈 LLM 단독 작업 | 본 문서 §6 + `docs/integrated-workplan-2026-05-27.md` |
| coding-agent ↔ cks 통합 (다른 repo) | `docs/coding-agent-mcp-mapping.md` 만 읽으면 충분 |
| ckv / ckg 작업 (다른 repo) | `docs/research/session-handoff-2026-05-23.md` §6 + 해당 repo 자체 문서 |

## 6. 미완료 작업 / 다음 후보

본 세션 마지막 시점의 후보 우선순위 (이전 답변에서 정리):

### 6.1 P0 — 발견된 데이터 정합성 fix

| 작업 | 책임 |
|---|---|
| A4.system_contracts.addresses 등 기존 entries 의 file 경로 재검증 (b2c2e75 의 A8 fix 외에 비슷한 케이스 잠재) | LLM 단독 (`cks-inventory-check` 가 잡으면 자동 발견) |

### 6.2 P1 — 사용자 검증 시작

| 작업 | 도구 | 책임 |
|---|---|---|
| 23 needs_verification entries 검증 → verified 전이 | `cks-inventory-check`, `cks-entry-verify` | 사용자 (도메인 검토) |
| verified 도달 시 `cks-glossary-gen` 재실행 → glossary.yaml 발급 | `cks-glossary-gen` | 사용자 |
| `cks.yaml` 에 `vocab.glossary_path: docs/domain-knowledge/projects/go-stablenet/glossary.yaml` 설정 | (edit only) | 사용자 |

### 6.3 P2 — LLM 단독 가능 (검증과 병렬)

| ID | 작업 | 규모 | 우선 |
|---|---|---|---|
| D1 | `cks.ops.health` 에 vocab status 노출 + Makefile `inventory-check` target + cks-glossary-gen 을 `internal/inventory` 위로 리팩토링 | XS+XS+S | 사용자 검증 가속 |
| D2 | A4 GovCouncil / GovMasterMinter + A11 tx taxonomy / mempool ordering / fee-delegation 0x16 entries 추가 | M | 인벤토리 폭 |
| D3 | CKS eval 시나리오 11 MCP tool 회귀 (현재 9 개는 `get_for_task` 중심) | M | 회귀 방지 |
| D4 | vocab footprint 분석 도구 (`composer.stage1_extracted` 이벤트의 vocab_expanded / vocab_keywords 분포 집계) | S | verified glossary 있어야 의미 |
| D5 | S-13 Cross-encoder Reranker (BGE Reranker M3) | L | Real embedder 후 |

### 6.4 외부 의존 (다른 작업자 / 세션)

| 트랙 | 상태 | 책임 |
|---|---|---|
| CKV (Ollama qwen3 + go-stablenet index + PR fixture + real eval + contextual enrichment) | 23 일자 handoff 와 동일 — 본 세션에서 변경 없음 | ckv repo 작업자 |
| CKG (인덱스 확인 / store.Reader / PR breadcrumb / real BM25 score / CamelCase FTS5) | 동일 — 사용자가 "ckg가 완성된 줄 알았는데 내가 계속 수정 중" 이라 명시 (본 세션). 진행률 본 세션에서 확인 안 함 | ckg repo 작업자 |
| 인프라 I-1 Ollama 설치 / I-2 Atlassian MCP / I-3 ChainBench MCP | 동일 — 미설치 / 미인증 | 사용자 / 운영 |
| coding-agent plugin (P-1 ~ P-7) | 다른 repo · 다른 세션에서 진행 중 — 본 세션은 cks 측 매핑 문서만 제공 | coding-agent repo 작업자 |

## 7. 본 머신 ↔ 다른 머신 sync 절차

다른 머신에서 작업을 이어 받기 위한 보수적 단계:

```bash
# 본 머신 (현재 위치):
git status                              # docs/coding-agent-mcp-mapping.md 등 확인
git add docs/coding-agent-mcp-mapping.md docs/research/session-handoff-2026-05-29.md
git commit -m "docs(integration): coding-agent MCP mapping + 2026-05-29 session handoff"
git push origin main                    # 사용자 명시 승인 후

# 다른 머신:
git pull origin main
go build ./... && go test ./...         # cached pass 기대
go run ./cmd/cks-inventory-check -project docs/domain-knowledge/projects/go-stablenet
                                        # 기대: 23 entries, 0 errors, 0 warnings
```

다른 repo (ckv / ckg / coding-agent / go-stablenet) 는 23 일자 handoff §14.7 의 절차와 동일 — 본 세션은 cks repo 단독 진척이므로 그쪽은 추가 sync 없음.

## 8. Cross-references

- 이전 handoff: `docs/research/session-handoff-2026-05-23.md` (3-repo 통합 점검)
- 통합 워크플랜: `docs/integrated-workplan-2026-05-27.md`
- 시스템 리뷰: `docs/system-review-2026-05-27.md`
- 기술 분석: `docs/technical-analysis-2026-05-27.md`
- coding-agent 매핑: `docs/coding-agent-mcp-mapping.md` *(본 세션 마지막 산출)*
- 도메인 인벤토리: `docs/domain-knowledge/`
- CLI 사용법: `cmd/cks-glossary-gen/README.md`, `cmd/cks-inventory-check/README.md`, `cmd/cks-entry-verify/README.md`

## 9. 변경 이력

| 일자 | 변경 |
|---|---|
| 2026-05-29 | 초안. 2026-05-23 handoff 이후 cks repo 단독 진척 (commits b6fdf04 ~ b2c2e75 + uncommitted coding-agent-mcp-mapping) 정리. 다른 머신 cold start 절차 §5 + §7. |
