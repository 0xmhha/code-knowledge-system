# 설계: EvidencePack 본문 출처 보장 (snapshot-first BodyFetcher)

작성: 2026-07-10 · 상태: 구현 대기 · 대상 리포: code-knowledge-system(주) + code-knowledge-vector(보조)
전제 브랜치: 두 리포 모두 `docs/retire-ckg-node-id` (cks `3e7c22d8`, ckv `8c0426f` 이후)
관련 사건 기록: `cks-seminar/REPORT-bench-a2-postfix.md` run#4 절

---

## 0. 한 줄 요약

팩 body의 계약을 **"디스크(source_root)에서 읽는다" → "indexed_head 시점의 본문을 준다"**로
바꾼다. ckv DB에 이미 있는 인덱스 시점 스냅샷(`chunks.text`)을 1순위 출처로 쓰고, 파일 폴백은
기동 시 HEAD 게이트로 보호한다. 이후 source_root가 어디를 가리키든/드리프트하든 팩은 구조적으로
오염 불가능해진다.

## 1. 배경 — 실측된 사고 (왜 이 작업인가)

- cks-mcp 설정(`cks-stablenet.yaml`)의 `backends.ckg.source_root`가 실개발 리포
  (`~/Work/github/go-stablenet`, HEAD 44d75d17)를 가리킨 채 서버가 기동됐다. 인덱스(pr-77-2)는
  0bf2f4d1b 기준. 실개발 리포에는 벤치 대상 버그의 수정(fix #77, 98f05c2a0)이 이미 반영돼 있었다.
- 오염 경로 2개가 동시에 열렸다:
  1. **BodyFetcher**: `cmd/cks-mcp/main.go` `budget.FilesystemFetcher{Root: cfg.Backends.CKG.SourceRoot}`
     — 팩 body가 수정된 트리의 파일에서 읽혔다 (라인 시프트 포함).
  2. **health 광고**: `cks.ops.health` 응답의 `source_root` 필드가 이 경로를 광고 → 에이전트들이
     "소스 트리"로 오인하고 그 경로를 Read → **수정된 코드를 타깃 코드로 오인해 진짜 근본 원인
     엣지를 기각** (벤치 run#2·run#3에서 `gasTipChanged` — 수정 커밋에만 존재하는 식별자 — 를
     기각 근거로 인용한 것이 증거).
- 참고: ckv/ckg **매니페스트**의 src_root는 각각 `cks-seminar/test/vector-db-5`,
  `Work/github/test/analysis-test-3`로 둘 다 0bf2f4d1b 클린이었다. 오염원은 인덱스가 아니라
  **config 값과 그것을 신뢰한 소비 경로**다.
- 기존 방어선의 상태: `internal/mcp/alignment.go`에 config-vs-manifest 불일치(142행)와
  source_root HEAD 드리프트(151행) **경고 코드가 이미 있으나**, WARNING(서빙 계속) 수준이고
  사고 당시 health 응답에서 에이전트가 소비하지 않았다. 경고로는 부족하다 — 구조로 막는다.

## 2. 목표 계약 (설계 불변식)

> **INV-BODY-1**: EvidencePack의 모든 `bodies[].text`는 indexed_head 시점의 본문이거나,
> 그렇게 보장할 수 없으면 body 없이(엣지-온리/citation-only) 서빙된다. 조용한 드리프트 본문은
> 어떤 경로로도 나가지 않는다.

> **INV-BODY-2**: health는 파일시스템 경로를 "읽어도 되는 소스 트리"로 광고하지 않는다.
> 본문 출처는 `body_provenance` 필드로 명시한다(snapshot / filesystem-verified / degraded).

## 3. 설계

### 3.1 ckv — 스냅샷 본문 조회 API (신규)

ckv DB의 `chunks` 테이블에는 인덱스 시점 본문이 이미 있다(`text` 컬럼, `start_line`/`end_line`
좌표 포함). 임의 라인 스팬 인용에 본문을 주는 조회를 추가한다.

**(a) store 레이어** — `internal/store/sqlitevec/store.go`에 추가:

```go
// BodyByRange returns the indexed-snapshot text covering [startLine, endLine]
// of file, sliced to exactly that range. found=false when no stored chunk
// covers the span (partial overlap is NOT served — a partial body is worse
// than no body for an LLM consumer).
func (s *Store) BodyByRange(ctx context.Context, file string, startLine, endLine int) (text string, found bool, err error)
```

구현 지침:
- 쿼리: `SELECT` + 기존 `chunkSelectCols` 재사용, `WHERE c.file = ? AND c.start_line <= ? AND
  c.end_line >= ? ORDER BY (c.end_line - c.start_line) ASC LIMIT 1`
  (요청 스팬을 **완전히 덮는** 가장 작은 청크. 파라미터 순서: file, startLine, endLine 주의 —
  덮는 조건은 `start_line <= startLine AND end_line >= endLine`).
- 같은 파일을 이미 청크 단위로 정렬 반환하는 `LookupByFileOrdered`(store.go:849)가 있으니
  스캔 헬퍼(`scanChunk`, store.go:527 인근)를 그대로 재사용할 것.
- **라인 슬라이스**: 청크 텍스트는 `[chunk.StartLine .. chunk.EndLine]`에 대응한다.
  `lines := strings.Split(c.Text, "\n")`에서 `lines[startLine-c.StartLine : endLine-c.StartLine+1]`
  를 잘라 `strings.Join(..., "\n")` 반환. 경계 검증: 슬라이스 인덱스가 len(lines)를 벗어나면
  (인덱스 손상) found=false로 처리 — panic 금지.
- 덮는 청크가 없으면 `("", false, nil)`. 에러는 SQL 실패만.
- 청크 종류 무관(symbol/file_header/doc/invariant 모두 대상). doc/invariant 청크(파일 밖
  코퍼스)도 file 경로가 맞으면 그대로 서빙 — DocsRoots 파일 읽기를 대체한다.

**(b) engine/공개 API** — `internal/query/engine.go` + `pkg/ckv/ckv.go`에 위임 메서드 추가:

```go
// internal/query/engine.go
func (e *Engine) BodyByRange(ctx context.Context, file string, startLine, endLine int) (string, bool, error) {
    return e.store.BodyByRange(ctx, file, startLine, endLine)
}
// pkg/ckv/ckv.go — 같은 시그니처로 위임 (기존 위임 패턴: SemanticSearch 184행 참조)
```

**(c) 테스트** — `internal/store/sqlitevec/body_by_range_test.go` 신규:
- 정확 스팬 일치(청크 전체) / 부분 스팬(청크 내부 슬라이스) / 덮지 못하는 스팬(found=false) /
  여러 청크가 덮을 때 가장 작은 청크 선택 / 라인 경계(첫 줄·마지막 줄) 케이스.
- 기존 테스트 픽스처 `mkChunk`(store_test.go:15) 재사용. text에 개행 포함 다줄 본문을 넣을 것.

### 3.2 cks — SnapshotFetcher (BodyFetcher의 새 기본 구현)

**(a) ckvclient 인터페이스 확장** — `internal/ckvclient/interface.go`:

```go
// FetchBody returns the indexed-snapshot text for a citation span.
// found=false means the snapshot has no covering chunk (the caller may
// fall back to a VERIFIED filesystem read, or degrade to edge-only).
FetchBody(ctx context.Context, file string, startLine, endLine int) (text string, found bool, err error)
```

- `real.go`: `r.eng.BodyByRange(...)` 위임.
- `fake.go`: `BodyByRangeVal map[string]string` (key: `fmt.Sprintf("%s:%d-%d", file, s, e)`) +
  `Calls.FetchBody` 기록 + `FetchBodyErr`. 기존 Fake 패턴(SearchHits/SearchErr) 그대로.
- Dummy/Degraded 구현체가 있으면 `("", false, nil)` 반환.

**(b) SnapshotFetcher** — `internal/composer/budget/snapshot.go` 신규:

```go
// SnapshotFetcher serves citation bodies from the ckv indexed snapshot
// (chunks.text), guaranteeing INV-BODY-1: every body is indexed_head-vintage.
// Fallback (optional) is a FilesystemFetcher used ONLY when FallbackVerified
// is true — the operator's startup gate (§3.4) confirmed the tree HEAD equals
// indexed_head. Without verification, missing snapshot coverage returns
// ("", nil): stage4 skips the body and the citation survives as edge-only.
type SnapshotFetcher struct {
    CKV              ckvclient.Client
    Fallback         BodyFetcher // usually *FilesystemFetcher; nil allowed
    FallbackVerified bool
}

func (f *SnapshotFetcher) Fetch(ctx context.Context, c contract.Citation) (string, error) {
    text, found, err := f.CKV.FetchBody(ctx, c.File, c.StartLine, c.EndLine)
    if err == nil && found {
        return text, nil
    }
    // snapshot miss (or ckv error): verified filesystem fallback only.
    if f.Fallback != nil && f.FallbackVerified {
        return f.Fallback.Fetch(ctx, c)
    }
    return "", nil // stage4: skip body, citation may still ride edge-only
}
```

주의:
- ckv 조회 에러는 폴백 시도 후 그래도 없으면 `("", nil)`로 삼킨다 — BodyFetcher 계약
  (fetcher.go 주석: empty+nil = "no body available", 단건 실패가 전체 할당을 죽이지 않음) 유지.
  단 footprint 관측을 위해 에러·미스 카운트를 원하면 Stage4Output이 아니라 fetcher 내부
  카운터(atomic)로 두고 §3.5의 health에 노출해도 된다(선택).
- **DocsRoots 처리**: 현재 FilesystemFetcher가 DocsRoots로 코퍼스 파일을 읽는다
  (main.go:139-146 주석 참조). doc/invariant 청크는 ckv 스냅샷에 본문이 있으므로 대부분
  스냅샷 히트가 된다. 폴백 검증(§3.4)은 source_root만 대상으로 하므로, DocsRoots 파일은
  스냅샷 미스 시 **검증 없이 읽어도 오염 위험이 낮으나**, 일관성을 위해 동일 규칙 적용
  (FallbackVerified 게이트)을 권장 — 코퍼스도 바뀔 수 있다.

**(c) 테스트** — `internal/composer/budget/snapshot_test.go` 신규:
- 스냅샷 히트 → 그대로 반환, 폴백 미호출(Fake.Calls로 단언).
- 스냅샷 미스 + FallbackVerified=true → 폴백 반환.
- 스냅샷 미스 + FallbackVerified=false → ("", nil) (폴백 존재해도 미호출).
- ckv 에러 + verified 폴백 → 폴백 반환.
- 통합: Allocator에 SnapshotFetcher를 꽂고 "미스 인용은 Skipped(EmptyBodies)로 가되 엣지-온리
  편입(assemblePack)에서 살아남는다" — 기존 `assemble_edgeonly_test.go` 패턴 참조.

### 3.3 main.go 배선 교체

`cmd/cks-mcp/main.go` (현재 ~146행):

```go
// 현재
fetcher := &budget.FilesystemFetcher{Root: cfg.Backends.CKG.SourceRoot, DocsRoots: docsRoots}
// 변경
fsFetcher := &budget.FilesystemFetcher{Root: cfg.Backends.CKG.SourceRoot, DocsRoots: docsRoots}
fetcher := &budget.SnapshotFetcher{
    CKV:              be.ckv,
    Fallback:         fsFetcher,
    FallbackVerified: sourceRootVerified, // §3.4
}
```

- 기존 FilesystemFetcher는 삭제하지 않는다(폴백 + 기존 테스트 유지).
- be.ckv가 Real이 아닐 때(Dummy/Degraded): FetchBody가 found=false만 주므로 자동으로
  "verified 폴백 or 엣지-온리"로 동작 — 별도 분기 불필요.

### 3.4 기동 게이트 — source_root HEAD 검증 (폴백 보호)

이미 있는 부품을 승격한다: `internal/mcp/alignment.go`
- `AlignmentInputs.SourceHead`(66행)와 151행의 드리프트 WARNING("source_root HEAD != indexed
  commit — stale tree")이 존재한다. `computeStartupAlignment`(main.go 118행에서 호출)가
  SourceHead를 채우는지 확인하고, 비어 있으면 `git -C <source_root> rev-parse HEAD`로 채울 것
  (git 실패/비-git 디렉토리 → 미검증 취급).
- **신규 결과 변수**: `sourceRootVerified := (SourceHead != "" && SourceHead == indexed_head
  && SourceRootOK)` — 이것을 §3.3의 `FallbackVerified`로 전달.
- 정책 결정(중요): 드리프트를 **기동 실패로 만들지 않는다**. 스냅샷 경로가 본편이므로 서버는
  뜨되, 폴백만 잠그고 health에 명시한다. (기동 실패는 운영 편의를 해치고, 스냅샷만으로 대부분의
  팩이 완성되기 때문. alignment.OK 자체(ckg↔ckv 좌표 불일치)는 기존대로 serviceable=false.)

### 3.5 health 광고 교체 (INV-BODY-2)

`internal/mcp/health.go`:
- `SourceRoot` 필드(30행, 115행)는 **유지하되 의미를 재정의**하지 말고, 새 필드를 추가한다:

```go
BodyProvenance string `json:"body_provenance"` // "index-snapshot(+verified-fs-fallback)" | "index-snapshot(edge-only-on-miss)"
```

- 값: FallbackVerified=true → `"index-snapshot(+verified-fs-fallback)"`,
  false → `"index-snapshot(edge-only-on-miss)"`.
- **설명 필드 개보수**: 도구 설명(`internal/mcp/server.go`의 health 설명)에 한 줄 추가 —
  "source_root is the operator's index-build path, NOT a tree to read; bodies come from the
  index snapshot (see body_provenance)". 에이전트가 source_root를 읽기 대상으로 오인하는 것을
  설명 수준에서도 차단(사고의 오염 경로 2 대응).
- Deps에 `BodyProvenance string` (또는 bool `FallbackVerified`) 추가, main.go에서 전달.

### 3.6 절차 보조 (코드 밖, 선택)

- `scripts/gen-cks-config.sh` 사용 절차: 벤치 리셋 시 `--src <이번 워크스페이스>`로 재생성+재시작.
  단 본 설계 적용 후에는 잘못돼도 팩이 오염되지 않고 폴백만 잠긴다(안전한 실패).

## 4. 구현 순서 (의존성 순)

1. ckv `Store.BodyByRange` + 테스트 → `go test ./internal/store/sqlitevec/`
2. ckv engine/pkg 위임 → `go build ./...` (ckv 리포)
3. cks ckvclient `FetchBody` (interface/real/fake) — cks go.mod에 ckv 로컬 replace 이미 있음
4. cks `SnapshotFetcher` + 테스트 → `go test ./internal/composer/budget/`
5. 기동 게이트(`sourceRootVerified`) — alignment.go/main.go
6. health `body_provenance` + 도구 설명 한 줄
7. main.go 배선 교체 → `go build ./... && go test ./...` (전체)
8. `make build-bins`(버전 스탬프) → 서버 재시작(포트 172.20.82.90:8080, 앵커 워밍업 수 분,
   `nc -z`로 폴링) → §5 검증

## 5. 검증 (수용 기준)

1. **정합 환경 재생**: source_root=클린 클론 상태에서 ABC-A 티켓 프롬프트로
   `cks.context.get_for_task` 재생 → 팩 bodies/citations/neighbors 수가 종전(본문 12=코드10+
   불변식2, 인용 28, 이웃 16±)과 동등해야 한다. **스냅샷 전환으로 팩 품질이 후퇴하면 안 된다**
   (bodies 급감 시 BodyByRange의 덮음 판정/슬라이스 버그 의심).
2. **드리프트 시뮬레이션 (핵심 수용 테스트)**: config의 source_root를 일부러 다른 커밋의
   클론(예: 실개발 리포)으로 바꿔 재기동 →
   - health: `body_provenance = "index-snapshot(edge-only-on-miss)"` + alignment 경고 노출
   - 팩 재생: bodies의 모든 text가 **인덱스 커밋 시점 본문**이어야 함 — 스팟체크로
     `eth/gasprice/anzeon.go`의 SetCurrentBlock 가드 인용을 유도하는 프롬프트를 쓰고, body에
     `gasTipChanged`(수정 커밋의 식별자)가 **절대 등장하지 않음**을 확인. 이것이 사고 재발
     방지의 직접 검증이다.
3. **유닛 전체 그린**: 두 리포 `go test ./...` (ckv coreml 링크 실패는 기존 환경 이슈, 무시).
4. 완료 후 `cks-seminar/REPORT-bench-a2-postfix.md` run#4 절의 "조치 후보" 항목에 반영 완료
   표기(선택 — 벤치 세션 담당).

## 6. 하지 않는 것 (스코프 밖)

- 요청/세션 단위 source_root 오버라이드(멀티 워크스페이스 동시 서빙) — 후속 과제.
- 기동 실패(hard fail) 정책 — 채택 안 함(§3.4 근거).
- ckg 그래프 자체의 드리프트 검증 — 기존 alignment(ckg↔ckv 좌표)로 충분.
- freshness 도구의 의미 변경 — 그대로 둔다(신선도는 여전히 운영자 신호).

## 7. 커밋 가이드

- ckv 1커밋: `feat(store): BodyByRange — indexed-snapshot text lookup by citation span`
- cks 1커밋: `feat(composer): snapshot-first body provenance (INV-BODY-1) — SnapshotFetcher,
  verified fs fallback gate, health body_provenance`
- 메시지에 사고 요지(측정된 오염: 벤치 run#2·3이 드리프트 트리의 fix 코드를 근거로 진짜 원인
  기각) 1문단 포함. 영어, co-author 없음.
