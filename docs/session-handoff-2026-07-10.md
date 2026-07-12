# 세션 핸드오프 — reindex P1 출하 + 발견 사실 (2026-07-10)

> 이 문서 하나로 다른 세션이 컨텍스트 손실 없이 이어갈 수 있도록: **완료된 것 / 남은 것 /
> 이 세션에서 알게 된 사실(문서에 없던 것)** 을 기록한다. 작업 브랜치:
> `docs/retire-ckg-node-id`. 관련: [`remaining-work-2026-07-10.md`](./remaining-work-2026-07-10.md)(감사),
> [`coordination-response-cks-reindex-2026-07-10.md`](./coordination-response-cks-reindex-2026-07-10.md)(협의),
> [`ops-blue-green-reindex.md`](./ops-blue-green-reindex.md)(운영 절차).

---

## 1. 이 세션에서 완료된 것 (커밋 순)

| 커밋 | 내용 |
|---|---|
| `9a960f9` | 문서↔코드 감사(E1~E5 오류 / M1~M7 미완료) — remaining-work-2026-07-10.md |
| `3d9be48` | 감사 재검증(M1→M1′ 전환, M5 완화, M7 진행) |
| `d03bfde` | CKV reindex 설계 4결정 회신(재시작 전환·인스턴스 blue-green·CLI 오케스트레이터·기동 assert) |
| `1a9ccde` | CKV P1 랜딩 동기화 기록(sources 원장 `8816915` + 상호 alignment `e49c19b`) |
| `593c656` | **reindex P1-1/P1-2**: 기동 시 데이터셋 symlink 1회 resolve + ckg↔ckv alignment assert(2단계 심각도) + health `alignment` 블록 + serviceable 게이트 (+단위테스트 8케이스) |
| `d3f24ec` | **reindex P1-3/P1-4**: `scripts/reindex-dataset.sh`(lock→빌드→버전디렉터리→5단계 게이트→원자 promote) + `docs/ops-blue-green-reindex.md` |

**라이브 검증 완료**: alignment assert가 실결함(E1)을 기동 시 포착 → source_root 정정 →
`alignment.ok=true`. 오케스트레이터 gate 5단계를 pr-77-2 실산출물로 전부 통과
(validate 0 errors / alignment / chunk_count 15,909 / **B7 매칭률 게이트** / audit soft).
promote·롤백·sentinel 거부·status는 scratchpad KD에서 검증.

## 2. 현재 라이브 상태 (이 세션 종료 시점)

- **서빙**: `cks-stablenet` 인스턴스, 172.20.82.90:8080, **alignment 코드 포함 바이너리**.
  health: `serviceable=true`, `alignment.ok=true`, 경고는 `sources.ckg ledger absent`
  1건뿐(pre-P1 ckv 인덱스 — 재빌드 시 자동 소거, 합의됨).
- **데이터셋**: 레거시 flat 레이아웃 `knowledge-data/pr-77-2/{graph.db, ckv/}` (@0bf2f4d1b,
  schema 1.23). 버전 레이아웃(`@<ver>` + `@current`)은 **첫 오케스트레이터 run이 생성**.
- **config**: `cks-stablenet.yaml`(gitignored) — `source_root`가
  `cks-seminar/test/vector-db-5`로 정정됨(E1 종결).
- **go.mod**: `replace ckv => ../code-knowledge-vector`(커밋됨, `3e7c22d`) — 로컬 ckv
  HEAD로 빌드 중.

## 3. 이 세션에서 알게 된 사실 (문서에 없던 것 — 컨텍스트 보존용)

### 데이터셋 원본 토폴로지 (E1 조사에서 발견)
- **pr-77-2의 ckg 그래프 원본** = `~/Work/github/test/analysis-test-3` (@0bf2f4d1b)
- **pr-77-2의 ckv 인덱스 원본** = `~/Work/github/cks-seminar/test/vector-db-5` (@0bf2f4d1b)
- 즉 **두 백엔드가 서로 다른 체크아웃**(같은 커밋)에서 빌드됨 — 2단계 심각도 설계
  (같은 커밋·다른 경로=warning)가 정확히 이 케이스를 위해 존재. config `source_root`는
  ckv 원본(vector-db-5)을 가리키게 정정함(body fetch 대상).

### 8080 포트 점유 사건 (운영 교훈)
- 다른 세션이 16:16에 구버전 바이너리로 8080에 자체 인스턴스를 띄웠고, serve 스크립트의
  pidfile은 그 프로세스를 모름 → `restart`가 죽은 stale pidfile만 정리하고 새 프로세스는
  **bind 실패로 조용히 죽었으며**, health는 계속 구버전을 응답했다(builder_version으로 발각).
- **교훈**: 배포 확인은 반드시 `health.builder_version`으로. 포트 소유자 확인은
  `lsof -nP -iTCP:8080 -sTCP:LISTEN`. 인스턴스 기동은 serve 스크립트 한 경로로 통일할 것.

### 수치/능력 (실측)
- cks-mcp 재시작 비용 ≈ **9~15초** (엔진 오픈 + intent 앵커 **61개** pre-embed) — 재시작
  전환 SLA의 근거.
- `ckg`에 `validate`(무결성: dangling edge/스키마 불변식)와 `audit`(파일셋 대조) 서브커맨드
  존재 — 게이트 1/5단계로 채택.
- `ckv build` 주요 플래그: `--ckg <dir>`(canonical 정렬), `--flow-corpus`, `--docs`(반복),
  `--include-pr-history`, `--files-from`, `--policy`, `--model-name`(qwen3-embedding:0.6b|4b
  옵션 존재 — M4/reindex-B enabler).

### 3자 합의 스펙 (ckv/ckg 문서에 확정, cks 구현이 따름)
- **`<ver>` = `<short-commit>-<graph_digest[:8]>`**; digest = nodes+edges 정렬 sha256
  (**temporal 제외** — temporal-only 재빌드에서 digest 불변이라 false-positive 방지).
- 같은 커밋에 **다중 임베딩 모델 공존이 필요해지면 `<ver>`에 `-<emb8>` 추가**(현재 deferred;
  임베딩 identity는 vector-db manifest + PR#12 open 게이트가 담당).
- 역할 분담: **CKV=권위 키(src_commit+digest) 검증 / CKS=경로(source_root) 검증(warning 몫)**
  — 상호보완, 동일 2단계 심각도.
- `current` symlink swap은 **오케스트레이션 소관**(CKG Q2는 버전 디렉터리 기입만).

### 외부 진행 (같은 날 타 세션/타 repo)
- CKV: sources 원장(`8816915`) + 자체 alignment 감지(`e49c19b`) 랜딩. `ReadCoords`가 CKG
  digest를 선제 소비(공표 즉시 자동 채움).
- CKG: digest 정의·Q1(공표)·Q2(버전 디렉터리) 착수 가능 상태 — **Q1 공표가 다음 외부 이벤트**.
- coding-agent: 0.1.50까지 진행(오염 게이트, PR-77 워크드예시 leakage 제거 등) — 상세는
  coding-agent WORKLIST/HANDOFF-phase2 참조.

## 3.5 저녁 추가 갱신 (digest 배선 + 라이브 상태 변동)

- **CKG Q1 랜딩 확인**(`1d8eb5b`/`3bcefd2`/`d53c97d`): `graph_digest`가 manifest.json과
  in-db manifest 테이블 양쪽에 공표됨. **CKS 배선 완료**(`d0038cf`): ckgclient
  ManifestSnapshot→Health→startup assert→`ops.health`(backends.ckg.graph_digest +
  alignment.graph_digest_actual). go.mod ckg를 digest HEAD로 bump. 오케스트레이터의
  `<ver>` 명명·게이트 digest 비교는 필드명 일치로 자동 동작.
- **⚠️ 라이브 상태 변동(17:03~17:09, CKG 세션의 재구성)**: `pr-77-2/`가
  **fresh digest-보유 그래프(635MB, digest `4be26516…`, @0bf2f4d1b, schema 1.23)로
  교체**되고 **구 벡터 인덱스(ckv/)와 env 파일은 제거됨**. → 서빙 인스턴스는
  ckv.Open 실패로 **degraded(fail-loud, 정확한 동작)** — alignment도 "ckv manifest
  missing" 경고. **CKV 재빌드가 들어와야 서빙 복구**. 이는 in-place 변경 위험의 실증
  사례이기도 함(blue-green이 막으려는 것) — 다음 재인덱싱부터는 버전 디렉터리로.
- 즉 **다음 이벤트 = CKV 재빌드**(타 세션 진행 예상). 완료 시 우리 서버 재시작 →
  digest 양측 비교가 처음으로 활성화(assert commit+digest 강화) + ledger 경고 소거 확인.

## 4. 남은 작업 (우선순위 — §3.5 라이브 변동 반영, 2026-07-10 저녁)

> **Superseded by [`remaining-work.md`](./remaining-work.md)** (consolidated, living).
> The P0–P4 table below is preserved as the snapshot at ship time; consult the
> consolidated file for current status.

> ⚠️ 서빙이 **degraded**(pr-77-2 벡터 인덱스 부재)인 상태가 우선순위를 지배한다.

| 순위 | 작업 | 상태/선행 |
|---|---|---|
| **P0** | **pr-77-2 재빌드로 서빙 복구** — `reindex-dataset.sh run`(FAMILY=pr-77-2, SRC=vector-db-5). 한 번에: 서빙 복구 + E2 종결(버전 레이아웃 `@<commit8>-4be26516`) + sources 원장 전파(ledger 경고 소거) + **digest 양측 비교 첫 활성화**(CKV 체크포인트) + 오케스트레이터 첫 실전 | ⚠️ **누가 돌릴지 조율 필요** — CKG 세션이 그래프를 방금 재구성했으므로 CKV 재빌드가 타 세션에서 진행 중일 수 있음. ckv full build 수 시간 |
| **P1** | 필러(P0 빌드 중 병행, 독립): E4/E5 문서 정정(symbol-identity §7 상태·coordination T1 표기) + M7 anchors `kind:` 37건 | 즉시 가능 |
| **P2** | P0 직후: 서버 재시작 → digest 비교·ledger 소거·alignment.ok 검증 → CKV 통지 → **M2 cks 팔 벤치**(마지막 팔) | **M2는 P0 선행 필수** — degraded 상태에서 측정 불가(이전 "즉시 가능" 판정은 §3.5 변동으로 무효) |
| **P3** | M1′(ckv push + retire 안정화 시 replace 제거·pin 복귀, main 머지 전 필수) / M6 retire(현 브랜치 목적, ckv 컬럼 제거 선행) | 세션 조율 |
| **P4** | M3(T7, M2 동결 충돌 회피) / M4(reindex-B — qwen3 옵션 준비됨, CKV 주관) / M5(전용 도구 — CKV Engine 대기) | 후속 |

## 5. 재개 빠른 확인 명령

```bash
scripts/serve-cks-http.sh status && curl -s ... cks.ops.health   # builder_version + alignment 확인
FAMILY=pr-77-2 scripts/reindex-dataset.sh status                 # 버전/락 상태
git -C ../code-knowledge-vector status -sb                       # M1′ (ahead?) 확인
```
