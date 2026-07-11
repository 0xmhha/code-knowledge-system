---
flow: ep-keystore
entry_point: EP-FILE-KEYSTORE
trigger: "키스토어 디렉터리 스캔(파일 로드) + unlock/sign 요청"
root_symbol: "accounts/keystore.(*accountCache).scanAccounts"
summary: "디스크의 암호화 키 파일을 로드하고, 비밀번호로 복호화해 트랜잭션·해시 서명에 쓴다. 파일·비밀번호·잠금에서 갈린다."
links: [ep-rpc-sendrawtx]
called_by: [ep-main]
---

# Flow: EP-FILE-KEYSTORE — 키스토어 (파일 로드 + 서명, geth 원본)

> 기동 시 키스토어 디렉터리를 스캔해 계정을 인지하고, 서명 요청 시 키를 복호화해 사용한다. 전부 geth
> 원본. (검증자 합의 서명은 별개 — 노드 키에서 파생한 BLS 키, [ep-main](ep-main.md) main-03.)

### STEP keystore-01
- symbol: `accounts/keystore.(*accountCache).scanAccounts` / `maybeReload`
- at: `accounts/keystore/account_cache.go:241` / `:202`
- kind: geth
- calls: [keystore-02]
- reads: 키스토어 디렉터리의 키 파일들(EP-FILE-KEYSTORE)
- writes: 계정 캐시(주소→파일 경로)
- emits: —
- branches:
  - when: "디렉터리에 새/변경 파일" → then: "캐시 갱신(add/delete)" at: `accounts/keystore/account_cache.go:202`
  - when: "키 파일 JSON 파싱 실패/주소 불일치" → then: "해당 파일 스킵(경고)" at: `accounts/keystore/account_cache.go:241`
- invariant: —
- prose: 키스토어 디렉터리를 스캔해 키 파일에서 계정 주소를 읽어 캐시에 올린다(주소→파일 매핑). 파일이 깨졌거나 내부 주소가 파일명과 안 맞으면 그 파일은 건너뛴다. 디렉터리 변경은 주기적으로 반영한다.

### STEP keystore-02
- symbol: `(*KeyStore).Unlock` / `TimedUnlock` → `getDecryptedKey` → `DecryptKey`
- at: `accounts/keystore/keystore.go:322` / `:345` → `:383`
- kind: geth
- calls: [keystore-03]
- reads: 키 파일(scrypt/AES 암호화), 비밀번호
- writes: 잠금 해제된 키를 메모리에 보관(unlocked 맵)
- emits: —
- branches:
  - when: "계정이 캐시에 없음" → then: "ErrNoMatch" at: `accounts/keystore/keystore.go:383`
  - when: "비밀번호 틀림(복호화 실패)" → then: "에러(키 노출 안 됨)" at: `accounts/keystore/keystore.go:383`
  - when: "TimedUnlock & 타임아웃 경과" → then: "키를 메모리에서 지움(재잠금)" at: `accounts/keystore/keystore.go:345`
- invariant: —
- prose: 계정을 비밀번호로 복호화해 평문 키를 메모리에 올린다. 계정이 없으면 ErrNoMatch, 비밀번호가 틀리면 복호화 실패로 거부한다(키는 노출되지 않음). TimedUnlock은 지정 시간 뒤 키를 메모리에서 지워 재잠금한다.

### STEP keystore-03
- symbol: `(*KeyStore).SignTx` / `SignHash`
- at: `accounts/keystore/keystore.go:277` / `:263`
- kind: geth
- calls: [ep-rpc-sendrawtx]   # 서명된 tx가 제출되면 이 경로로
- reads: 메모리의 잠금 해제 키, 서명 대상(tx/hash)
- writes: —
- emits: 서명된 트랜잭션/서명값
- branches:
  - when: "계정이 잠겨 있음(unlocked에 없음)" → then: "ErrLocked" at: `accounts/keystore/keystore.go:277`
  - when: "잠금 해제됨" → then: "ECDSA 서명 후 서명된 tx 반환" at: `accounts/keystore/keystore.go:277`
  - when: "비밀번호 동반 서명(SignTxWithPassphrase)" → then: "일시 복호화→서명→폐기" at: `accounts/keystore/keystore.go:305`
- invariant: —
- prose: 잠금 해제된 키로 트랜잭션/해시에 ECDSA 서명을 한다. 계정이 잠겨 있으면 ErrLocked로 거부하고, 비밀번호 동반 서명은 일시 복호화 후 즉시 폐기한다. 서명된 트랜잭션이 제출되면 ep-rpc-sendrawtx 경로로 흐른다.

---

## prose 렌더 (분기/실패 포함)
> 키스토어는 기동 시 디렉터리를 스캔해 키 파일에서 계정 주소를 캐시에 올린다(깨진 파일은 스킵). 서명을
> 하려면 먼저 `Unlock`이 비밀번호로 키를 복호화해 메모리에 두는데, 계정이 없으면 ErrNoMatch·비밀번호가
> 틀리면 복호화 실패로 거부한다(키 비노출). TimedUnlock은 시간이 지나면 재잠금한다. `SignTx`는 잠금 해제된
> 키로 서명하고(잠겨 있으면 ErrLocked), 비밀번호 동반 서명은 일시 복호화 후 폐기한다. 서명된 tx는
> ep-rpc-sendrawtx로 제출된다.
