# 운영: 데이터셋 재인덱싱 + blue-green 전환 (P1-3/P1-4)

> 2026-07-10. 3자 합의(reindex-migration-design + consensus §10.6) 구현의 운영 절차.
> 도구: `scripts/reindex-dataset.sh`(오케스트레이터) + `scripts/serve-cks-http.sh`(인스턴스)
> + cks-mcp 기동 alignment assert.

## 레이아웃 (합의 확정)

```
knowledge-data/
  <family>@<short-commit>-<digest8>/   # 불변 버전 아티팩트 (예: pr-77-2@0bf2f4d1-a1b2c3d4)
    graph-db/    graph.db + manifest.json     (ckg)
    vector-db/   vector.db + manifest.json    (ckv, sources 원장 포함)
  <family>@current -> <family>@<ver>   # promote가 움직이는 유일한 포인터
  .<family>.build.lock.d/              # 빌드 직렬화 (mkdir 원자 락, 서빙과 무관)
```

- `<ver>` = `<short-commit>-<graph_digest[:8]>`. CKG가 digest를 공표하기 전까지는
  `<short-commit>-<UTC타임스탬프>`(과도기, 스크립트가 note 출력).
- 버전 디렉터리는 **생성 후 불변**. 롤백 = 이전 버전 디렉터리로 다시 promote.

## 재인덱싱 (조율 파이프라인)

```bash
FAMILY=pr-77-2 SRC=/abs/indexed-checkout \
  [CKG_POLICY=...] [CKV_POLICY=...] [CKV_DOCS=a,b] [CKV_FLOW=corpus.jsonl] [CKV_PR=1] \
  scripts/reindex-dataset.sh run
```

순서(§3.2): **lock → ckg build → 버전 디렉터리 확정 → ckv build(`--ckg` 정렬, sources
원장 기록) → 검증 게이트 → promote(@current 원자 교체)**. 크래시 시 @current는 옛
버전을 그대로 가리킴(라이브 무손상), `@staging-*`/`.building` 잔재는 수동 정리.

### 검증 게이트 (§5.1 — 실패 시 promote 안 함)

| # | 검사 | 강도 |
|---|---|---|
| 1 | `ckg validate` (dangling edge·스키마 불변식) | HARD |
| 2 | manifest alignment: src_commit 일치, schema ≥1.19, digest(양측 공표 시) | HARD |
| 3 | counts: ckv chunk_count > 0 | HARD |
| 4 | B7 canonical 매칭률 ≥90% (cks 라이브 fixture; `SKIP_LIVE_GATE=1`로 스킵 가능) | HARD |
| 5 | `ckg audit` (파일셋 대조) | SOFT |

게이트만 따로: `VERDIR=/abs/<family>@<ver> scripts/reindex-dataset.sh gate`
(2026-07-10 라이브 검증: pr-77-2 실산출물로 5단계 전부 통과 확인.)

## 서빙 전환

cks-mcp는 **기동 시 1회** 심볼릭을 resolve해 구체 버전에 고정(수명 내 재해석 없음,
health가 resolved 경로·`alignment` 블록으로 identity 증명). 전환 방법 2가지:

### (a) 재시작 전환 (기본 — 세션/벤치 경계에서)
```bash
CKS_DATASET_DIR=$KD/<family>@current GO_STABLENET_ROOT=<SRC> scripts/gen-cks-config.sh
scripts/serve-cks-http.sh restart          # ≤15s (엔진 오픈 + intent 앵커 pre-embed)
```
또는 `RESTART=1 … reindex-dataset.sh run`으로 promote 직후 자동 수행.

### (b) 인스턴스 blue-green (무중단이 필요할 때)
in-process reload는 하지 않는다(합의) — 대신 **인스턴스를 통째로 교체**한다:
```bash
# 1. 새 버전을 새 이름·포트로 기동 (green)
CKS_HTTP_CONFIG=<new-config> scripts/serve-cks-http.sh start cks-<family>-green <ip>:8081
# 2. health로 identity 검증 (name / dataset 버전 / alignment.ok / serviceable)
# 3. 소비자 전환: CKS_MCP_URL → :8081  (전역 settings 또는 프로젝트 settings.local.json)
# 4. 구 인스턴스(blue) 정지
scripts/serve-cks-http.sh stop cks-<family>
```
전환 원칙: **run 중 데이터셋 교체 금지** — 측정 무결성을 위해 세션/벤치 경계에서만.

## 롤백

```bash
VERDIR=$KD/<family>@<이전-ver> scripts/reindex-dataset.sh promote   # 포인터 되돌림
scripts/serve-cks-http.sh restart
```
옛 버전 디렉터리가 곧 백업. 보존 정책(최근 N개 유지 후 GC)은 후속 결정(§9).

## 상태/진단

```bash
FAMILY=<family> scripts/reindex-dataset.sh status   # current·버전 목록·락 유무
# 서빙 identity: cks.ops.health → name / builder_version / backends.*.data_path(resolved)
#                / alignment{ok, src_commit_*, schema, digest, warnings}
```

- `alignment.warnings`의 "sources.ckg ledger absent"는 **pre-P1 ckv 인덱스** 표시 —
  새 ckv로 재빌드한 버전부터 자동 소거(합의).
- commit/digest 불일치는 `serviceable=false`(fail-loud) — 그 인스턴스는 진단용으로만
  살아있고 쿼리는 거부된다.

## 미결(§9 후속)
- 증분을 서빙본에 직접 vs swap-only — 정책 미결(현재 오케스트레이터는 항상 새 버전 빌드).
- 버전 보존/GC 정책, 도메인지식 커밋별 버전 관리.
