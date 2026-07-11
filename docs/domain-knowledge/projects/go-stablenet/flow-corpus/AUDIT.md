# 코퍼스 정합성 감사 기록

> cks 적재 전에 코퍼스의 정합성·논리적 모호성·취약점을 검토한 기록. 검사는 재현 가능
> (`tools/check_corpus.py`, 코드 주장 검증은 아래 절차).

## 검토 3층

### 1층 — 구조 정합성 (`tools/check_corpus.py`)
JSONL 참조 무결성 + 불변식 양방향 대칭 자동 검사. **초기 4건 발견 → 전부 수정 → 현재 0건.**

| 발견 결함 | 원인 | 수정 |
|-----------|------|------|
| 끊어진 calls `ep-p2p-eth:p2peth-02→p2peth-03` | step을 `03a`/`03b`로 분리했는데 calls 미갱신 | `[p2peth-03a, p2peth-03b]`로 수정 |
| 비대칭 INV-STATE-02 ↔ `finalize-03` | step에 INV 누락 | finalize-03에 INV-STATE-02 추가 |
| 비대칭 INV-STATE-01 ↔ `loopsync-03`,`p2psnap-03` | 파서가 다중 ref 누락 + p2psnap 후추가 | 파서 수정 + p2psnap-03 enforced_at 추가 |
| 비대칭 INV-SEC-01 ↔ `statetx-06` | FeePayer 블랙리스트 step에 INV 누락 | statetx-06에 INV-SEC-01 추가 |

**파서 취약점 2건도 발견·수정**(코퍼스가 아닌 변환기 결함):
- `enforced_at` 한 줄에 ref가 2개면 첫 번째만 포착 → 모두 포착하도록 수정.
- flow:step ref 정규식이 소문자만 허용 → `rounds-TO`(대문자) 누락 → 대소문자 무관으로 수정.

### 2층 — 코드 주장 검증 (file:line ↔ 실제 코드)
load-bearing한 25개 주장을 실제 소스와 대조. **MISMATCH 0건.** 20개 정확,
5개는 ±1~2라인 드리프트(심볼이 그 자리에 존재; 이동 중인 브랜치를 시점 캡처한 결과).
검증된 핵심 지점 예: `commit.go:123`(정족수), `validator/default.go:226`(QuorumSize=ceil(N−(N−1)/3)),
`engine.go:972/1033/1045/1341`(base fee 분배·seal), `state_transition.go:341/572/677/504`,
`validation.go:133/250/284`, `evm.go:140-141`, `native_manager.go:214/246`,
`block_validator.go:142`, `justification.go:95`.
→ **코드 레이어는 견고하다.** 단, 라인은 ±2 근사이므로 cks 조인은 `symbol` 기준이 안전(SCHEMA.md §주의).

후반 경량 flow의 근사 라인 4곳도 정밀화: `legacypool.go:1294`(scheduleReorgLoop)/`1364`(runReorg),
`eth/sync.go:92`(chainSyncer.loop), `snap/sync.go:2480`(OnAccounts 외).

### 3층 — 논리적 모호성
- prose ↔ branches 정합: 분기 225개 모두 `when/then/at` 채워짐, prose가 분기를 반영함(불일치 없음).
- 불변식 전제(`assumes`)가 모두 명시됨 — "전제 없이 진술만 보면 거짓"인 항목(LEDGER-01의 mint/burn,
  CONSENSUS-*의 N≥3f+1) 누락 없음.
- flow 간 중복 서술 대신 `calls`/`called_by` 링크로 단일화(예: p2p-eth→txpool 검증은 rpc flow 재사용).

## 잔여 한계 (결함 아님, 의도/범위)
1. **집계 step**: `ep-discovery:disc-03`은 table.go/dial.go/server.go를 횡단하는 "→연결" 요약 step이라
   단일 라인이 없다. 의도된 추상화.
2. **step = 코드 영역**: 한 step의 `symbol`은 대표 1개이며, 실제로는 함수 구간을 가리킨다. cks의
   세밀한 심볼과 1:N 매핑이므로, 정밀 영향 분석은 cks 호출그래프와 `symbol` 조인으로 보강해야 한다.
3. **커버리지 갭**: behavior-determining 경로 위주다. 미서술 영역 — RANDAO 상세, fork choice/reorg
   내부, EIP-7702 setcode 실행, blob tx 세부, governance 컨트랙트 호출 경로, 에폭 전환 세부.
   같은 형식으로 확장 가능(README 카탈로그에 자리만 두면 됨).
4. **불변식은 부분 추출**: 16개 INV가 모든 가드를 덮지는 않는다(예: 논스 단조성·서명 일반 유효성은
   branches엔 있으나 INV로 승격 안 됨). 안전성 핵심만 INV로 올렸다.

## 재실행
```
python3 tools/build_corpus.py    # 재생성
python3 tools/check_corpus.py    # 구조 검사 (exit 0 = 통과)
```
코드 주장 검증은 시점 캡처라 코드 변경 시 재대조 필요(브랜치 이동에 따라 ±라인 드리프트).
