# T7 스펙 충분성 검토 (2026-07-13)

> 대상: [`2026-07-12-t7-causal-chain-orchestration-design.md`](./2026-07-12-t7-causal-chain-orchestration-design.md).
> 헤더의 "approved"는 자기 선언이며 **원 요구사항 대조 검토가 없었다** — 본 문서가 그 검토다.
> **판정: 현 스펙은 필요조건이나 불충분. 아래 G1~G5를 반영한 개정 없이 plan/구현으로
> 진행하면 안 된다.**

## 1. 이 스펙이 존재하는 이유 (배경 복원)

T7의 원 요구는 세 문서에 명시돼 있다:

1. **coding-agent 합의 형상** (coordination-response-coding-agent-2026-06-29.md 협의 2):
   > "Phase 2 인과 체인이 돌려줘야 할 형상: flow 위 각 값 V에 대해 **{생산자, 모든
   > 복사본/캐시, 각 캐시의 invalidator 유무, 소비자}** — 즉 **invalidator 주석이 달린
   > produce→store→consume 그래프**. 이 형상이면 analyzer가 find_callers+get_subgraph로
   > 손조립하던 걸 도구 호출로 대체한다."
2. **CKS 자신의 I/O 초안** (CKV coordination §2-R): 출력 = {랭크된 flow 노드, 엣지 종류,
   **invariant 위반 후보**}.
3. **소비자 스킬** (root-cause-lifecycle): "버그 = 소비자가 쓰는 값 == 생산자가 마지막으로
   쓴 값 불변식이 어느 edge에서 깨진 것. **값의 모든 edge를 빠짐없이 열거**하면 깨진 곳을
   못 놓친다." — step2 "모든 복사본·캐시를 **빠짐없이**", step8 "캐시 invalidator 0 = 용의자",
   store 실패모드의 대표 = "**소스가 바뀌어도 갱신 안 됨(stale)**".

즉 T7이 풀어야 할 문제는 정확히 **"stale 캐시류 버그의 깨진 edge를 도구 호출로 찾게
하라"**다. 검증 기준 사례도 이미 존재한다: 5팔 실험의 GasTip 거버넌스 캐싱 버그
(표층 `anzeonTipCap` 영구캐시 + 근본 `SetCurrentBlock`의 상태루트 비교로 헤더 갱신 스킵
— **invalidator가 없는/조건이 잘못된 store** 그 자체).

## 2. 갭 분석 — 현 스펙 vs 원 요구

### G1 (치명) — 커버리지: flow corpus 앵커 한정이라 실검증 사례를 추적 불가
- 스펙 §3이 seed를 flow corpus 앵커(step/entry/invariant/flow)로 **하드 제약**하고,
  §7이 의존성을 FlowClient로 한정(ckg 미사용).
- 그러나 기록된 사실: `get_flow(entry_point="SetCurrentBlock")` → **"no flow found"**
  (coding-agent coordination-outbound-2026-07-05) — **GasTip 버그의 근본원인 지점이
  큐레이션 corpus에 없다.** 현 스펙대로면 T7은 자신이 만들어진 이유인 버그 클래스를
  시연조차 못 한다.
- 원 합의도 "Phase 2 = cross-flow 다중홉 + **CKV/CKG 교차** 오케스트레이션"이었다.
  ckg는 이미 `writes_field`/`calls` 엣지와 control-flow 노드를 보유(§1-R) — corpus에
  없는 값/지점은 **ckg 엣지로 폴백/융합**해 체인을 이어야 한다. (ckv/ckg *변경*은
  non-goal이 맞지만 ckg *사용*은 변경이 아니다.)

### G2 (핵심 산출물 누락) — invalidator 주석이 없다
- 합의 형상의 핵심 = "**각 캐시의 invalidator 유무**" 주석. step8이 이를 직접 소비
  ("invalidator 0 = 용의자").
- 현 `CausalLink`에는 invalidator 개념이 전무하다. store로 분류된 링크에 대해
  **무효화/갱신 site 열거 + 부재 표시**(`invalidators: []` = 용의자)가 있어야
  "깨진 edge 찾기"가 도구 출력만으로 성립한다. (탐색원: 동일 대상에 쓰는 다른 step,
  flow branches, ckg write-sites, get_invariant_enforcement.)

### G3 (완전성 계약 부재) — "모든 복사본·캐시 빠짐없이"가 스펙에 없다
- lifecycle의 제1 요구는 **전수 열거**(한 경로만 보면 오진). 현 스펙은 단일 seed BFS +
  MaxSteps 24 + composer ≤2 chains + dedup — "V의 **모든** store site 열거"라는 목표
  서술도, 못 채웠을 때의 **완전성 보고**(무엇이 잘렸는지)도 없다. `Truncated: true` 한
  비트로는 analyzer가 "불완전 열거=오진 위험"을 판단할 수 없다.

### G4 — invariant 위반 후보가 출력에서 빠짐
- CKS 자신의 초안(§2-R)이 약속한 출력 요소. flow step은 `Invariants[]`를 이미 갖는데
  `CausalLink`가 버린다. 체인 위 invariant + enforcement 부재 교차는 G2와 직결.

### G5 — 문제 기반 수용 기준 부재
- §9 수용 기준이 구조적("ordered chain을 반환")일 뿐, **실버그 검증이 없다**.
  수용 기준에 명시해야 한다: *GasTip 캐싱 버그 시나리오에서 T7 출력이 깨진 edge
  (갱신 스킵되는 store)를 지목한다* — G1을 해소해야만 통과 가능한 기준이며, 그래서
  이 기준이 스펙의 방향을 강제한다.

### G6 (경미) — 값 동일성이 문자열 포함 매칭뿐
- 같은 값이 이름을 바꿔 흐르는 경우(`gasTip` → `anzeonTipCap` → `currentBlock.header`)
  string containment로는 체인이 끊긴다. 최소한 별칭 처리 방침(vocab/glossary 연동 또는
  ckg 엣지로 보강)을 명시해야 한다.

## 3. 판정과 다음 단계

- **판정**: 현 스펙(§4~§9)은 "flow corpus 위의 단일 값 BFS 조립"으로는 완결적이지만,
  그것은 **원 문제의 부분집합**이다. G1(커버리지)·G2(invalidator)·G3(완전성)이 빠진
  채 구현하면 "동작하지만 진단에 못 쓰는" 도구가 된다.
- **다음 단계**: ① 스펙 개정 — G1~G5 반영(§3 제약 완화: corpus 우선 + ckg 폴백/융합,
  CausalLink에 invalidator/invariants 필드, 전수 열거·완전성 보고 계약, GasTip 수용
  기준) ② 개정 스펙 검토·승인 ③ 그 후에 plan 작성 → 구현.
- 스펙 헤더 Status를 "draft — under review"로 정정함(자기 선언 approved 회수).
