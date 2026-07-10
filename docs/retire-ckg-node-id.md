# `ckg_node_id` 은퇴 · `canonical_id` 단일화 (ADR-0001 마감)

- 상태: In progress — ckg 작업분 ✅ 마감 / ckv·cks 🔶 진행 중(전용 세션, ckv 먼저 → cks)
- 작성일: 2026-07-08 · 상태 갱신: 2026-07-10
- 범위: code-knowledge-graph (ckg) · code-knowledge-vector (ckv) · code-knowledge-system (cks)
- 관련: ckg `docs/adr/0001-canonical-symbol-id.md` · ckv `docs/adr/007-canonical-id-join-key.md` · cks `docs/symbol-identity-design.md`
- **Cross-repo 상태(2026-07-10)**: 전수조사 확인 — `ckg_node_id`/`CKGNodeID`는 ckg 코드 0건(외부
  이름), ckv 24건·cks 18건(비-테스트)이 통합 대상. ckg는 변경 없음으로 마감, 실제 코드 단일화는
  ckv(생산자)·cks(소비자) 세션이 아래 체크리스트대로 수행 중. 각 repo 체크박스는 해당 세션이 체크.

---

## TL;DR (결정)

ckv 청크와 cks Hit이 지금 **두 개**의 심볼 식별자를 실어 나른다: `ckg_node_id`(위치 해시)와 `canonical_id`(import-경로 정규화). 조사 결과 **`canonical_id`가 유일한 실사용 조인 키**이고 **`ckg_node_id`는 어떤 조회에도 쓰이지 않는 죽은 필드**다.

결정: **공유 표면(ckv `chunks` 컬럼, cks `Hit`)에서 `ckg_node_id`를 제거하고 `canonical_id`로 단일화한다.** ckg 내부 PK(`nodes.id`)는 그대로 둔다 — 거기선 진짜 노드 id다.

이 작업은 새 설계가 아니라 **ADR-0001(canonical symbol id) 마이그레이션의 미완 부분을 마감**하는 것이다.

---

## 배경 — 두 식별자

| 식별자 | 값 스킴 | 안정성 | 소유 |
|---|---|---|---|
| `ckg_node_id` | `sha256(qualified_name \| lang \| start_byte)[:16]` (`ckg/internal/parse/idgen.go:12`) | **빌드 종속** — 코드가 이동(라인 변화)하면 값이 바뀜 | ckg 내부 PK `nodes.id` |
| `canonical_id` | `<importpath>.(<*Recv>).<Method>` / 타입·const·var는 `<importpath>.<Name>`, 비-Go는 `<relpath>:<qname>` (`ckg/internal/parse/golang/declarations.go:376`) | **빌드 불변** — 이동·재빌드에도 유지 | ADR-0001, ckv/cks가 정렬 시 상속 |

정렬(`ckv build --ckg`) 시 aligner가 `(file_path, start_line)`로 청크를 ckg 노드에 매칭하고 **두 값을 그대로 복사**한다 (`ckv/internal/build/builder.go:346`). 그래서 `chunks`에 두 컬럼이 공존한다 (`ckv/internal/store/sqlitevec/store.go:146-147`).

`ckg_node_id`는 원래 식별자였고, `canonical_id`는 ADR-0001에서 "빌드 불변 조인 키"로 나중에 추가되었다. **둘을 병행 저장한 채 이관을 끝내지 않은 것**이 현재의 미정리 상태다.

---

## 냉정한 판정 — `ckg_node_id`는 죽은 필드다

세 repo + 외부 전수조사(`grep -rn "ckg_node_id\|CKGNodeID"`) 결과, **`ckg_node_id`를 입력으로 받거나 조회 키로 쓰는 코드가 한 줄도 없다.** 하는 일은 `계산 → 저장 → 인덱싱 → JSON 출력`뿐.

| 근거 | 사실 |
|---|---|
| ckg repo | `ckg_node_id`/`CKGNodeID` 참조 **0건**. ckg는 이 이름을 모른다 — 내부는 `nodes.id`(살아있는 PK, edges FK·traversal 백본). "ckg_node_id"는 ckv/cks가 붙인 외부 이름. |
| ckv | 생산(`builder.go:346`)·저장·인덱스(`store.go:174` `idx_chunks_ckg_node`)·JSON 출력(`query/engine.go:159`)만. `WHERE ckg_node_id` / `FindByNodeID` **0건**. |
| cks | ckv hit에서 실어 나르고(`ckvclient/real.go:140`) `Hit.CKGNodeID`로 직렬화만. 조인은 전부 `FindByCanonicalID`(canonical-first, `ckgclient/real.go:283-289`). |
| 테스트 | `cks/internal/ckgclient/b7_join_live_test.go:112,123` — `if hit.CKGNodeID != ""`로 **존재 여부 로깅만**. 실제 수집 대상은 canonical id. |
| 입력 API 부재 (결정적) | ckg 도구 표면(`internal/mcp/graph.go:106,126`)은 `qualified_name` · bare name · `canonical_id`만 입력으로 받는다. **raw node id를 받는 API가 없다** → 값을 들고 있어도 되먹일 곳이 없다. |
| 중복성 | "한 빌드 내 노드 위치 고정" 용도조차 `citation(file:start_line:commit_hash)`가 이미 수행(정렬도 그 매칭을 씀). |

외부 소비자: 세 repo 밖 Go 소비자 **0건**. 유일한 외부 언급은 (1) 수동 sqlite 디버그 쿼리, (2) 캡처된 MCP 출력 로그 — 둘 다 관찰용이며 소비 로직이 아니다.

`idx_chunks_ckg_node`는 **아무도 필터하지 않는 인덱스**라 명백한 dead artifact.

### 커버리지 백필은 제거의 전제조건이 아니다

`canonical_id`가 비어 있는 노드(builtin·blank `_`·함수 지역 var·pre-1.19 그래프)가 존재하지만, 그런 청크는 이미 **`Symbol → FindSymbol(qname)` 폴백**으로 조인된다. 아무것도 `ckg_node_id`를 키로 쓰지 않으므로 그 존재/부재는 동작을 바꾸지 않는다. `goCanonicalID` 커버리지 확대는 *조인 적중률 개선(독립 과제)*이지 이 제거의 선행조건이 아니다.

---

## 목표 종단 상태

- 공유 브리지 = **`canonical_id` 단일**.
- ckv `chunks.ckg_node_id` 컬럼·인덱스·구조체 필드·JSON 필드 제거.
- cks `Hit.CKGNodeID` 제거.
- ckg 내부 `nodes.id`는 **유지**(진짜 PK, 이름도 정직함).
- `canonical_id` 이름은 유지 — ADR-0001·store 컬럼·주석이 이미 이 이름이라 재명명 시 세 번째 미완 이관이 생긴다.

---

## 마이그레이션 체크리스트

### repo 1 — code-knowledge-graph (변경 없음)
- [ ] `nodes.id`(위치 해시)는 edges FK·traversal 백본이므로 유지.
- [ ] (선택·독립) `goCanonicalID` 커버리지 확대 — 빈 canonical 축소로 조인 적중률↑. **이 이관의 필수 아님.**

### repo 2 — code-knowledge-vector (생산자 — 먼저)
- [ ] `pkg/types/chunk.go:196` — `CKGNodeID` 필드 삭제
- [ ] `internal/build/builder.go:340,346` — `chunks[i].CKGNodeID = e.ID` 스탬프 삭제, 루프 조건을 `CanonicalID`/`StartLine>0` 기준으로 조정(`CanonicalID` 스탬프는 유지)
- [ ] `internal/ckgalign/aligner.go` — `Entry.ID` 및 `LookupEntry` 반환의 ID 제거(유일 소비처가 builder.go:346, 매칭은 line 기반이라 무관)
- [ ] `internal/query/engine.go:159` — 결과 구조체 `CKGNodeID` + `json:"ckg_node_id"` 삭제
- [ ] `internal/query/snippet.go:136` — `CKGNodeID` 매핑 삭제
- [ ] `internal/store/sqlitevec/store.go` — 스키마 `ckg_node_id`(146)·인덱스(174)·INSERT(293,308,352)·SELECT/scan(431,478,519,557) 전부 제거
- [ ] **DB 마이그레이션 결정** — SQLite `DROP COLUMN`이 번거로움:
  - (a) `CREATE TABLE`에서 제거 + `schema_version` 범프로 콜드 리빌드 유도(결정론적 재생성이라 안전) — **권장**
  - (b) 컬럼은 남기고 쓰기/읽기만 중단(orphan, 무해) — 임시 대안
- [ ] 테스트: ckv 테스트 참조 **0건** — 수정 불필요
- [ ] `docs/adr/007-canonical-id-join-key.md`에 "ckg_node_id 은퇴" 후속 기록

### repo 3 — code-knowledge-system (소비자 — ckv 다음)
- [ ] `pkg/contract/hit.go:44` — `Hit.CKGNodeID` 삭제 + 주석(27-35) 정리("Prefer CanonicalID" → "canonical_id 단일 조인 키")
- [ ] `internal/ckvclient/real.go:140` — `CKGNodeID: h.CKGNodeID` 매핑 삭제 + 주석(120-125) 정리(ckv 응답 필드 소멸과 커플링)
- [ ] `pkg/contract/retrievaltrace.go:67` — 주석의 CKGNodeID 언급 제거
- [ ] `internal/ckgclient/b7_join_live_test.go:12,112,123` — CKGNodeID 관찰/로깅 라인 삭제(테스트 의미는 canonical id 수집이라 그대로 유효)
- [ ] **JSON 계약 변경 명시** — EvidencePack/hit JSON에서 `ckg_node_id`(omitempty) 필드 소멸. 부재 허용 소비자는 무영향
- [ ] `docs/symbol-identity-design.md`에 ADR-0001 마감 반영

### 공통 — 순서 & 완료 게이트
1. **ckv → cks 순서** (생산자 먼저; cks의 `h.CKGNodeID` 참조가 컴파일 에러로 드러나 누락 방지)
2. ckv `schema_version` 범프 + ADR 후속 기록으로 ADR-0001 마감
3. **문서/자료 동기화**:
   - 세미나 덱 `cks-seminar/slides/deck-v10.md` — "id 통일 — 청크마다 `ckg_node_id`를 심는다" → `canonical_id`
   - 슬라이드 자산 `s09-ckg-map.svg` / `s10-ckv-map.svg`의 `ckg_node_id — 열쇠` 박스 → `canonical_id`
   - `cks-seminar/references/.claude/settings.local.json`의 디버그 sqlite 쿼리에서 `ckg_node_id` 제거(컬럼 드롭 시 에러)
4. **완료 게이트**: 세 repo에서 `grep -rn "ckg_node_id\|CKGNodeID"` → **0건**(ckg 내부 `nodes.id`는 별개이므로 잔존 정상)

---

## 리스크 평가

**낮음.**

- 런타임 동작은 이미 canonical-first 조인이라 **불변**.
- cks 쪽 누락은 컴파일러가 잡아준다(`h.CKGNodeID` 참조 컴파일 에러).
- 유일한 관찰 가능 변화: JSON 출력에서 죽은 필드 `ckg_node_id`(omitempty)가 사라짐. 프로그램적 소비자 0건 확인됨.
- DB는 결정론적 재생성이라 콜드 리빌드로 안전하게 컬럼 제거 가능.

---

## 부록 — 근거 파일:라인

- ckg id 스킴: `ckg/internal/parse/idgen.go:12` `MakeID`
- canonical id 스킴: `ckg/internal/parse/golang/declarations.go:376` `goCanonicalID`
- canonical-first 조인: `cks/internal/ckgclient/real.go:283-289` `FindByCanonicalID`
- ckg 입력 API(node id 미수용): `ckg/internal/mcp/graph.go:106,126`
- ckv 정렬 스탬프: `ckv/internal/build/builder.go:340-348` · `ckv/internal/ckgalign/aligner.go`
- ckv 저장/인덱스: `ckv/internal/store/sqlitevec/store.go:146-147,174,293-352,431-557`
- ckv 질의 출력: `ckv/internal/query/engine.go:159` · `ckv/internal/query/snippet.go:136` · `ckv/pkg/types/chunk.go:196`
- cks Hit 계약: `cks/pkg/contract/hit.go:27-44` · `cks/internal/ckvclient/real.go:120-141` · `cks/pkg/contract/retrievaltrace.go:67`
- cks 테스트: `cks/internal/ckgclient/b7_join_live_test.go:12,112,123`
