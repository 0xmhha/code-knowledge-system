# 멀티 프로젝트 테스트 셋업 가이드 (coding-agent / cks / ckg / ckv)

서로 다른 코드베이스를 기준으로 여러 번 테스트를 수행할 때, `coding-agent`·`cks`·`ckg`·`ckv`에
각각 다른 **DB**와 **코드 경로**를 바인딩하는 방법을 정리한다.

> 환경 (이 문서 작성 시점)
> - cks   : `/Users/wm-it-25_0220/Work/github/code-knowledge-system`
> - ckg   : `/Users/wm-it-25_0220/Work/github/code-knowledge-graph`
> - ckv   : `/Users/wm-it-25_0220/Work/github/code-knowledge-vector`
> - 코드  : `/Users/wm-it-25_0220/Work/github/test/<proj>`         (편집·인덱싱 대상)
> - DB    : `/Users/wm-it-25_0220/Work/github/knowledge-data/<proj>` (DB 출력 위치)

---

## 1. 핵심 구조 — 무엇이 "재시작" 대상인가

먼저 흔한 오해부터 정리한다. **ckg와 ckv는 따로 떠 있는 서버가 아니다.**

| 구성요소 | 정체 | 전환 방식 |
|---|---|---|
| **ckg** | 빌드 시 `graph.db`를 만드는 CLI + cks가 **in-process로 읽는 DB 파일** | DB 파일 경로만 바꾸면 됨 (재시작 불필요) |
| **ckv** | 빌드 시 `vector.db`를 만드는 CLI + cks가 in-process로 읽음 (임베딩만 ollama 호출) | 동일 |
| **cks** | `cks-mcp` 서버. **기동 시 `CKS_CONFIG`를 1회 읽고 고정** | config 교체 + **MCP 서버 재기동(=세션 재시작)** |
| **coding-agent** | Claude Code 플러그인(파이프라인). cks를 MCP로 호출, 코드는 **실행 디렉토리(git repo)** 에서 편집 | 해당 repo에서 Claude 실행 |

따라서 프로젝트 전환 시 실제로 신경 쓸 것은 **두 개의 바인딩**뿐이다.

1. **DB 바인딩** = `CKS_CONFIG`가 가리키는 cks config (그 안에 ckg/ckv db 경로 + `source_root`)
2. **코드 바인딩** = Claude Code를 실행하는 git repo 디렉토리(cwd) — coding-agent가 여기를 편집

> ⚠️ **이 둘은 반드시 같은 코드를 가리켜야 한다.**
> cks config의 `source_root` == Claude Code를 실행한 repo 경로

### 환경변수 주입 지점

`CKS_CONFIG` / `CKS_MCP_BIN`은 셸 프로파일(`~/.zshrc` 등)이 아니라
**Claude Code 설정의 `env` 블록**에서 주입된다.

- 전역: `~/.claude/settings.json` → `env`
- 프로젝트: `<repo>/.claude/settings.local.json` → `env`  (전역을 override, git 비추적)

coding-agent 플러그인의 `plugin/.mcp.json`은 cks 서버를 다음과 같이 정의하므로,
기동 시점의 `CKS_CONFIG` 값으로 데이터셋이 결정된다.

```json
"cks": { "command": "${CKS_MCP_BIN}", "args": ["-config", "${CKS_CONFIG}"] }
```

---

## 2. 프로젝트별 준비 (코드 스냅샷당 1회, 오프라인)

DB 빌드는 재시작과 무관한 순수 파일 생성 작업이다. 코드가 바뀔 때마다 1회 수행한다.

```bash
CODE=/Users/wm-it-25_0220/Work/github/test/<proj>            # 편집·인덱싱 대상 코드
DATA=/Users/wm-it-25_0220/Work/github/knowledge-data/<proj>  # DB 출력 위치
CKG=/Users/wm-it-25_0220/Work/github/code-knowledge-graph/bin/ckg
CKV=/Users/wm-it-25_0220/Work/github/code-knowledge-vector/bin/ckv

mkdir -p "$DATA/ckg" "$DATA/ckv" "$DATA/logs/footprint" "$DATA/logs/audit"

# 1) ckg 그래프 DB  (go-ethereum 전체 트리 기준 약 7분)
$CKG build --src "$CODE" --out "$DATA/ckg" --lang go
$CKG validate --graph "$DATA/ckg" --format json   # Issues: null 이면 정상

# 2) ckv 벡터 DB  (약 30분, ollama + bge-m3 모델 필요)
#    사전: `ollama serve` 가 떠 있고 `ollama pull bge-m3` 완료되어 있어야 함
CKV_OLLAMA_ENDPOINT=http://localhost:11434 \
  $CKV build --embedder=ollama --model-name=bge-m3 --src "$CODE" --out "$DATA/ckv"
```

### 3) cks config 작성 — `$DATA/cks-<proj>.yaml`

`source_root` / ckg `path` / ckv `path` 세 곳이 **반드시 이 프로젝트 경로**여야 한다.

```yaml
# cks config for the <proj> dataset (독립 — 다른 프로젝트 DB와 연결 금지)
version: 1

backends:
  ckg:
    path: "<DATA>/ckg/graph.db"
    source_root: "<CODE>"          # ← Claude 실행 repo와 동일해야 함
    binary_path: "/Users/wm-it-25_0220/Work/github/code-knowledge-graph/bin/ckg"
    timeout_ms: 5000
  ckv:
    path: "<DATA>/ckv"
    timeout_ms: 3000
    embed_model: "bge-m3"
    ollama_url: "http://localhost:11434"
    binary_path: "/Users/wm-it-25_0220/Work/github/code-knowledge-vector/bin/ckv"

listen:
  http_addr: "127.0.0.1:8080"
  mcp_stdio: true

logging:
  level: "info"
  mode: "prod"
  footprint_dir: "<DATA>/logs/footprint"
  audit_dir: "<DATA>/logs/audit"

sanitize:
  rules_path: "/Users/wm-it-25_0220/Work/github/code-knowledge-system/policies/sanitization_rules.yaml"
  default_action: "drop"
  fail_closed_on_unknown_rule: true
```

---

## 3. 테스트 전환 방법 — 2가지

### 방식 A (권장): repo별 `.claude/settings.local.json`

각 코드 repo 안에 로컬 설정을 두면, **그 repo에서 Claude를 실행하는 것만으로** 올바른 DB에
자동 바인딩된다. 프로젝트 설정이 전역 설정을 override 하고, `.local`은 git에 올라가지 않으므로
**동시에 여러 프로젝트를 다른 세션에서 돌려도 충돌이 없다.**

`<CODE>/.claude/settings.local.json`:

```json
{
  "env": {
    "CKS_MCP_BIN": "/Users/wm-it-25_0220/Work/github/code-knowledge-system/bin/cks-mcp",
    "CKS_CONFIG":  "/Users/wm-it-25_0220/Work/github/knowledge-data/<proj>/cks-<proj>.yaml"
  }
}
```

사용법:
```bash
cd <CODE> && claude     # 이 디렉토리에서 띄우면 자동으로 <proj> DB에 바인딩
```

### 방식 B (간단): 전역 `~/.claude/settings.json` 편집

한 번에 하나씩 순차로 테스트할 때 사용. `env.CKS_CONFIG` 값을 대상 프로젝트 config로 바꾸고
세션을 재시작한다.

```jsonc
// ~/.claude/settings.json
"env": {
  "CKS_MCP_BIN": "/Users/wm-it-25_0220/Work/github/code-knowledge-system/bin/cks-mcp",
  "CKS_CONFIG":  "/Users/wm-it-25_0220/Work/github/knowledge-data/<proj>/cks-<proj>.yaml",
  ...
}
```

단점: 전역이라 동시에 다른 DB로 도는 세션과 값이 섞인다. (동시 테스트엔 방식 A를 쓸 것)

---

## 4. 재시작 + 검증 절차 (공통, 매 테스트 시작 시 필수)

1. (방식 A) 대상 repo의 `.claude/settings.local.json` 작성
   / (방식 B) 전역 `CKS_CONFIG` 수정
2. **Claude Code 완전 종료 후, 대상 코드 repo 디렉토리에서 재실행.**
   MCP 서버는 기동 시 env를 1회만 읽으므로, 세션 중간에 값을 바꿔도 무효다.
   ```bash
   cd <CODE> && claude
   ```
3. **바인딩 검증** — cks freshness 조회 결과의 `indexed_head` 가
   대상 코드의 `git -C <CODE> rev-parse HEAD` 와 일치하는지 확인.
   - 일치 → 올바른 DB
   - 불일치 → 잘못된 config로 떠 있음 (1번부터 재점검)

> 보조 검증 (셸에서 직접):
> ```bash
> # ckg
> .../code-knowledge-graph/bin/ckg validate --graph <DATA>/ckg --format json
> # ckv 시맨틱 쿼리
> CKV_OLLAMA_ENDPOINT=http://localhost:11434 \
>   .../code-knowledge-vector/bin/ckv query "검색어" --out <DATA>/ckv \
>   --embedder=ollama --model-name=bge-m3 --top 5
> # cks 통합 헬스 (cks repo에서)
> .../code-knowledge-system/scripts/cks-health.sh <DATA>/cks-<proj>.yaml
> ```

---

## 5. 주의사항 / 함정

- **MCP는 기동 시 1회만 env를 읽는다.** config를 바꿨으면 반드시 세션 재시작.
- **동시에 떠 있는 다른 세션의 cks-mcp는 각자 자기 config로 독립 동작한다.**
  `settings.json`을 바꿔도 이미 떠 있는 세션엔 영향이 없다(= 새 세션부터 적용).
- **전역 `~/.claude/settings.json`의 `env`는 모든 세션 공통.** 동시 멀티 테스트는 방식 A 필수.
- **코드 경로와 DB의 `source_root`가 불일치하면** retrieval 결과의 파일/라인이 편집 대상과
  어긋난다. 항상 같은 경로로 맞춘다.
- **프로젝트 간 DB를 섞지 말 것.** 각 `knowledge-data/<proj>`는 독립이며 서로 참조하지 않는다.
- ckv 빌드는 ollama 데몬 + `bge-m3` 모델이 필요하다. `ollama serve` 미기동 시 빌드 실패.

---

## 6. 현재 상태 (pr-77)

| 항목 | 상태 |
|---|---|
| ckg DB | ✅ `knowledge-data/pr-77/ckg/graph.db` (220,378 nodes / 1,905,296 edges) |
| ckv DB | ✅ `knowledge-data/pr-77/ckv/vector.db` (19,343 chunks, bge-m3 1024-dim) |
| cks config | ✅ `knowledge-data/pr-77/cks-pr77.yaml` (+ `cks-pr77.env`) |
| 인덱싱 커밋 | `0bf2f4d1bfeb6605006d556957ef8c045d8f8ed8` |
| **바인딩** | ❌ 아직 전역 `CKS_CONFIG`가 `cks-stablenet.yaml`(go-stablenet)을 가리킴 |

### pr-77 테스트 착수 절차

1. (다른 세션 테스트 종료 대기)
2. `/Users/wm-it-25_0220/Work/github/test/pr-77/.claude/settings.local.json` 생성 (방식 A,
   `<proj>=pr-77`)
3. `cd /Users/wm-it-25_0220/Work/github/test/pr-77 && claude` 로 재시작
4. cks freshness → `indexed_head: 0bf2f4d1...` 확인
5. `/coding-agent:analyze "..."` 로 `pr-77.md` 작업 진행
