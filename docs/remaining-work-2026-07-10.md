# 남은 작업 — 문서↔코드 대조 결과 (2026-07-10)

> Tier 3 (dated snapshot). 문서들(coordination-response-cks, HANDOFF-cks-evaluation-remaining,
> retire-ckg-node-id, symbol-identity-design, coding-agent HANDOFF-phase2/WORKLIST, CKV
> coordination-prompts)이 주장하는 작업 상태를 **실제 코드/인프라와 대조**해, **오류(E)와
> 미완료(M)만** 남긴 목록이다. 완료 항목은 각 원본 문서가 권위.
>
> 검증 시점: 2026-07-10, cks HEAD `a786143` 기준. 트리가 활발히 변경 중(검증 직후에도
> filter push-down/ChunkKind 추가 확인)이므로, 착수 전 각 항목의 근거를 재확인할 것.

---

## E — 오류 (문서↔코드 불일치 / 설정 결함)

### E1. 서빙 config의 source_root 불일치 (실질 결함) 🔴
- **현상**: `cks-stablenet.yaml`의 `source_root = …/go-stablenet`(HEAD `44d75d1`,
  fix/wbft 브랜치)인데, pr-77-2 인덱스는 `…/test/analysis-test-3`(@`0bf2f4d1b`)에서
  빌드됨(`knowledge-data/pr-77-2/manifest.json .src_root`).
- **영향**: citation body를 **인덱스와 다른 커밋의 트리에서 읽는다** — 스니펫/프레시니스가
  인덱스와 어긋날 수 있음. generator의 source-consistency assertion(`42533a4`)이 막으려는
  바로 그 결함이 현행(구세대) config에 존재.
- **수정**: `GO_STABLENET_ROOT=…/test/analysis-test-3` 기준으로 config 재생성
  (E2 해결과 함께). 재생성 시 assertion이 통과해야 함.

### E2. generator 기본값이 존재하지 않는 데이터셋 레이아웃을 가리킴 🔴
- **현상**: `scripts/gen-cks-config.sh` 기본 `CKS_DATASET_DIR=…/knowledge-data/pr-77` +
  `graph-db/graph.db`·`vector-db/` 레이아웃 기대. 실제 pr-77은 `ckg/`·`ckv/`(구 레이아웃),
  pr-77-2는 `graph.db`·`ckv/`. → **기본 실행 시 없는 경로를 가리키는 config가 생성됨**.
- **수정(택1)**: ① 데이터셋을 신레이아웃(`graph-db/`·`vector-db/`)으로 리네임/재빌드
  ② generator 기본값을 현존 레이아웃(pr-77-2)으로 정정. 레이아웃 이관이 계획이라면
  이관 완료 전까지 기본값이 깨진 상태임을 인지할 것.

### E3. cks-mcp 인스턴스 다운 🔴
- **현상**: `serve-cks-http.sh status` → stale pidfile. coding-agent WORKLIST의
  "T-2 배선 완료·부분 실동작" 서술과 달리 현재 서빙 중인 인스턴스 없음.
- **수정**: E1/E2 정정 + M1(deps bump) 후 `make build-bins` →
  `scripts/serve-cks-http.sh start` → `cks.ops.health`로 identity 검증.
  (그 전에 기동하면 E1 결함을 그대로 태움.)

### E4. symbol-identity-design.md §7 상태 stale 🟡
- **현상**: Phase 1(ckg canonical_id)·Phase 2(ckv Chunk.CanonicalID)는 **완료됨**
  (schema 1.23 / CKV PR#9), Phase 3의 resolution 절반도 canonical-first FindSymbol로
  구현됨(`e456698`). 문서에 상태 표기가 없어 전체가 미착수처럼 읽힘.
- **수정**: §7에 완료 표기 추가. 실제 잔여는 M7(anchor 마이그레이션)뿐.

### E5. coordination-response T1 표기 과대 🟡
- **현상**: T1 ✅ 서술이 `(+FindInvariants/GetConventions)`를 포함하는 듯 읽히나,
  구현은 flow 4메서드뿐. 그 2종은 **ckv Engine에도 미출시**(gated) — M5.
- **수정**: 해당 문서 T1 항목에 "2종은 CKV 출시 대기" 명시.

---

## M — 미완료

### M1. ckv deps stale — 기능 영향 있음 🔴 (즉시)
- pin `73e0763` ← ckv HEAD(`27226a8`)에 미반영 변경:
  - `494007e` **get_flow selector misses self-correcting** — coding-agent가 보고한
    "`get_flow(SetCurrentBlock)` → no flow found" 문제의 해결 커밋.
  - `fe40e76` qwen3-embedding 옵션(embed registry, `pkg/mcp/server.go`) — M4의 enabler.
- **조치**: go.mod bump → build/test → E3 재기동과 한 묶음.
- (ckg delta `f4b5be8..HEAD`는 빌드타깃/문서뿐 — 기능 영향 없음.)

### M2. cks 팔 벤치 미실행 (HANDOFF §5.1(d) / handoff T-3) 🔴
- 5팔(normal/skills/harness/vector-db/graph-db)은 완료·분석됨. **cks(결합) 팔만 미실행**.
- **선행**: M1 + E1~E3 정리 (측정 대상이 온전한 최신 cks여야 함).

### M3. T7 — Phase 2 인과 오케스트레이션 미착수
- composer가 `expand_flow` 다중홉으로 produce→store→consume 인과 체인 조립.
- 선행 없음(플로 도구 라이브 검증 완료). M2와의 측정 동결 충돌만 주의.

### M4. T8/R1 — 임베딩 차원 실측 (후순위 유지)
- ckv에 qwen3 옵션 추가(`fe40e76`)로 enabler 진전. **reindex-B(Qwen3) 인덱스 대기**.
- 도착 시: 1024-truncate vs full-dim 실측 → `embed_model` config + `embedder.knownDims` 갱신.

### M5. D-3 확장 2종 노출 — CKV gated
- `find_invariants`/`get_conventions`: **ckv `pkg/ckv.Engine`에 메서드 미출시**(0건) →
  cks 표면 노출 불가. CKV 출시 시 flow 4종과 동일 패턴(FlowClient + MCP 도구)으로 추가.

### M6. ckg_node_id 은퇴 (cks측) — 계획대로 대기
- `contract.Hit.CKGNodeID` 잔존(정상 — retire-ckg-node-id.md 체크리스트상 **ckv 컬럼
  제거가 선행**). ckv측 제거 후: Hit 필드 삭제 + real.go 매핑 삭제 + 주석/테스트 정리 +
  JSON 계약 변경 명시 + symbol-identity-design.md 반영.

### M7. Phase 3 anchor 마이그레이션 (경미)
- two-kind 스키마는 구현됨(`internal/inventory` `AnchorKindDef`, 빈값=def back-compat).
- **39 entries의 `code_anchors`에 `kind:` 필드 미부여** — back-compat로 동작 중이므로
  기능 문제는 없음. 명시 마이그레이션만 잔여.

---

## 권장 조치 순서

1. **[한 묶음] M1 → E1/E2 → E3**: ckv bump → config 재생성(source_root=analysis-test-3,
   현존 데이터셋 레이아웃) → `make build-bins` → 인스턴스 기동 → health 검증.
2. **M2**: cks 팔 벤치 실행 (5팔과 동일 조건: sonnet-5, 동일 태스크).
3. 벤치 후: **M3(T7)** 착수, **E4/E5** 문서 정정.
4. 외부 대기: **M4**(reindex-B), **M5**(ckv Engine 출시), **M6**(ckv 컬럼 제거).
