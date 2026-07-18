# go-stablenet 코드-지식 데이터셋 구축 메뉴얼

> **목적**: `go-stablenet` 소스 코드와 도메인 지식을 기반으로
> **cks / ckv / ckg** 코드-지식 데이터셋을 **반복 가능하게** 구축한다.
> 결과물은 코딩 에이전트가 MCP로 소비하는 *EvidencePack* 의 검색 기반(grounding)이 된다.
>
> **검증 환경 (이 메뉴얼이 실제로 실행·검증된 기준)**
> - macOS (Apple Silicon, darwin/arm64), Go 1.26.4
> - go-stablenet: Go 파일 1,256개 → **ckg 노드 220,507 / 엣지 1,928,983** (graph.db ≈ 725 MB)
> - 도메인 엔트리 43개(40 verified) → **ckv 카테고리 14 / ckg 정책 40 / governed_by 엣지 112**
> - 임베딩 모델: **bge-m3** (1024-dim, 다국어 한/영) via Ollama

---

## 0. 시스템 개요 — cks / ckv / ckg 의 관계

```
                         coding-agent  (MCP client)
                              │  MCP (stdio)
                              ▼
                      ┌───────────────┐
                      │   cks-mcp     │  오케스트레이터
                      │ Intent→Fan-out│  - 토큰 예산 / RRF 융합
                      │  →RRF→Budget  │  - 정화(sanitize) 경계
                      │  →Sanitize    │  - LLM 호출 없음
                      └───┬───────┬───┘
            in-process ▼           ▼ in-process
                ┌──────────┐   ┌──────────┐
                │   ckv    │   │   ckg    │
                │ 벡터/의미 │   │ 그래프/키워드│
                │ bge-m3   │   │ BM25+graph│
                │ sqlite-vec│   │ SQLite/PG │
                └────┬─────┘   └────┬─────┘
                     └──── go-stablenet 소스 ────┘
```

| 시스템 | 역할 | 데이터셋 산출물 | LLM/모델 |
|--------|------|----------------|----------|
| **ckg** (code-knowledge-graph) | 키워드·그래프 검색. 심볼/호출/동시성/거버넌스 엣지 | `data/ckg-stablenet/graph.db` | 빌드: **LLM 불필요**. 평가(eval)만 claude-sonnet-4-6 |
| **ckv** (code-knowledge-vector) | 의미 기반 벡터 검색 | `ckv-stablenet/vector.db` + `manifest.json` | 빌드: **bge-m3 임베딩** (Ollama) |
| **cks** (code-knowledge-system) | ckv+ckg 합성 → EvidencePack, MCP 노출 | `cks-stablenet.yaml` (설정) | **내부 LLM 호출 없음** |

**채널 ②(도메인 지식)**: `docs/domain-knowledge/projects/go-stablenet/entries/*.yaml` 43개를
→ `cks-domain-export`로 마크다운 코퍼스로 변환 → ckv가 코드와 함께 임베딩,
→ `cks-domain-sync`로 ckv/ckg 정책(governance) 뷰 생성.

---

## 1. 언어모델 설정 (핵심)

데이터셋 구축에 관여하는 모델은 **2종**이다. cks 자체는 LLM을 호출하지 않는다.

### 1-A. ckv 임베딩 모델 — `bge-m3` (필수)

| 항목 | 값 |
|------|----|
| 모델명 | `bge-m3` |
| 차원 | **1024** (cks 부팅 시 manifest dim과 대조 검증) |
| 백엔드 | Ollama (`--embedder=ollama`) |
| 엔드포인트 | `http://localhost:11434` (`CKV_OLLAMA_ENDPOINT`로 override) |
| 특징 | 다국어(한국어+영어), 8192 토큰 입력 |

> ⚠️ **반드시 빌드 모델과 질의 모델이 일치해야 한다.** `cks-stablenet.yaml`의
> `backends.ckv.embed_model`은 ckv 인덱스를 만든 모델과 동일해야 하며, 다르면 cks가 부팅을 거부한다.

**대안(오프라인/Ollama 불가 시)**: `--embedder=bgeonnx` (ONNX Runtime + `~/.cache/ckv/models/`에
ONNX 모델 다운로드, `-tags bgeonnx` 빌드 필요). 본 메뉴얼은 Ollama 경로를 기준으로 한다.

### 1-B. ckg 평가 모델 — `claude-sonnet-4-6` (선택, 빌드에는 불필요)

ckg **그래프 빌드 자체는 LLM이 필요 없다.** 검색 품질을 정량 평가(`ckg eval`)할 때만 필요:

```bash
export ANTHROPIC_API_KEY=sk-...        # api 백엔드
# Makefile 기본값: LLM_BACKEND=api, LLM_MODEL=claude-sonnet-4-6
```

### 1-C. Ollama 설치 — Apple Silicon 주의점 (실측 함정)

> 🛑 **`brew install ollama` (formula) 를 쓰지 말 것.** Apple Silicon에서 formula는
> Go 오케스트레이터만 설치하고 모델을 실제 실행하는 `llama-server` 런너를 포함하지 않아
> 임베딩 요청이 HTTP 500(`llama-server binary not found`)으로 실패한다.

올바른 설치 — **앱 cask** (런타임 번들 포함):

```bash
brew install --cask ollama-app          # /Applications/Ollama.app + 런너 포함
# 데몬 기동
/Applications/Ollama.app/Contents/Resources/ollama serve &   # 또는 앱 실행
ollama pull bge-m3                       # 1.2 GB
# 검증 (1024 기대)
curl -s http://localhost:11434/api/embed -d '{"model":"bge-m3","input":"hello"}' \
  | python3 -c "import sys,json;print('dim=',len(json.load(sys.stdin)['embeddings'][0]))"
```

---

## 2. 사전 준비물

| 도구 | 버전/비고 |
|------|----------|
| Go | 1.23+ (검증: 1.26.4) |
| Ollama | **app cask** + `bge-m3` pull |
| 리포지토리 | `code-knowledge-system`, `code-knowledge-vector`, `code-knowledge-graph`, `go-stablenet` 이 같은 부모 디렉터리에 체크아웃 |
| 디스크 | graph.db ≈ 0.7 GB, vector.db 수 GB, 모델 1.2 GB |
| 시간 | ckg ≈ 1~2분, **ckv 임베딩 ≈ 수 시간(~10h, ~26k 청크)** |

환경 변수(권장):
```bash
export GO_STABLENET_ROOT="$(cd ../go-stablenet && pwd)"   # 형제 체크아웃 (또는 본인 경로로 지정)
export CKV_OLLAMA_ENDPOINT=http://localhost:11434
```

---

## 3. 데이터셋 구축 — 단계별 절차

> **모든 명령은 `code-knowledge-system` 리포 루트에서 실행한다.**
> 전체를 한 번에 돌리려면 [§5 원-샷 스크립트](#5-원-샷-스크립트) 사용.

### Step 0 — 바이너리 빌드
```bash
cd code-knowledge-graph  && make build-no-viewer        # → bin/ckg
cd ../code-knowledge-vector && make build               # → bin/ckv (ollama 지원)
cd ../code-knowledge-system && make build-bins          # → bin/cks-mcp 등
go build -o bin/cks-domain-export ./cmd/cks-domain-export
go build -o bin/cks-domain-sync   ./cmd/cks-domain-sync
```

### Step 1 — 도메인 코퍼스 export (채널 ②)
```bash
mkdir -p generated/domain-corpus
./bin/cks-domain-export \
  -project docs/domain-knowledge/projects/go-stablenet \
  -out     generated/domain-corpus/go-stablenet
# 기대: "43 entries, 0 docs -> generated/domain-corpus/go-stablenet"
```

### Step 2 — 정책 동기화 (ckv + ckg 뷰 생성)
```bash
mkdir -p generated/policies
./bin/cks-domain-sync \
  -entries docs/domain-knowledge/projects/go-stablenet \
  -ckv-out generated/policies/stablenet-ckv.yaml \
  -ckg-out generated/policies/stablenet-ckg-policy.yaml
# 기대: "40 verified entries → 14 ckv categories, 40 ckg policies"
```

### Step 3 — ckg 그래프 인덱스 빌드 (LLM 불필요)
```bash
rm -rf data/ckg-stablenet && mkdir -p data/ckg-stablenet
../code-knowledge-graph/bin/ckg build \
  --src "$GO_STABLENET_ROOT" \
  --out data/ckg-stablenet \
  --lang go \
  --policy-file generated/policies/stablenet-ckg-policy.yaml
# 기대: "built 220507 nodes / 1928983 edges", policy_nodes=40 governed_by_edges=112
../code-knowledge-graph/bin/ckg validate --graph data/ckg-stablenet --format json | tail -5
```
> 일부 `policy governs[] target not found` 경고는 정상(엔트리가 가리키는 일부 심볼이
> 현재 코드에 없을 때). 빌드는 성공하며 매칭된 112개 엣지가 생성된다.

### Step 4 — ckv 벡터 인덱스 빌드 (bge-m3, **장시간**)
```bash
# 전제: ollama serve 가동 + `ollama pull bge-m3` 완료
curl -fsS http://localhost:11434/api/version    # 데몬 확인
rm -rf ckv-stablenet && mkdir -p ckv-stablenet
CKV_OLLAMA_ENDPOINT=http://localhost:11434 \
../code-knowledge-vector/bin/ckv build \
  --embedder=ollama --model-name=bge-m3 \
  --src "$GO_STABLENET_ROOT" --out ckv-stablenet \
  --policy generated/policies/stablenet-ckv.yaml \
  --docs   generated/domain-corpus/go-stablenet
# ~26k 청크 임베딩, 수 시간 소요. nohup/백그라운드 권장:
#   nohup ... > logs/ckv-build.log 2>&1 &
```
산출물: `ckv-stablenet/vector.db` + `ckv-stablenet/manifest.json`
(`embedding_model: bge-m3`, `embedding_dimension: 1024` 확인).

### Step 5 — cks 설정·env **생성** (수작업 금지)
절대경로를 손으로 박지 않는다. 현재 체크아웃 위치를 런타임에 탐지해 config와 env를 **생성**한다:
```bash
./scripts/gen-cks-config.sh
# 생성물:
#   cks-stablenet.yaml  — 절대경로로 해석된 cks 설정 (ckg+ckv+domain)
#   cks.env             — coding-agent plugin/.mcp.json 이 읽는 export 4종
```
리포를 옮기거나 경로가 바뀌면 **이 스크립트만 다시 실행**하면 된다(파일 수정 불필요).
`build-stablenet-dataset.sh`는 마지막 단계에서 이 생성을 자동 수행한다.

---

## 4. 검증 (smoke test)

```bash
export GO_STABLENET_ROOT="$(cd ../go-stablenet && pwd)"   # 형제 체크아웃 (또는 본인 경로로 지정)

# 1) cks 헬스체크 — cks.ops.health 를 MCP stdio 로 호출 (status "ok" 기대, 종료코드 0)
./scripts/cks-health.sh ./cks-stablenet.yaml
#    기대 출력: {"status":"ok","backends":{"ckg":{"reachable":true,...},"ckv":{"reachable":true,...}}}
#    ckg.indexed_head == ckv.stats_hash 이면 두 인덱스가 동일 커밋 기준으로 정합

# 2) ckg 단독 검색 정상성
../code-knowledge-graph/bin/ckg eval-retrieval --graph data/ckg-stablenet \
  --fixtures eval/retrieval 2>/dev/null | tail -10

# 3) ckv 단독 의미 검색 (Ollama 가동 중)
../code-knowledge-vector/bin/ckv query "WBFT quorum size calculation" \
  --out ckv-stablenet --embedder=ollama --model-name=bge-m3 --top 5

```

---

## 4-B. coding-agent(Claude Code 플러그인)에 등록

coding-agent은 Claude Code **플러그인**이며 cks는 이미 그 `plugin/.mcp.json`에
`${CKS_MCP_BIN}` / `${CKS_CONFIG}` 슬롯으로 들어가 있다. 따라서 등록은 **플러그인 설치 +
env 한 줄 source** 가 전부다 — `.mcp.json` 을 손으로 고칠 필요 없다.

**1) 플러그인 설치 (마켓플레이스)** — Claude Code 안에서:
```
/plugin marketplace add 0xmhha/coding-agent
/plugin install coding-agent
```
> 주의: 마켓플레이스 add는 repo 루트에 `.claude-plugin/marketplace.json` 이 필요하다.
> 현재 coding-agent repo에는 `plugin/.claude-plugin/plugin.json` 만 있어, 없으면
> 로컬 경로 설치(`/plugin install <local-path>`)로 대체한다.

**2) cks env 활성화 (on-demand — `~/.zshrc` 수정 없음)** — 플러그인을 쓸 셸에서만 주입:
```bash
source ./activate.sh    # cks.env(+jira.env) 를 현재 셸에 로드
claude                  # 같은 셸에서 Claude Code 실행 → .mcp.json 의 ${CKS_MCP_BIN}/${CKS_CONFIG} 상속
```
> 새 머신 전체 셋업은 [SETUP.md](./SETUP.md) 의 `scripts/setup-all.sh` (one-click) 참고.

**3) 재시작 후 확인** — 새 셸에서 Claude Code 기동 → MCP 패널에 `cks` connected,
또는 `./scripts/cks-health.sh` 가 `status: ok`. 계약 드리프트는
`coding-agent/contract/lint-tool-names.sh` 로 점검(cks 19개 도구 등재).

검증 체크리스트:
- [ ] `data/ckg-stablenet/graph.db` 존재, `ckg validate` OK
- [ ] `ckv-stablenet/manifest.json` 의 model=bge-m3, dim=1024
- [ ] `ckv query` 가 관련 심볼을 반환
- [ ] `cks.ops.health` 가 ckg/ckv **ok** (degraded 아님)

---

## 5. 원-샷 스크립트

반복 실행은 `scripts/build-stablenet-dataset.sh` 하나로 수행한다.

```bash
# 전체 빌드 (ckv 포함 — 수 시간)
GO_STABLENET_ROOT=/path/to/go-stablenet ./scripts/build-stablenet-dataset.sh

# ckv(장시간) 제외하고 빠르게 ckg+정책+코퍼스만
SKIP_CKV=1 ./scripts/build-stablenet-dataset.sh

# 특정 단계만: bins | domain | policy | ckg | ckv
STAGE=ckg ./scripts/build-stablenet-dataset.sh
```
스크립트는 멱등(idempotent)이며 각 단계가 자기 산출물을 새로 만든다.

---

## 6. 반복/증분 갱신 절차

go-stablenet 소스가 바뀌었을 때:

| 변경 규모 | 절차 |
|-----------|------|
| **소스 일부 변경** | `ckv reindex`(변경 파일만 증분 임베딩) + `ckg build`(증분 캐시 자동) 재실행. `STAGE=ckg` 로 그래프만 재빌드 가능 |
| **도메인 엔트리 추가/수정** | `entries/*.yaml` 편집 → `cks-entry-verify`로 검증 → `STAGE=domain` + `STAGE=policy` 재실행 → ckv 재빌드(코퍼스 반영), ckg 재빌드(정책 반영) |
| **모델 교체** | 새 모델로 `ollama pull` → `EMBED_MODEL=...` 로 ckv 전체 재빌드 → `cks-stablenet.yaml`의 `embed_model` 동일 변경 |
| **전체 재구축** | `./scripts/build-stablenet-dataset.sh` 전체 실행 |

증분 ckv 예시:
```bash
../code-knowledge-vector/bin/ckv reindex --out ckv-stablenet \
  --embedder=ollama --model-name=bge-m3 --src "$GO_STABLENET_ROOT"
```

cks에는 운영 도구 `cks.ops.index`가 있어 에이전트가 직접 `ckg build` / `ckv reindex`를
재실행하고 정책·코퍼스를 자동 재내보내기 할 수 있다(설정의 binary_path/policy_file/domain 사용).

---

## 7. 트러블슈팅

| 증상 | 원인 / 해결 |
|------|-------------|
| ckv 빌드 시 임베딩 500 `llama-server binary not found` | **brew formula ollama 설치됨.** `brew uninstall ollama && brew install --cask ollama-app` 후 데몬 재기동 (§1-C) |
| cks 부팅이 degraded | Ollama 미가동 또는 `embed_model` 불일치. `ollama serve` 확인, manifest dim=1024 확인 |
| `embedder dim=N, want 1024` | 인덱스를 bge-m3가 아닌 모델로 빌드함. ckv 재빌드 |
| `policy governs[] target not found` 경고 | 정상(엔트리가 현재 코드에 없는 심볼 참조). 빌드는 성공 |
| `code_root unset; skipping authoritative_docs copy` | 정상(코드 루트의 권위 문서 복사를 건너뜀). 엔트리 43개는 정상 export |
| ckg binary not found | `data/`, `ckv-stablenet/`은 gitignore 권장. 절대경로로 `binary_path` 지정 |

---

## 8. 산출물 레이아웃 (참조)

```
code-knowledge-system/
├── cks-stablenet.yaml                         # ★생성★ cks 설정 (gen-cks-config.sh 산출)
├── cks.env                                     # ★생성★ coding-agent용 env exports
├── scripts/gen-cks-config.sh                  # config+env 생성기 (경로 런타임 탐지)
├── scripts/build-stablenet-dataset.sh         # 반복 빌드 드라이버 (마지막에 gen 호출)
├── scripts/cks-health.sh                      # cks.ops.health 헬스체크
├── docs/go-stablenet-dataset-build-manual.md  # 본 메뉴얼
├── docs/domain-knowledge/projects/go-stablenet/
│   ├── project.yaml · subsystems.yaml · glossary.yaml
│   └── entries/*.yaml                          # 43 도메인 엔트리 (소스, 40 verified)
├── generated/                                  # (gitignored) 재생성물
│   ├── domain-corpus/go-stablenet/entries/*.md # 채널② 코퍼스
│   └── policies/
│       ├── stablenet-ckv.yaml                  # ckv 정책 뷰 (14 카테고리)
│       └── stablenet-ckg-policy.yaml           # ckg 거버넌스 정책 (40)
├── data/ckg-stablenet/graph.db                 # ckg 그래프 데이터셋 (~725MB)
└── ckv-stablenet/{vector.db,manifest.json}     # ckv 벡터 데이터셋
```

> `.gitignore` 권장 추가: `/data/`, `/ckv-stablenet/`, `/cks-stablenet.yaml`, `/cks.env`
> (모두 생성물 — `/generated/`, `/logs/` 는 이미 무시됨).
