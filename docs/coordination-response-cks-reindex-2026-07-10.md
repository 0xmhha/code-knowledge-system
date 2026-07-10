# CKS 회신 — 재인덱싱 오케스트레이션 + 무중단 서빙 (2026-07-10)

> Tier 3 (dated snapshot). CKV의 크로스-repo 설계
> `code-knowledge-vector/docs/reindex-migration-design-2026-07-10.md`가 CKS에 요청한
> 4개 결정에 대한 회신. 근거 = cks 코드 현실(`internal/mcp/ops_index.go`,
> `scripts/{serve-cks-http.sh,build-stablenet-dataset.sh,gen-cks-config.sh}`,
> 재시작 비용 실측, 기존 fail-loud 전례). 관련: `remaining-work-2026-07-10.md`(E1/E2).

## 총평

설계 방향(불변 버전 아티팩트 + 검증 게이트 + 원자적 promote, §4.1 blue-green) **수용**.
이미 겪은 사고들(E1 source_root 불일치, 2026-06-09 데이터셋 오염)의 구조적 해법이다.
참고: gen-cks-config.sh의 신레이아웃(`graph-db/`·`vector-db/`) 기본값은 이 설계 §4.1의
선행 반영 — **데이터셋 버전 디렉터리 이관을 P1에 포함하면 E2도 함께 해소**된다.

## Q1 — 재로드 방식 + 무중단 SLA

**결정: (a) config swap + restart 채택. "무중단"이 필요해지면 (b) in-process reload가
아니라 인스턴스-레벨 blue-green으로.**

- 재시작 비용 실측: 엔진 오픈 + intent 앵커 61개 pre-embed ≈ **9~15초**. 현 운영
  (개발 플릿·벤치)에 수용 가능.
- **측정 무결성이 우선**: 세션/벤치 run 도중 데이터셋이 바뀌면 결과가 오염된다.
  "인스턴스 수명 = 단일 불변 데이터셋 버전"이 요구사항이며, in-process hot-reload는
  이 원칙과 충돌한다.
- (b)의 실비용: 임베더 identity가 바뀌면 intent 앵커 재임베드 + composer 스택 재구축
  필요 → 사실상 재시작과 동급. mid-request swap 위험만 추가.
- **인스턴스 blue-green (P5 대체안)**: `serve-cks-http.sh`가 이미 다중 named instance를
  지원 → 새 버전을 새 이름·포트로 기동 → `cks.ops.health`로 identity 검증 →
  `CKS_MCP_URL` 포인터 전환 → 구 인스턴스 stop. 프로세스 내 복잡도 0으로 0-downtime.
- **SLA**: 전환은 세션/벤치 경계에서만, 재시작 ≤15s. 무중단 요구 시 인스턴스 blue-green.

## Q2 — 데이터셋 버전 포인터 소비 주체 + advisory lock 소유

- **포인터 소비**: 오케스트레이터가 promote 시점에 `current`를 쓰고, **cks-mcp는 기동
  시 1회만 resolve**해 구체 버전 경로를 연다. run 중 재해석 금지(불변 원칙). config가
  `current`를 가리켜도 **health는 항상 resolved 구체 버전·sha를 보고** — 어느 인스턴스가
  어느 버전을 서빙하는지 항상 증명 가능(identity 투명성 원칙의 연장).
- **advisory lock 소유**: 락이 보호하는 것은 **빌드 직렬화**뿐(서빙은 불변 버전만 읽어
  무락). 소유 = 오케스트레이션 실행기(아래 Q3에 따라 CKS 소유 — 단 **서빙 프로세스가
  아니라 빌드 파이프라인이 잡는다**). 위치: `knowledge-data/<dataset>/.build.lock`(flock).

## Q3 — 조율 오케스트레이터 소유권

**결정: CKS 소유 동의. 단 형태는 MCP 툴이 아니라 CLI/스크립트.**

- 기존 전례 2개: ① `cks.ops.index`(G8) — ckg/ckv 바이너리를 서브프로세스로 조율(도메인
  corpus export 포함) ② `scripts/build-stablenet-dataset.sh` — STAGE/SKIP 지원 전체
  파이프라인. §3.2 순서(CKG 빌드→CKV 정렬→게이트→promote)는 후자의 버전화+게이트 확장.
- 단서: ckv full build ~10h — 동기 MCP 호출 안에서 불가. 분담:
  - `cks.ops.index` = 가벼운 **증분** 진입점 유지
  - **조율 full 재인덱싱 = CLI**(build-stablenet-dataset.sh 진화 또는 `cks-dataset`):
    lock 획득 → §3.2 실행 → §5.1 게이트 → promote → (재)기동 호출.

## Q4 — stale/mismatch 서빙 전 assert 인터페이스

**결정: cks-mcp 기동 시 alignment assert + 불일치 시 `serviceable=false` fail-loud.**

- 기존 전례 3개의 확장: gen-cks-config source-consistency assertion(`42533a4`) /
  ckv PR#12 임베딩 identity open-refusal / "degraded ⇒ not-serviceable"(2026-06-15 정책).
- 기동 시(buildBackends) 검사:
  1. ckg manifest `{src_commit, schema_version ≥1.19, graph_sha256(신설)}` 로드
  2. ckv manifest `sources.ckg {graph_sha256, src_commit}`(§2.2 신설)와 대조
  3. config `source_root` vs 양쪽 manifest `src_root` 대조 — **현행 E1이 정확히 이
     클래스**(이 assert가 있었으면 기동 시점에 잡혔음)
  4. 불일치 → 기동은 하되 `serviceable=false` + reason(완전 기동 거부보다 운영 진단
     가능, 기존 S5 패턴과 일치). health에 `alignment: {ok, expected/actual graph_sha,
     src_commit_ckg/ckv, source_root_ok, reason}` 블록 노출.
- **인터페이스는 manifest 파일 기반으로 충분** — 새 API 불요(ckv `Manifest()`·ckg
  `LoadManifestSnapshot()` 이미 사용 중). CKS가 상대측에 요구하는 것:
  **CKG manifest `graph_sha256` 공표(P1) + CKV manifest `sources` 블록(P1)**.

## CKS 측 액션 (설계 수용에 따른)

1. **P1 동참**: 데이터셋 버전 디렉터리 이관(=E2 해소), config/serve가 버전 경로 소비 +
   health에 resolved 버전 노출.
2. **P2 게이트 재사용**: 기존 B7 라이브 fixture(`internal/ckgclient/b7_join_live_test.go`,
   canonical 매칭률 ≥90%)를 promote 검증 게이트로 재사용.
3. **Q4 구현**: 기동 시 alignment assert + health `alignment` 블록.
4. **P5(수정안)**: 인스턴스 blue-green 절차 문서화(serve-cks-http.sh 기반).
