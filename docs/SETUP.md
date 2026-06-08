# SETUP — go-stablenet 코드-지식 스택 (새 머신 one-click)

이 문서는 **빈 머신에서** cks/ckv/ckg + go-stablenet 데이터셋 + coding-agent 플러그인을
재현 가능하게 셋업하는 단일 가이드다. 모든 단계는 스크립트화되어 있고 멱등(idempotent)이다.

> 전역 셸 설정을 건드리지 않는다. 환경변수는 **필요할 때 `source activate.sh`** 로 현재 셸에만
> 주입한다(`~/.zshrc` 수정 없음).

관련 문서: 데이터셋 내부 동작·증분 갱신은 [go-stablenet-dataset-build-manual.md](./go-stablenet-dataset-build-manual.md).

---

## 0. 전제 — 리포 레이아웃 (형제 디렉터리)

여섯 리포를 **하나의 부모 아래 형제**로 클론한다 (스크립트가 상대경로로 서로를 찾는다):

```
<parent>/                         예: ~/Work/github
├── code-knowledge-system    (cks · 오케스트레이터 + 이 스크립트들)
├── code-knowledge-vector    (ckv · 벡터/bge-m3)
├── code-knowledge-graph     (ckg · 그래프/키워드)
├── go-stablenet             (대상 소스)
├── coding-agent             (Claude Code 플러그인 + jira-gateway)
└── chainbench               (선택 · 평가 러너)
```

## 1. 사전 도구

| 도구 | 용도 | 비고 |
|------|------|------|
| Go ≥ 1.23 | 모든 바이너리 빌드 | `go version` |
| C 툴체인 (cc/clang) | cks 의 sqlite-vec (CGO) | Xcode CLT |
| Ollama (**app cask**) | bge-m3 임베딩 | ⚠️ brew **formula 금지** (§7) |
| Node ≥ 18 (선택) | chainbench MCP | |
| gh, python3, git | PR/검증/빌드 | |

---

## 2. One-click 설치

cks 리포 루트에서:
```bash
cd code-knowledge-system
./scripts/setup-all.sh                 # 전체 (ckv 임베딩 포함 — 수십 분~수 시간)
# 빠른 경로(긴 ckv 임베딩 제외):
SKIP_CKV=1 ./scripts/setup-all.sh
# Ollama+bge-m3 이미 있음:
SKIP_OLLAMA=1 ./scripts/setup-all.sh
```

`setup-all.sh` 가 수행하는 일 (각 단계는 개별 스크립트로도 실행 가능):

| 단계 | 내용 | 개별 실행 |
|------|------|-----------|
| 0 | 전제 도구·형제 리포 확인 | — |
| 1 | Ollama **cask** 설치 → `ollama serve` → `bge-m3` pull → 1024-dim 검증 | — |
| 2 | 바이너리 빌드: ckg/ckv/cks + domain tools | `STAGE=bins build-stablenet-dataset.sh` |
| 3 | 도메인 코퍼스(36) + 정책(ckv 14/ckg 36) | `STAGE=domain`, `STAGE=policy` |
| 4 | ckg 그래프 + ckv 벡터(bge-m3) 빌드 | `STAGE=ckg`, `STAGE=ckv` |
| 5 | config/env 생성: `cks-stablenet.yaml` + `cks.env` | `gen-cks-config.sh` |
| 6 | jira-gateway 빌드 | `go build ./cmd/server` |
| 7 | `~/.config/coding-agent/jira.env` 스캐폴드(600) | — |

---

## 3. 활성화 — 핵심

### 권장(easy): Claude Code `settings.json` 에 env 주입 (실행 방식 무관)
Claude Code는 플러그인 `.mcp.json` 의 `${VAR}` 를 **`~/.claude/settings.json` 의 `env` 블록**에서
해석한다. 여기에 한 번 써두면 **GUI/Dock/터미널 어떤 방식으로 띄워도** MCP가 연결된다
(매번 `source` 불필요, `~/.zshrc` 변경 없음).
```bash
./scripts/apply-cc-settings.sh    # activate.sh로 경로 해석 → settings.json 의 env 에 병합(기존 보존)
# 그 후 Claude Code 를 한 번 재시작하면 끝.
```
> `setup-all.sh` 는 이 단계를 자동 수행한다. 경로가 바뀌거나 jira 토큰을 채운 뒤에는
> `apply-cc-settings.sh` 만 다시 실행한다.

### 대안: 터미널 on-demand (settings.json 안 쓰고 싶을 때)
```bash
source ./activate.sh        # cks.env(+jira.env) 를 이 셸에만 로드
claude                      # 반드시 같은 셸에서 실행 (GUI 실행은 미지원)
```

검증:
```bash
./scripts/cks-health.sh     # → status: ok (ckg+ckv reachable)
```

---

## 4. 플러그인 설치 (마켓플레이스)

활성화된 셸에서 띄운 Claude Code 안에서:
```
/plugin marketplace add 0xmhha/coding-agent
/plugin install coding-agent@coding-agent
/reload-plugins
```
설치 확인: `/plugin`(installed), `/mcp`(cks/jira-gateway connected), `/doctor`(로드 에러 상세).

> cks 가 connected 가 아니면 → Claude Code 를 `source ./activate.sh` 한 셸에서 다시 띄웠는지,
> `ollama serve` 가 떠 있는지 확인(§7).

---

## 4-B. 자율 실행 (권한 미요청) — 런처로 기동

Claude Code 권한 모드는 **세션 시작 시점에 한 번 로드**되고 **실행 디렉터리(프로젝트)** 기준이다.
플러그인은 권한을 격상할 수 없으므로(`permissionMode` frontmatter 무시), 자율 실행은
**대상 프로젝트(go-stablenet)에 `bypassPermissions` 설정을 두고 그 안에서 Claude Code를 띄우는**
방식으로만 가능하다.

이를 자동화한 런처를 사용한다(설정 생성 → go-stablenet에서 기동):
```bash
./scripts/coding-agent.sh                          # 자율 모드로 Claude Code 기동
./scripts/coding-agent.sh /coding-agent:work STABLE-1234
```
- `enable-autopilot.sh` 가 `go-stablenet/.claude/settings.local.json` 에 `bypassPermissions` 를
  **idempotent 생성**(이미 있으면 pass)하고 `.gitignore` 처리한다. `setup-all.sh` 8단계에서 자동 수행.
- 세션은 orchestrator·planner·implementer·evaluator(서브에이전트 포함) 전체가 세션 권한 모드를
  **상속**하므로 파일 읽기/수정·Bash·MCP 호출을 묻지 않고 자율 진행한다.
- ⚠️ 이미 떠 있는 세션에는 적용 불가(다음 기동부터). hook으로 "세션 도중" 격상도 불가(보안).

---

## 5. Jira 연동 (선택) — 공식 MCP 불필요

`jira-gateway` 는 공식 Atlassian MCP의 래퍼가 아니라, **Jira Cloud REST API v3 에 직접 연결**하는
보안 프록시다(LLM 입력 전 민감정보 redaction). 인증은 **이메일 + API 토큰(HTTP Basic)** 뿐 —
OAuth 앱·공식 MCP 없이 연결된다.

```bash
# 시크릿 파일(리포 밖, 600). 토큰: id.atlassian.com/manage-profile/security/api-tokens
$EDITOR ~/.config/coding-agent/jira.env
#   JIRA_BASE_URL   = https://<your>.atlassian.net
#   JIRA_USER_EMAIL = you@your-domain.com
#   JIRA_API_TOKEN  = <token>
./scripts/apply-cc-settings.sh   # settings.json 의 env 갱신 → Claude Code 재시작하면 반영
```

## 6. chainbench (선택)
```bash
( cd ../chainbench && ./install.sh )    # ~/.chainbench 로 클론 + MCP 빌드
```
`install.sh` 의 심볼릭 단계는 `/usr/local/bin`(쓰기 시 sudo 필요)을 시도한다. sudo 없이 쓰려면
PATH 에 이미 있는 `~/.local/bin` 으로 직접 링크:
```bash
ln -sf ~/.chainbench/chainbench.sh      ~/.local/bin/chainbench
ln -sf ~/.chainbench/bin/chainbench-mcp ~/.local/bin/chainbench-mcp
```
`activate.sh` 가 `CHAINBENCH_DIR=$HOME/.chainbench` 를 export 하고 `chainbench-mcp` PATH 해석을 점검한다.
.mcp.json 의 `chainbench` 엔트리는 변경 불필요(원래부터 `chainbench-mcp` + `${CHAINBENCH_DIR}`).

---

## 7. 트러블슈팅

| 증상 | 원인 / 해결 |
|------|-------------|
| ckv 임베딩 500 `llama-server binary not found` | brew **formula** ollama. `brew uninstall ollama && brew install --cask ollama-app` 후 `ollama serve` 재기동 |
| `/mcp` 에서 cks 가 failed/empty | Claude Code 를 `source ./activate.sh` 안 한 셸에서 띄움 → `${CKS_MCP_BIN}` 빈 값. 활성화된 셸에서 재시작 |
| cks health = degraded | Ollama 미가동 또는 모델 불일치. `ollama serve` + manifest dim=1024 |
| `1 error during load` (플러그인) | 보통 미빌드/미설정 MCP 서버(jira-gateway 미빌드, chainbench 미설치). `/doctor` 로 확인 |
| jira_* 호출 401 | `jira.env` 미설정(CHANGE-ME). 토큰 채우고 재-source |
| 경로 바뀜 | `gen-cks-config.sh` 재실행(또는 `activate.sh` 가 자동) → config/env 재생성 |

---

## 8. 스크립트·산출물 인벤토리

```
code-knowledge-system/
├── activate.sh                          # on-demand env 로더 (source 해서 사용)
├── scripts/
│   ├── setup-all.sh                     # ★one-click 마스터★
│   ├── build-stablenet-dataset.sh       # 바이너리+데이터셋+config (STAGE/SKIP_CKV)
│   ├── gen-cks-config.sh                # cks-stablenet.yaml + cks.env 생성 (결정적)
│   ├── apply-cc-settings.sh             # ~/.claude/settings.json env 병합 (GUI/터미널 무관)
│   ├── enable-autopilot.sh              # go-stablenet bypassPermissions 생성 (idempotent)
│   ├── coding-agent.sh                  # ★자율 런처★ autopilot 보장 후 claude 기동
│   └── cks-health.sh                    # cks.ops.health 체크
├── cks-stablenet.yaml                   # ★생성★ 절대경로 cks 설정
├── cks.env                              # ★생성★ 경로 export (cks)
├── data/ckg-stablenet/graph.db          # ckg 그래프 (220,507 노드 / 1.93M 엣지)
├── ckv-stablenet/{vector.db,manifest}   # ckv 벡터 (19,807 청크, bge-m3 1024d)
├── generated/{domain-corpus,policies}/  # 채널② 코퍼스 + 정책(ckv14/ckg36)
└── docs/{SETUP.md, go-stablenet-dataset-build-manual.md}

~/.config/coding-agent/jira.env          # ★시크릿★ JIRA_* (리포 밖, 600, on-demand)
coding-agent/.claude-plugin/marketplace.json   # 플러그인 마켓플레이스 매니페스트(푸시됨)
coding-agent/tools/jira-gateway-mcp/bin/jira-gateway-mcp   # 빌드된 보안 프록시
```

> 생성·시크릿 파일은 커밋 금지: `cks-stablenet.yaml`, `cks.env`, `jira.env`,
> `data/`, `ckv-stablenet/` (`generated/`, `logs/` 는 cks `.gitignore` 에 이미 포함).
