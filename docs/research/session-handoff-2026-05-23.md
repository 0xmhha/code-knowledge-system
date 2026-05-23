# Session Handoff — cks/ckv/ckg 통합 점검 세션 (2026-05-22~23)

> **목적**: 본 세션의 *발견 + 결정 + 산출물* 을 한 곳에 묶어 다른 세션이 cold
> start 로 이어 받을 수 있게 한다. 본 문서만 읽으면 컨텍스트 100% 복원.
>
> **세션 트리거**: 사용자가 "resume" → ckg/ckv 개발 상태 점검 요청 → cks 가
> 의존하는 두 backend 의 *evaluation 완성도* + *통합 작업 차단 요소* 식별 →
> 사용자 명세 추가 (vocabulary bridge, multi-stage eval, fixture 점진 학습,
> PR-aware, 3-leg BM25 임시) 로 *3 repo 동시 정리 작업* 으로 확장.

---

## 0. TL;DR (60 초 핸드오프)

1. **사용자 명세가 진화**: ckv 가 vector-only 가 아니라 **vocabulary bridge** (한국어/모호 → 코드 정확 키워드) 역할 추가. ADR-003 (CKV vector-only) 영구 결정 *유보* — 3-leg BM25 임시 적용 후 데이터로 결정.

2. **3 repo 모두 정리 문서 작성 완료**:
   - `cks/docs/research/knowledge-data-best-practice-2026-05-22.md` (570 줄) — 보고서 R-A/B/C/D
   - `ckv/docs/evaluation-design-2026-05-22.md` §10 (+493 줄) — 사용자 명세 + ckv-NEW-1~9
   - `ckg/eval/stablenet/CKS-INTEGRATION-2026-05-23.md` (478 줄) — ckg-NEW-1~9 + Stage B 명세

3. **모두 uncommitted** — 사용자가 직접 검토 후 commit 결정 (사용자 명시).

4. **다음 세션 시나리오**: ckv prregress 작업자 / ckg 작업자 / cks 통합 작업자 *3 갈래로 병렬 진행 가능*. 각자 자기 repo 의 문서 §0~§3 정독 후 Day 1 작업 시작.

5. **핵심 발견**: ckv `internal/eval/prregress/` 가 *사용자 명세 multi-stage evaluation 의 80% 이미 구현* (1710 LOC, base_sha checkout → plan generation → diff 비교). 신규 작업이라고 생각했던 게 *측정 분해 + fixture 확장* 으로 좁혀짐.

---

## 1. 세션 흐름 (시간순)

| 단계 | 사용자 명세 / 발견 | 산출 |
|---|---|---|
| 1 | "resume" → ckg/ckv 점검 요청 | 점검 결과 보고 (ckg dirty, ckv 메모리 stale, dogfood loop 양호) |
| 2 | ADR-003 ↔ R4 (BM25 in MCP flow) 충돌 발견 | D1 (BM25 위치) 결정 요청 |
| 3 | 사용자 "ckv/ckg/cks 모두에 BM25 임시 + best practice 알고싶다" | R-A/B/C/D 보고서 작성 → cks repo |
| 4 | 사용자: vocabulary bridge 본질 명세 + 점진 학습 + ADR 결재는 본인 + 장애 재현 dev 브랜치 활용 | ckv §10 추가 작성 |
| 5 | 발견: ckv `internal/eval/prregress/` 1710 LOC, 이미 multi-stage 패턴 구현. backlog 의 "다른 세션 진행 중" 마킹이 *해당 세션 산출물* 이라고 판명 | fixture 12 개 자동 매핑 명세 (PR 단위) |
| 6 | 사용자: ckv 만이 아니라 ckg 도 동일 수준 정리 요청 | ckg `eval/stablenet/CKS-INTEGRATION-2026-05-23.md` 작성 |
| 7 | 사용자: "여기까지 정리작업을 모두 문서로 정리" | 본 문서 (마스터 핸드오프) |

---

## 2. 사용자 명세 진화 — R1~R8 + R9~R13

`stablenet-ai-agent` HLD + ckv evaluation-design (5/22 원본) 의 R1~R8 외에,
본 세션에서 사용자가 명시 추가:

| 신규 요구 | 본질 | 영향 받는 repo |
|---|---|---|
| **R9: Vocabulary bridge** | 사람의 모호/한국어 표현 → 코드 정확 용어 변환. ckv 가 *semantic retrieval* + *vocabulary mapping* 둘 다 책임 | ckv 핵심, ckg graceful, cks fusion |
| **R10: Multi-stage evaluation** | E1 intent / E2 location / E3 plan / E4 code — 단계별 분해 측정 | ckv prregress (이미 80% 구현), ckg Stage B, cks Stage C |
| **R11: 점진 fixture 학습** | F1 (interactive CLI) / F2 (MCP feedback) / F3 (web UI) / F4 (PR) — 실 사용 중 fixture 누적 | ckv F1, cks F2 흡수 |
| **R12: PR-aware retrieval** | "왜 이렇게 고쳤어?" 류 — PR description corpus + symbol-level PR breadcrumb. A+B+C 모두 채택 | ckv PR corpus (NEW-3), ckg breadcrumb (NEW-2~4), cks fusion (B) |
| **R13: 3-leg BM25 임시** | ckv/ckg/cks 모두에 BM25 임시 적용 → 측정으로 영구 위치 결정 (ADR-006) | 세 repo 모두. ADR-003 유보 상태 |

### 사용자 검증된 의향 (대화 답변에서 명시)

| 의향 | 근거 (사용자 표현) |
|---|---|
| **평가가 최우선** | "evaluation 반드시 go-stablenet 코드로", "평가 개선해야" |
| **영구 결정 늦춤** | "BM25 모두에 임시 적용 + 평가로 결정" |
| **vocabulary bridge 진짜 가치** | "0번 블록 → genesis" 예시 + 한국어/모호 → 코드 키워드 명세 |
| **사용자 본인이 ADR 결재** | "BM25 위치, corpus 범위 등 양손을 차안도에서 떼고 결정" |
| **장애 시나리오 = dev 브랜치 git log** | "git 전체가 아니라 PR 단위로 보라" → 40 commit → 12 fix PR |
| **fixture는 사용자 큐레이션** | "초기 버전 이후 실 사용하며 추가/학습 가능해야" |
| **작은 단위부터 검증** | "ckv → ckg 단독 평가 후 cks 통합" |

---

## 3. 핵심 발견

### 3.1 ckv 메모리 10일 stale → 실제 87.5% 완성

이전 메모 (2026-05-12): "MCP stub, fake adapter 로 병행".
실제 (2026-05-22 검증): MCP 4 tool 완전 구현, `pkg/ckv.Engine` in-process API, bge-large-en-v1.5 + ONNX, sliding split + contextual prefix Phase D.1, citation stale check, `ckv reindex` S1.5.

→ **fake → real 전환은 지금 가능**. cks `internal/ckvclient/real.go` 이미 존재.

### 3.2 ckv `internal/eval/prregress/` 가 사용자 명세 80% 이미 구현

`testdata/prs.yaml` 헤더 주석에 명시된 6 단계:
```
1. git worktree checkout at base_sha
2. ckv build over that worktree
3. Claude CLI agent 에 PR Background 만 전달 → plan markdown 생성
4. plan vs PR diff 비교 (LLM-as-judge + File-set F1)
5. Threshold gate (0.80)
```

→ 사용자 명세 multi-stage evaluation 의 *거의 그대로*. 신규 작업이라고 생각했던 게 *E1/E2/E3 메트릭 분해* + *fixture 4 → 12 확장* 으로 좁혀짐.

### 3.3 ckg 가 이미 stable-net 인덱싱 + 4-baseline 측정 인프라 갖춤

`ckg/eval/stablenet/`: 5/11~5/21 작업, 13 파일, EXECUTION_STRATEGY.md (19KB) + HANDOFF.md (28KB) + VERIFICATION_REPORT.md (17KB) + 3 task YAML + MCP probes + eval-v1 결과.

→ ckg 는 cks 통합 관점에서 *추가* 필요한 것만 식별: PR breadcrumb (A 옵션) / Stage B mirror task / 한국어 graceful / public surface (T-14 와 함께).

### 3.4 PR 단위 시각 — 40 commit → 12 fix PR

사용자 지적 "git 전체가 아니라 PR 단위". dev 브랜치 40 PR 중 stable-net 고유 영역 19 PR, fix PR 12 개. **fixture seed 로 자연 정착**.

각 PR 이 *base_sha + 장애 설명 + 수정 이유 + changed_symbols* 자동 제공 → fixture entry 자동 생성 가능.

### 3.5 ADR-003 유보 상태

| ADR | 상태 | 변화 사유 |
|---|---|---|
| ADR-001 (sqlite-vec) | Accepted | 유지 |
| ADR-002 (bge-large pivot) | Accepted | 유지 |
| **ADR-003 (BM25 dual-track)** | **유보** | 사용자 R4 (MCP flow 내 BM25) 와 충돌. 3-leg 임시 적용 후 측정으로 결정 |
| ADR-004 (ckv reindex S1.5) | Accepted | 유지 |
| ADR-005 (CoreML MLProgram) | Accepted | 유지 |
| **ADR-006 (예정)** | **draft 시점 보류** | 3-leg BM25 측정 결과 기반 정식 결정 |

---

## 4. 산출물 — 3 repo cross-link

### 4.1 cks repo

**파일**: `code-knowledge-system/docs/research/knowledge-data-best-practice-2026-05-22.md`
**상태**: Untracked (uncommitted)
**줄 수**: 570
**내용**:
- R-A: Code RAG semantic gap 해결 best practice 카탈로그 (12 기법: Hybrid / Domain glossary / Query expansion / HyDE / MultiQuery / Cross-encoder reranker / Entity linking / GraphRAG / ColBERT / Symbol-aware chunking / Iterative agentic RAG / Embedder fine-tune)
- R-B: go-stablenet 권장 기술 스택 (Tier 1/2/3)
- R-C: ckg ↔ ckv 키워드 공유 기술 (C1-C5)
- R-D: cks 측 신규 기능 (D1-D7, ~1030 LOC)
- D6 corpus 명세 (.claude 파일 리스트 + G1-G7 추가 후보)
- 3-leg BM25 임시 적용 + 측정 후 결정 framework

### 4.2 ckv repo

**파일**: `code-knowledge-vector/docs/evaluation-design-2026-05-22.md` §10 (신설, +493 줄)
**상태**: Modified (uncommitted)
**내용**:
- §10.1 사용자 명세 재정의 (R9~R13)
- §10.2 Multi-stage Evaluation 분해 (E1/E2/E3/E4)
- §10.3 fixture 4 → 12 자동 확장 (pr69/70/72/74 기존 + pr77/75/73/67/63/58/56/55 신규)
- §10.4 ckv 신규 작업 ckv-NEW-1~9 (~1530 LOC 총)
- §10.5 ckg PR-aware A+B+C 통합
- §10.6 Stage A/B/C 평가 체계
- §10.7 의존 그래프
- §10.8 fixture 점진 학습 모드 F1~F4
- §10.9~10.10 D1 + D6 사용자 결정 답변
- §10.11 다음 세션 (prregress 작업자) Day 1~5 권장 순서
- §10.12 cross-link (cks 보고서)

### 4.3 ckg repo

**파일**: `code-knowledge-graph/eval/stablenet/CKS-INTEGRATION-2026-05-23.md`
**상태**: Untracked (uncommitted)
**줄 수**: 478
**내용**:
- §1 사용자 명세 R9~R13 (ckg 측 영향)
- §2 ckg 현재 상태 매트릭스 (cks 통합 관점)
- §3 ckg 신규 작업 ckg-NEW-1~9 (~500 LOC)
- §4 Stage B 평가 명세 (cks 통합 전 ckg 단독)
- §5 T-14 + ckg-NEW-2/3/4/9 동시 진행 권장 (public surface 한 번에 정착)
- §6 fixture mapping (ckv 12 ↔ ckg task 14)
- §7 사용자 결정 항목 (commit timing / CKG-3 / viewer dirty)
- §8 다음 세션 (ckg 작업자) Day 1~5 권장 순서
- §9 cross-link (cks + ckv §10)

### 4.4 메모리 갱신

**파일**: `~/.claude/projects/-Users-wm-it-22-00661-Work-github-tools-code-knowledge-system/memory/project_ckv_status.md`
**변경**: 10 일 전 "MCP stub" 가정 → 현재 87.5% 완성 사실 갱신. fake → real 전환 가능, vocabulary bridge gap 명시, ADR-003 유보 명시.

---

## 5. 작업 분해 — 3 repo 신규 작업 ID 통합

### 5.1 ckv 신규 (ckv-NEW-1~9)

| ID | 작업 | 사용자 명세 | LOC | 의존 |
|---|---|---|---|---|
| ckv-NEW-1 | `ckv query --alias <yaml>` rule-based query expansion | R9 | ~50 | 없음 |
| ckv-NEW-2 | `ckv eval --record` interactive fixture 추가 (F1) | R11 | ~150 | 없음 |
| ckv-NEW-3 | PR corpus indexing (Phase C, backlog #4) | R12 | ~400 | prregress fetcher 재사용 |
| ckv-NEW-4 | `score.go` 확장 (E1/E2/E3 메트릭 분해) | R10 | ~250 | 없음 |
| ckv-NEW-5 | fixture 4 → 12 확장 | R12 | YAML | git/gh fetch |
| ckv-NEW-6 | Symbol-level PR breadcrumb 데이터 (ckg A 짝) | R12 | ~80 | NEW-3 |
| ckv-NEW-7 | `cks.context.related_changes` MCP tool | R12 (B backend) | ~150 | NEW-3, NEW-6 |
| ckv-NEW-8 | Glossary loader (.claude/docs → YAML) | R9 (D backend) | ~150 | 없음 |
| ckv-NEW-9 | 3-leg BM25 (chunk-aware, ckg `pkg/bm25` 재사용) | R13 | ~250 | 없음 |

### 5.2 ckg 신규 (ckg-NEW-1~9)

| ID | 작업 | 사용자 명세 | LOC | 우선 |
|---|---|---|---|---|
| ckg-NEW-1 | 한국어 graceful degradation 검증 | R9 | ~80 | P1 |
| ckg-NEW-2 | `pkg/store.Node.RecentPRs []PRRef` 메타 | R12 (A 옵션) | ~120 | **P0** |
| ckg-NEW-3 | Temporal slicing `RecentPRsBefore(cutoff)` | R12 | ~50 | **P0** |
| ckg-NEW-4 | `pkg/store.Reader.GetNodePRs` public accessor | C2/C4 | ~50 | **P0** |
| ckg-NEW-5 | ckv fixture 12 의 ckg-side mirror task | R10 | YAML | **P0** |
| ckg-NEW-6 | qname canonical 사용 가이드 (docs) | R10 | docs | P1 |
| ckg-NEW-7 | CKG-3 cross-snapshot 재평가 → 옵션 C 권장 | R12 시간축 | 결정 | P1 |
| ckg-NEW-8 | Stage B evaluation harness | R10 (Stage B) | ~150 | **P0** |
| ckg-NEW-9 | `pkg/bm25.Scorer` 외부 import 안정성 | R13 | ~50 | P1 |

### 5.3 cks 신규 (cks-T1-D1~D7)

R-D 에서 정의된 cks 측 신규 모듈. Stage C (통합 평가) 단계에서 작업:

| ID | 모듈 | 위치 | 역할 | LOC |
|---|---|---|---|---|
| D1 | `internal/glossary/` | cks 신규 | .claude/docs → YAML 추출 + loader | ~150 |
| D2 | `internal/vocab/resolver.go` | cks 신규 | 모호 query → canonical keywords | ~200 |
| D3 | `internal/manifest/dual.go` | cks 신규 | ckg + ckv manifest 정합 | ~80 |
| D4 | `internal/fusion/rrf.go` | cks 신규 (또는 stage2 확장) | RRF fusion | ~100 |
| D5 | `internal/bm25/local.go` | cks 임시 | cks 측 BM25 (3-leg) | ~50 |
| D6 | `internal/reranker/` | cks Tier 2 | BGE Reranker M3 | ~300 |
| D7 | `internal/intent/` 확장 | cks 기존 | intent 별 vocab profile | ~150 |

---

## 6. 의존 그래프 + 작업 병렬화

```
[3 갈래로 병렬 진행 가능]

[ckv 세션 — prregress 작업자]
  Day 1: ckv-NEW-5 (fixture 12), ckv-NEW-1 (alias)
  Day 2: ckv-NEW-8 (glossary), ckv-NEW-2 (record)
  Day 3: ckv-NEW-4 (E1/E2/E3 metric), ckv-NEW-3 (PR corpus)
  Day 4-5: ckv-NEW-9 (BM25), Stage A 측정
  → ADR-006 초안 → 사용자 결재

[ckg 세션 — ckg 작업자]
  Day 1: T-14 + ckg-NEW-2/3/4/9 한 PR 정착 (public surface)
  Day 2: ckg-NEW-1 (한국어 graceful), ckg-NEW-5 (11 task YAML)
  Day 3: ckg-NEW-8 (Stage B harness), 14 task × 4 baseline 측정
  Day 4-5: T-04 V4 (Hallucination 30-Q 측정), Stage B 회귀 fix
  → Stage B 결과 → cks 측 전달

[cks 세션 — 통합 작업자]
  ckv Stage A + ckg Stage B 결과 대기
  → Stage C 시작 가능 시점에 진입
  → cks-T1-D1~D7 (~1030 LOC, 1~2주)
  → 통합 평가 (3-leg BM25 ablation, vocab bridge full path, PR-aware)
  → ADR-006 정식 결정 (BM25 영구 위치)
```

**병렬 가능성**: ckv ↔ ckg ↔ cks 세 갈래는 *서로 의존하지 않음*. 단 *fixture 12 개 base_sha* 만 양쪽 정합 (cross-check). cks 만 Stage A/B 결과 대기.

---

## 7. Stage A/B/C 평가 체계 (요약)

| Stage | 측정 대상 | 책임 repo | 진입 조건 | 산출 |
|---|---|---|---|---|
| **Stage A** | ckv 단독 — vocab bridge + retrieval + Multi-stage E1~E3 | ckv | ckv-NEW-1/2/4/5/8/9 + bge-large + 12 fixture | recall@k, MRR, intent score, file F1, symbol F1, plan steps score |
| **Stage B** | ckg 단독 — 영문 keyword → graph 정확도, PR breadcrumb, 한국어 graceful | ckg | T-14 + ckg-NEW-1/2/3/4/5/8 + 14 task | precision/recall, 4-baseline α/β/γ/δ, hallucination, T-04 V4 |
| **Stage C** | cks 통합 — fusion 시너지/회귀, vocab full path, PR context | cks | cks-T1-D1~D5 + Stage A/B 결과 | 3-leg BM25 ablation, vocab bridge full path, PR-aware effect, hallucination delta |

---

## 8. 사용자 결정 대기 항목

### 8.1 즉시 결정 가능

| 항목 | 옵션 | 비고 |
|---|---|---|
| 3 repo 문서 commit 여부 | A. 사용자 검토 후 commit (현재 명시) | uncommitted 유지 |
| ckg viewer dirty 파일 처리 | A. 별도 PR commit / B. drop / C. eval 작업과 분리 | eval 과 무관, 사용자 결정 |
| ckg-NEW-2/3/4 + T-14 commit timing | A. 한 PR 동시 / B. 별도 PR | 한 PR 권장 (public surface) |

### 8.2 측정 후 결정

| 항목 | 측정 의존 |
|---|---|
| ADR-006 (BM25 영구 위치) | Stage A/B/C 3-leg ablation 결과 |
| CKG-3 cross-snapshot 옵션 | 옵션 C 권장, 측정 후 정식 결정 |
| Tier 2 reranker 도입 | Stage C 결과 보고 |

### 8.3 인터뷰 대기 (사용자 직접 손대고 싶다고 명시)

| 항목 | 방식 |
|---|---|
| fixture 한국어 query 설계 | ckv-NEW-2 (record mode) 가동 후 일상 사용 |
| Glossary 수동 검수 | ckv-NEW-8 자동 추출 결과 검수 |
| ADR 의사결정 결재 | Stage A/B 결과 보고 후 |
| go-stablenet 실 장애 재현 공유 | dev 브랜치 PR fixture (12 개) 활용 — 자동 추출됨 |

---

## 9. 미해결 / 누락 위험

### 9.1 다른 세션의 prregress 작업자 진행도 동기화

사용자가 "다른 세션이 prregress 작업 중" 명시. 본 세션이 이 작업과 *충돌하지 않게* 본 문서 §10 형식으로 정착시킴. 단:

- 다른 세션이 이미 작성한 *내부 작업 메모* 가 있다면 본 문서와 정합 필요
- 다른 세션이 ckv-NEW-* 작업 일부를 *다른 ID 로* 진행 중일 수 있음
- ckv-NEW-3 (PR corpus indexing) 은 prregress fetcher.go 와 *코드 공유* 필요 — 충돌 시 prregress 우선

→ **권장**: 다른 세션 작업자가 본 문서 정독 시 *ID 충돌 / 우선순위 충돌* 즉시 보고. 본 문서 작성자 (cks 세션) 가 ID 재배정.

### 9.2 LLM 호출 backend (사용자 환경 의존)

ckv prregress 와 ckg eval-llm-smoke 모두 LLM backend 필요:
- 옵션 A: `ANTHROPIC_API_KEY` (api backend) — 권장. 토큰 분류 정확
- 옵션 B: `CLIWRAP_AGENT` (cli backend, cliwrap-agent 설치) — token classification 한계 (cached_tokens 로 빠짐)

→ 다음 세션 시작 시 *사용자 환경* 확인 필요. Stage A/B 측정 *시작 전* 차단 요소.

### 9.3 bge-large 모델 + libonnxruntime

Stage A 실측 진입 조건:
- libonnxruntime 설치
- libtokenizers.a 빌드 또는 다운로드
- bge-large-en-v1.5 ONNX 모델 다운로드 (`~/.cache/ckv/models/bge-large-en-v1.5/`)
- (macOS) CoreML EP 가용

→ Stage A 첫 측정 시 모델 + 라이브러리 누락 → 즉시 fail. 다른 세션 시작 시 *환경 검증* 단계 필요.

### 9.4 ckv-NEW-3 vs prregress fetcher.go 코드 공유 합의

ckv-NEW-3 (PR corpus indexing) 가 prregress 의 fetcher.go 를 *재사용* 한다고 명세. 그러나:
- fetcher.go 는 *eval* 책임 (PR base_sha checkout + Background 추출)
- ckv-NEW-3 은 *corpus indexing* 책임 (PR description chunk 화)

→ 두 책임이 다르므로 *완전 재사용* 은 어렵고 *helper 추출 + 양쪽 호출* 필요. ckv 작업자가 inflate 결정.

---

## 10. 다음 세션 시작 시 즉시 확인 사항 (체크리스트)

```
□ 본 문서 (session-handoff-2026-05-23.md) §0~§3 정독 (5 분)
□ 본인 작업 세션 구분:
    □ ckv prregress 작업자 → ckv/docs/evaluation-design-2026-05-22.md §10 정독
    □ ckg 작업자 → ckg/eval/stablenet/CKS-INTEGRATION-2026-05-23.md 정독
    □ cks 통합 작업자 → Stage A/B 결과 대기, cks/docs/research/...md 정독
□ 환경 검증:
    □ ANTHROPIC_API_KEY 또는 CLIWRAP_AGENT 가용?
    □ bge-large-en-v1.5 ONNX 다운로드 완료?
    □ libonnxruntime + libtokenizers.a 빌드 완료?
    □ go-stablenet-latest dev 브랜치 checkout 완료?
□ 다른 세션 작업자와 ID 충돌 확인 (ckv-NEW-* / ckg-NEW-* 재배정 필요?)
□ Day 1 작업 시작 — 본 문서 §6 의존 그래프 참조
```

---

## 11. 본 세션이 *명시적으로* 생성하지 않은 산출물 (의도적 제외)

다음 항목은 사용자 명시 또는 적합 시점 부재로 본 세션이 *작성하지 않음*. 다음 세션이 작성 가능:

| 미작성 항목 | 작성 시점 | 작성 위치 권장 |
|---|---|---|
| ADR-006 (BM25 영구 위치) | Stage A/B/C 3-leg 측정 결과 후 | ckv/docs/adr/006-*.md |
| ADR (cks-side) — cks 측 ADR 디렉토리 신설 | cks 측 첫 큰 결정 시 | cks/docs/adr/README.md + 001-* |
| ckv glossary YAML 첫 자동 추출 결과 | ckv-NEW-8 첫 실행 후 | ckv/testdata/stablenet/glossary.yaml |
| fixture 12 개 ckg-side mirror YAML 11 개 | ckg-NEW-5 작업 | ckg/eval/stablenet/tasks/T-pr*.yaml |
| Stage A 첫 measurement 결과 | ckv 작업자 Day 4 | ckv/eval/results/stage-a-2026-MM-DD.json |
| Stage B 첫 measurement 결과 | ckg 작업자 Day 3 | ckg/eval/stablenet/results/stage-b-2026-MM-DD.json |
| best practice 보고서 G3~G7 corpus 도입 결정 | 사용자 추가 결정 후 | 본 핸드오프 문서 update |

---

## 12. cross-link 빠른 참조

### 3 repo 산출물 비교 표

| 사용자 명세 | cks (R-A/B/C/D) | ckv §10 작업 ID | ckg CKS-INTEGRATION 작업 ID |
|---|---|---|---|
| R9 vocab bridge | R-B Tier 1.B/C, R-C C4, R-D D1/D2 | ckv-NEW-1 (alias), NEW-8 (glossary) | ckg-NEW-1 (graceful) |
| R10 multi-stage eval | R-A §2.1 카탈로그, R-D D4 (RRF) | ckv-NEW-4 (E1/E2/E3 metric), NEW-5 (fixture 12) | ckg-NEW-5/8 (Stage B) |
| R11 점진 fixture | R-A §2.1 #11 agentic | ckv-NEW-2 (record mode) | — (Stage C 단계) |
| R12 PR-aware | R-C C4 glossary, R-D D6 reranker | ckv-NEW-3 (PR corpus), NEW-6/7 (breadcrumb) | ckg-NEW-2/3/4 (Node.RecentPRs) |
| R13 3-leg BM25 | R-D D5 (cks-local BM25) | ckv-NEW-9 (chunk-aware) | ckg-NEW-9 (`pkg/bm25` 외부 안정성) |
| D6 corpus | §6.1 핵심 + §6.2 G1-G7 | §10.10 D6 결정 | §6 fixture mapping |

### 사용자 결정 요약

| 결정 | 위치 | 답변 |
|---|---|---|
| D1 (BM25 위치) | ckv §10.9 | 3-leg 임시. ADR-006 측정 후 |
| D6 (corpus 범위) | ckv §10.10, cks §6 | .claude skills 파일 + G3 tests 추가 |
| ckg PR-aware 옵션 | ckg §3.2 | A + B + C 모두 채택 |
| fixture 점진 학습 | ckv §10.8 | F1+F2+F3+F4 점진 도입 |
| 작업 순서 원칙 | ckg §0, ckv §10.6 | Stage A → B → C (작은 단위부터) |
| commit 여부 | 본 문서 §8.1 | 사용자 검토 후 commit |

---

---

## 14. Quick Start for Cross-Machine Session (보완 부록)

> 본 부록은 §0~§13 의 4 가지 gap 을 해소한다. 다른 머신에서 처음 들어오면 본
> §14 부터 시작 가능 (그 후 §0 으로 복귀).

### 14.1 fixture 12 개 base_sha 자동 추출

ckv §10.3 / ckg §6 의 `base_sha` 가 placeholder 형태로 명시되어 있음. 실제
hash 는 go-stablenet-latest dev 브랜치에서 자동 추출:

```bash
REPO=/path/to/go-stablenet-latest   # clone 위치
PRS="55 56 58 63 67 70 72 73 74 75 77"

# 각 PR 의 merge commit 직전 commit = base_sha
for PR in $PRS; do
  MERGE=$(git -C $REPO log dev --pretty=format:"%H" --grep="(#$PR)" | head -1)
  if [ -n "$MERGE" ]; then
    BASE=$(git -C $REPO rev-parse "$MERGE^" 2>/dev/null)
    TITLE=$(git -C $REPO log -1 --pretty=format:"%s" "$MERGE")
    echo "$PR | base_sha=$BASE | $TITLE"
  fi
done
```

산출 예시:
```
77 | base_sha=0bf2f4d1b... | fix: refresh AnzeonTipEnv current block when GasTip changes
75 | base_sha=e5a6e9e14... | fix(wbft): prevent drop of valid next-sequence messages
73 | base_sha=8b895799e... | fix: sync Account.Extra with GovCouncil contract slots
...
```

이 산출 결과를 ckv `testdata/prs.yaml` 의 `base_sha:` 필드에 매핑 + ckg
`eval/stablenet/tasks/T-pr*.yaml` 의 `base_sha:` 필드에 매핑.

### 14.2 동반 정독 필요 문서 (3 repo 산출물 외)

본 핸드오프 §4 의 *신규 산출물 4 개* 외에 다음 *기존 문서* 도 정독 필요:

| repo | 경로 | 정독 이유 |
|---|---|---|
| ckv | `internal/eval/prregress/{runner,agent,score,types,fetcher,checkout,ground}.go` (1710 LOC) | 사용자 명세 multi-stage eval 의 80% 이미 구현. ckv-NEW-3/4 진행 시 필수 |
| ckv | `docs/embedder-integration.md` (~300 줄) | bge-large + libonnxruntime + CoreML EP 셋업 |
| ckv | `docs/backlog.md` | 전체 추적 항목 (45 entries) SoT |
| ckv | `docs/pending-work-2026-05-21.md` | 5/21 시점 잔여 작업 + CKG eval gap |
| ckv | `docs/adr/00{1,2,3,4,5}-*.md` | 5 개 ADR (003 = BM25 dual-track, 본 세션이 *유보* 결정) |
| ckg | `eval/stablenet/{HANDOFF,EXECUTION_STRATEGY,VERIFICATION_REPORT,RESPONSE_VALIDATION_REPORT}.md` | T-01~T-15 명세. **T-14 가 cks import 차단 요소** — ckg-NEW-2/3/4/9 와 함께 정착 |
| ckg | `docs/EVAL.md` | 4-baseline α/β/γ/δ + LLM backend 가이드 |
| ckg | `docs/analysis/CKS-SPEC-COMPLIANCE.md` | 5/5 시점 cks spec 25% 가정 — 본 세션이 사용자 명세 R9-R13 으로 진화시킴 |
| ckg | `docs/stale-git-lock-analysis-2026-05-21.md` | gitstatusd race 알려진 패턴 — cks repo 에서도 발생 가능 (본 세션 실측) |
| go-stablenet | `.claude/docs/{BUILD_SOURCE_FILES,CLAUDE_DEV_GUIDE,SYSTEM_CONTRACT_FLOW,REVIEW_GUIDE}.md` | 도메인 어휘 source + 빌드 참여 781 파일 명세 |

### 14.3 환경 셋업 가이드 포인터

**ckv 작업자**:
- `ANTHROPIC_API_KEY`: Anthropic Console 발급. `~/.bashrc` 또는 `~/.zshrc` 에 export
- bge-large-en-v1.5 ONNX: HuggingFace `BAAI/bge-large-en-v1.5` 또는 ckv `docs/embedder-integration.md` §2-3 권장 다운로드 절차
- `libonnxruntime`: macOS `brew install onnxruntime` / linux `apt install libonnxruntime-dev` (ckv embedder-integration.md §2/§3)
- `libtokenizers.a`: ckv Makefile `make tokenizers` 또는 vendor 다운로드
- 모델 캐시 위치: `~/.cache/ckv/models/bge-large-en-v1.5/`

**ckg 작업자**:
- 동일 환경 + claude CLI (`@anthropic-ai/claude-code`) 또는 cliwrap-agent
- `cliwrap-agent`: `go install github.com/0xmhha/cli-wrapper/cmd/cliwrap-agent@latest`
- `CLIWRAP_AGENT=$(which cliwrap-agent)` export
- go-stablenet-latest dev 브랜치 clone (`base_sha` 추출 + git worktree 용)

**cks 통합 작업자**:
- Stage A (ckv) + Stage B (ckg) 결과 대기. 즉시 셋업 불필요
- 진입 시점: 양쪽 Stage 결과 JSON 받은 후

### 14.4 본 세션 task 상태 (final snapshot)

세션 내부 TaskTracker 14 개. *모두 completed*. 산출물 형태로 영구화되었으므로
cold start 시 task 상태 복원 *불필요*. 참조용 매핑만:

| Task | 산출물 위치 |
|---|---|
| 1. ckv 메모리 갱신 | `~/.claude/projects/.../memory/project_ckv_status.md` (10일 stale → 87.5% 갱신) |
| 2. ADR 정독 | (internal — ADR-003 유보 결정 → cks best-practice §0) |
| 3. R-A best practice 카탈로그 | `cks/docs/research/knowledge-data-best-practice-2026-05-22.md` §2 |
| 4. R-B go-stablenet 권장 스택 | 동 §3 |
| 5. R-C 키워드 공유 기술 | 동 §4 |
| 6. R-D cks 신규 기능 식별 | 동 §5 |
| 7. D6 corpus 명세 | 동 §6 |
| 8. cks 종합 보고서 | 동 (570 줄 전체) |
| 9. ckv §10 작성 | `ckv/docs/evaluation-design-2026-05-22.md` §10 (origin/main 검증 완료) |
| 10. ckg 상태 정독 | (internal — ckg `eval/stablenet/HANDOFF.md` 14 task 매핑) |
| 11. ckg 신규 작업 식별 | `ckg/eval/stablenet/CKS-INTEGRATION-2026-05-23.md` §3 |
| 12. ckg 평가 문서 작성 | 동 (478 줄 전체) |
| 13. 마스터 핸드오프 | 본 문서 §0~§13 |
| 14. Quick Start 보완 | 본 §14 (이 commit) |

### 14.5 ckv §10 living document 운영 (본 세션 검증)

본 세션이 작성한 ckv §10 은 *살아있는 문서* 로 작동 중. 다른 세션 작업자가
*작업 완료 시 §10.4 표에 ✅ + commit hash 마킹* 패턴:

```
ckv-NEW-1 (alias)    → ✅ commit ba5ba96
ckv-NEW-5 (fixture)  → ✅ commit c005e04
ckv-NEW-8 (glossary) → ✅ commit 3f4483c
ckv-NEW-2/3/4/6/7/9  → ⏳ 미진행
```

→ **ckv §10 이 작업 추적의 single source of truth**. ckg `CKS-INTEGRATION-
2026-05-23.md` §3 도 같은 패턴 적용 권장 (ckg-NEW-1~9 마킹).

### 14.6 ID 충돌 해소 액션 (§9.1 보완)

본 세션이 명세한 `ckv-NEW-1~9` / `ckg-NEW-1~9` / `cks-T1-D1~D7` ID 와 다른
세션이 이미 진행 중인 작업 사이 충돌 가능. 충돌 발견 시 *즉시 액션*:

1. 본 핸드오프 §5 의 작업 표를 *살아있는 문서* 로 직접 편집 — 새 commit
   message 에 "[ID conflict] resolved: <old-id> → <new-id>" 명시
2. ckv §10.4 / ckg §3 표도 같은 commit 에 동시 update
3. cross-link 문서 (cks best-practice) 도 일관 update
4. push 후 다른 세션 작업자가 fetch 시 자동 정합

→ ID 는 *권위 있는 식별자* 가 아니라 *작업 분류 라벨*. 의미 충돌이 아닌 한
유연 재배정 가능.

### 14.7 다른 머신 첫 5 분 워크플로우

```bash
# Step 1: clone (1 분)
git clone https://github.com/0xmhha/code-knowledge-system.git
git clone https://github.com/0xmhha/code-knowledge-vector.git
git clone https://github.com/0xmhha/code-knowledge-graph.git
git clone <go-stablenet-latest URL>  # base_sha 추출 + worktree 용

# Step 2: 진입점 문서 (3 분)
$EDITOR code-knowledge-system/docs/research/session-handoff-2026-05-23.md
# § 0 TL;DR → § 14.7 (현재 위치) → § 4 산출물 → § 6 의존 그래프 → § 10 체크리스트

# Step 3: 본인 작업 시나리오 선택 (§ 6)
#   A. ckv prregress 작업자
#   B. ckg 작업자
#   C. cks 통합 작업자 (Stage A/B 결과 대기)

# Step 4: 시나리오별 자기 문서 정독 (1 분)
#   A → code-knowledge-vector/docs/evaluation-design-2026-05-22.md § 10
#   B → code-knowledge-graph/eval/stablenet/CKS-INTEGRATION-2026-05-23.md
#   C → cks 본 문서 + best-practice

# Step 5: 환경 셋업 + Day 1 작업 시작 (§ 14.3)
```

---

## 13. 변경 이력

| 일자 | 변경 |
|---|---|
| 2026-05-23 | 초안. 본 세션 (cks 통합 점검, 2026-05-22~23) 전체 핸드오프. 3 repo 산출물 cross-link + 작업 분해 ckv/ckg/cks-NEW-* + 의존 그래프 + 사용자 결정 + 환경 체크리스트. |
| 2026-05-23 (보완) | §14 Quick Start 부록 추가. 4 가지 gap 해소: (14.1) fixture base_sha 자동 추출 명령, (14.2) 동반 정독 필요 문서 (3 repo 기존 문서 + go-stablenet `.claude`), (14.3) 환경 셋업 가이드 포인터, (14.4) task 14 개 final snapshot, (14.5) ckv §10 living document 검증, (14.6) ID 충돌 해소 액션, (14.7) 다른 머신 첫 5 분 워크플로우. |
