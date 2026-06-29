# 핸드오프: cks 평가 — 남은 작업 + 전체 컨텍스트

> 작성일 2026-06-12. 최종 갱신 2026-06-29(코드 대조 후 상태 최신화).
> 이 문서 하나로 다른 세션/나중에 작업을 재개할 수 있도록 **컨텍스트 누락 없이** 정리한다.
>
> **2026-06-29 갱신 요약**: 2026-06-12에 "미구현"으로 적었던 harness 작업은 코드 대조 결과
> 대부분 완료됐다 — §5.1(a) bug-cycle 루프, §5.1(b) compare.py 총비용 지표, §5.1(c) 태스크
> 다양화(STABLE-0001~0009), §5.2 jira-gateway 재빌드 모두 **완료**. **진짜 남은 핵심은 §5.1(d)
> = A/B/C 벤치를 실제로 end-to-end 완주시켜 §2 명제를 데이터로 판정하는 것 하나**다(현재 실제 ABC
> 셀은 ANALYSIS/DESIGN에서 중단, EVALUATION_PASS 0건, 비교 리포트 미생성).

---

## 0. 한 줄 요약
**cks(code-knowledge-system = ckg 그래프 + ckv 벡터를 합성)가 coding-agent 파이프라인의 "옳은 수정까지의 총비용"을 실제로 줄이는지** 를 A/B/C 벤치로 검증하는 게 핵심 미완 작업이다. **harness는 완성**(bug-cycle 루프 + compare.py 총비용 지표 + 다중 태스크 매니페스트 모두 구현). **남은 핵심은 그 harness를 실제로 end-to-end 완주시켜 §2 명제를 데이터로 판정하는 것**(§5.1d).

---

## 1. 배경 / 큰 그림
- **대상 프로젝트**: go-stablenet (geth 포크, WBFT 합의 + Anzeon 동적 가스팁 + Solidity 시스템컨트랙트).
- **cks/ckv/ckg**: go-stablenet 코드를 인덱싱한 지식 데이터셋.
  - **ckg**: 코드 그래프(심볼·호출·git이력). `code-knowledge-graph` repo.
  - **ckv**: bge-m3 벡터(의미 검색). `code-knowledge-vector` repo.
  - **cks**: 둘을 token-budgeted EvidencePack으로 합성하는 오케스트레이터(자체 LLM·DB 없음). `code-knowledge-system` repo.
- **coding-agent**: cks를 써서 자율로 (분석→구현→평가) 하는 플러그인. `coding-agent` repo. planner가 cks MCP를 사용.
- **검증하려는 명제**: *"cks 도입 = 토큰 ↓ + 정확도 ↑"* 가 실제로 성립하는가?

## 2. ⭐ 방법론 정정 (가장 중요 — 절대 누락 금지)
- **단일 싸이클(분석 단계) 토큰만 보면 cks는 더 비싸 보인다** (다각도 검색이라 입력 토큰 큼). 이걸로 결론내면 틀린다.
- **올바른 지표 = "옳은 수정에 도달할 때까지의 총비용"**:
  - cks의 값 = 완전한(다각도) 정보 → **첫 수정을 옳게** → 부적절 수정/사이드이펙트로 인한 **재작업(bug-cycle) 감소** → *총* 토큰 ↓ + 정확도 ↑.
  - B/C(cks 없음)는 분석은 싸지만 → 불완전한 정보 → 사이드이펙트/오수정 → EVALUATION_FAIL → bug-cycle 반복 → **총비용 ↑**.
- 따라서 측정은 **Σ(분석+구현+평가) × bug-cycle 수** 와 **사이드이펙트 적발/수정 정확성**.

## 3. A/B/C 모드 정의
같은 태스크를 모델 고정(opus-4.7)으로 정보 공급 방식만 바꿔 비교:
| 모드 | planner 정보원 | 에이전트 |
|---|---|---|
| **A_cks** | cks MCP(semantic+graph+domain) | 실제 `planner` |
| **B_code_only** | grep/glob/read만 | `bench-planner-codeonly` |
| **C_code_skills** | grep/read + 이해 스킬(stablenet-context 분류 + stablenet-invariants backstop) | `bench-planner-skills` |

implementer·evaluator는 **공유**(모드 불문 동일). → planner의 정보 regime만 격리 비교.

---

## 4. 현재 구현 상태 (2026-06-29 코드 대조 결과)
### ✅ 구현·완료된 것
- `/coding-agent:bench` 커맨드 — `coding-agent/plugin/commands/bench.md`
- orchestration 스킬 — `coding-agent/plugin/skills/bench-orchestration/SKILL.md`
  - **bug-cycle 루프 구현됨**(step e, L137–158): `max_eval_cycles`(기본 3)까지 `EVALUATION_FAIL→모드별 재진입(A/B/C planner)→재평가`, 싸이클별 토큰·결과·failure 누적, `run-meta.bug_cycles` 회계.
- 모드 B/C planner — `coding-agent/plugin/agents/bench-planner-{codeonly,skills}.md`
- 비교 스크립트 — `coding-agent/bench/compare.py`(엔트리) + `coding-agent/bench/lib/{collect,report,usage,capture}.py`
  - **총비용/cycle 지표 구현됨**: `collect.py`의 `bug_cycles`·`side_effect_failures`(회귀-클래스 실패 마커 집계), `report.py`의 `total_tokens(Σ across cycles)`·`avg_bug_cycles`·`side_fx`·`correct(최종정확성)` 3-way 표/JSON/CSV.
- 매니페스트 스키마/가격 — `coding-agent/bench/manifest.schema.json`, `bench/prices.json`
- **다중 태스크 매니페스트** — `STABLE-0001~0009`로 확장, 6종(`example`, `stable-0005-abc`, `stablenet-abc-phase1/2/3`, `stablenet-pr77`).
- jira-gateway 바이너리 재빌드(`bf9c0e1` `/search/jql` 수정 반영, 빌드 6/9 12:06) — §5.2.

### ❌ 빠진 것 (핵심 — §2 방법론을 측정하려면 반드시 필요)
1. **end-to-end 완주 이력 0 (★유일하게 남은 핵심)**: 실제 ABC 셀(`go-stablenet/.coding-agent/bench/{abc,abc2}/`)이 전부 **ANALYSIS/DESIGN 단계에서 중단**. **EVALUATION_PASS 도달 셀 없음**, `comparison.{json,md,csv}` 리포트 미생성. → 즉 harness는 완성됐으나 **실제로 완주시켜 본 적이 없다**.

→ **결론: harness(bug-cycle 루프 + 총비용 지표 + 다중 태스크)는 완성. 남은 건 그걸 실제로 end-to-end 완주시켜 §2 명제를 데이터로 판정하는 실행/분석 1건(§5.1d, §8 step 5~6).**

---

## 5. 남은 작업 (상세)

> 2026-06-29 갱신: (a)(b)(c)와 §5.2 재빌드는 코드 대조 결과 **완료**. 남은 핵심은 **(d) 실제 완주 실행 + 분석**.

### 5.1 [핵심·大] full A/B/C 벤치 — "총비용" 측정 완성
**목표**: §2 지표(총 토큰 across cycles, bug-cycle 수, 사이드이펙트, 정확성)로 A/B/C 비교.

**(a) ✅ 완료 — bench-orchestration bug-cycle 루프** — `coding-agent/plugin/skills/bench-orchestration/SKILL.md` (step e, L137–158)
   - `EVALUATION_FAIL`이면 `config.max_eval_cycles`(기본 3)까지 **모드별 planner(A/B/C)로 재진입→재평가** 반복, 싸이클마다 토큰·결과·failure 누적, `run-meta.bug_cycles` 회계 구현됨. (orchestrator.md 상태기계 재사용.)

**(b) ✅ 완료 — compare.py 총비용 지표** — `coding-agent/bench/compare.py` + `bench/lib/{collect,report}.py`
   - `collect.py`: `bug_cycles`, `side_effect_failures`(회귀-클래스 마커 집계). `report.py`: `total_tokens(Σ across cycles)`, `avg_bug_cycles`, `side_fx`, `correct(최종정확성)` 3-way 표/JSON/CSV. `--sessions`로 실토큰 합산.

**(c) ✅ 완료 — 태스크 다양화** — `coding-agent/bench/manifests/`
   - `STABLE-0001~0009`로 확장, 매니페스트 6종(`example`, `stable-0005-abc`, `stablenet-abc-phase1/2/3`, `stablenet-pr77`). PR#77(Anzeon)은 여러 케이스 중 하나로만.

**(d) 🔴 미완 (★유일하게 남은 핵심) — end-to-end 완주 검증 + 본 실행 + 분석**
   - 현 상태: 실제 ABC 셀(`go-stablenet/.coding-agent/bench/{abc,abc2}/`)이 ANALYSIS/DESIGN에서 중단, EVALUATION_PASS 0건, 비교 리포트 미생성.
   - 할 일: 한 셀이라도 ANALYSIS→…→**EVALUATION_PASS 완주** 확인 → 전체 매니페스트 실행 → compare.py로 §2 명제 판정.
   - 실행: autopilot 세션(§6.2)에서 `/coding-agent:bench <manifest>` (batch_size만큼 돌고 checkpoint) → `--continue` 반복 → 끝나면 compare.py → **실행 후 throwaway 브랜치 정리(§6.3)**.

### 5.2 jira-gateway (jira 실사용 시에만)
- **✅ 바이너리 재빌드 완료**: `tools/jira-gateway-mcp/bin/jira-gateway-mcp`가 수정 커밋 `bf9c0e1`(6/9 12:03, `/rest/api/3/search/jql` — legacy `/search`는 HTTP 410, CHANGE-2046) 직후인 6/9 12:06 빌드, 소스에 `/search/jql` 반영 확인.
- **⚠️ JIRA_API_TOKEN 미설정**: `~/.config/coding-agent/jira.env` 에 아직 `CHANGE-ME` placeholder 1개. (JIRA_BASE_URL=https://wemade.atlassian.net, JIRA_USER_EMAIL=mhha@wemade.com 은 설정됨.) jira 안 쓰면 무시.

### 5.3 (선택)
- ckv↔ckg 커밋 정렬 점검 — graph.db·vector.db 둘 다 2026-06-25 재빌드됨(현 데이터셋). 커밋 정렬은 미검증(선택).
- reset류 심볼 케이스별 정밀 재판정(ckg line-history, 다른 세션으로 이관됨 — 여기선 불필요).
- Solidity 심볼 인덱싱(`--lang go,sol` 재빌드) — 현재 .sol은 코드심볼 미인덱싱(이력 hunk만). 미착수(선택).

---

## 6. ⚠️ 실행 환경 / 주의사항 (반드시 숙지)
1. **A/B/C 벤치는 go-stablenet을 변경한다**: 모드·태스크별 브랜치 생성 + 커밋 + (evaluator가) chainbench 재빌드/e2e. **무겁다**(모드 3 × 태스크 N × 전체 파이프라인 × bug-cycle).
2. **bypassPermissions 세션 필요**: implementer의 go-stablenet 편집이 무프롬프트로 돌려면 **autopilot 런처로 go-stablenet에서 세션 기동**: `code-knowledge-system/scripts/coding-agent.sh`. (다른 디렉터리에서 띄운 세션은 편집 권한 프롬프트로 멈춤.)
3. **🔴 데이터셋 오염 방지 (최重要)**: 벤치가 만든 **throwaway 브랜치/커밋을 반드시 정리**해야 한다. 안 그러면 다음 CKG 재빌드 때 가짜 코드·커밋이 데이터셋에 유입(2026-06-09 실제 발생). 정리: 캐노니컬 브랜치로 checkout → `git branch -D <테스트브랜치>` → `git reflog expire --expire-unreachable=now --all && git gc --prune=now` → 재빌드는 임시 dir→검증(가짜심볼/커밋 0)→swap. (메모리: cks-dataset-test-branch-contamination)
4. **`/reload-plugins` ≠ MCP 재시작**: 재빌드한 graph를 라이브 cks MCP가 서빙하려면 **세션 재시작**(`exit`→`claude --continue`). `/reload-plugins`는 MCP 프로세스를 안 죽임. cks `ops.freshness`는 인메모리 staleness를 못 잡음. (메모리: cc-reload-plugins-no-mcp-restart)
5. **PR#77 편향 금지**: PR#77은 *평가용 한 케이스*일 뿐. production 코드/프롬프트(planner/evaluator/ckg)는 감사 결과 편향 없음(일반 원칙). 편향 위험은 **평가 셋(매니페스트)이 단일 태스크인 점** → §5.1(c)로 해소.

---

## 7. 핵심 경로 / 명령 레퍼런스
**데이터셋 (정규화 완료: 둘 다 data/ 하위)**
- ckg 그래프: `code-knowledge-system/data/ckg-stablenet/graph.db`
- ckv 벡터: `code-knowledge-system/data/ckv-stablenet/vector.db`
- cks config: `code-knowledge-system/cks-stablenet.yaml` (ckg/ckv 경로 지정)
- MCP env: `CKS_MCP_BIN=.../code-knowledge-system/bin/cks-mcp`, `CKS_CONFIG=.../cks-stablenet.yaml` (`~/.claude/settings.json`)

**빌드/운영 스크립트** (`code-knowledge-system/scripts/`)
- 데이터셋 빌드: `build-stablenet-dataset.sh` (`CKG_OUT=data/ckg-stablenet`, `CKV_OUT=data/ckv-stablenet`; STAGE/SKIP_CKV 지원, ckv는 ~10h)
- config 생성: `gen-cks-config.sh`
- autopilot 런처: `coding-agent.sh`

**벤치 (coding-agent repo)**
- 실행: `/coding-agent:bench coding-agent/bench/manifests/<manifest>.json` → `--continue` 반복
- harness: `plugin/skills/bench-orchestration/SKILL.md`, `plugin/commands/bench.md`, `plugin/agents/bench-planner-{codeonly,skills}.md`, `bench/compare.py`, `bench/manifest.schema.json`, `bench/prices.json`
- 셀 산출물: `go-stablenet/.coding-agent/bench/<experiment>/cells/<task>__<mode>/` (state.json, analysis/plan/design, test-report.md, run-meta.json, logs/agent-transcript.jsonl)

**ckg 개선 (이미 main 머지 — 다른 세션에서 추가 검증/수정 중)**
- PR #18: git `-L` line-history 귀속 (node_prs recall 57%→86%)
- PR #19: `--temporal-depth` 플래그(커밋/Hunk temporal cap 노출, 기본 10)

**관련 검증 산출물**: `code-knowledge-graph/eval/stablenet/func-verify/` (CKG 기능검증 보고서 일체 — 완료됨)

---

## 8. 재개 체크리스트 (순서대로)
> 2026-06-29 갱신: step 3(태스크 다양화)·4(harness 보강)는 완료 → 취소선. 이제 **1→2→5→6→7**만 남음.

1. **go-stablenet 상태 확인** — 다른 세션 수정 완료? 브랜치 깨끗?(throwaway 없음, §6.3)
2. **데이터셋 최신화** — 코드 바뀌었으면 ckg(+필요시 ckv) 재빌드(임시 dir→검증→swap) → **세션 재시작**(§6.4). (현 데이터셋은 2026-06-25 빌드.)
3. ~~**5.1(c) 태스크 다양화**~~ — ✅ 완료(STABLE-0001~0009, 매니페스트 6종).
4. ~~**5.1(a)(b) harness 보강**~~ — ✅ 완료(bug-cycle 루프 + compare.py 총비용/cycle 지표).
5. **5.1(d) 1셀 완주 검증** → autopilot 세션(§6.2)에서 full 실행. ← **여기부터가 실제 남은 일**
6. **분석** — compare.py 리포트로 A/B/C 총비용·bug-cycle·사이드이펙트·정확성 비교 → §2 명제 판정.
7. (벤치 후) **go-stablenet 브랜치 정리**(§6.3) — 데이터셋 오염 방지.

---

### 부록: 현재까지 완료된 것 (참고)
- 데이터셋 빌드(ckg/ckv/cks) + 헬스체크 + 수동 매뉴얼.
- coding-agent 플러그인 마켓플레이스 등록 + 자율화(자유텍스트 entry, 무프롬프트, auto_merge 게이트) + 0.1.10.
- CKG 기능 검증(키워드→코드/히스토리): C1 코드위치 100%, C2 히스토리 근본개선(PR#18) → **완료(다른 세션 이관)**.
- 데이터셋 오염 1회 복구 + DB 레이아웃 정규화(ckv→data/).
- PR#77 편향 감사(production clean).
