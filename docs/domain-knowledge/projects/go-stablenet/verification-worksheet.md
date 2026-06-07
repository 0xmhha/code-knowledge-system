# Verification Worksheet — `go-stablenet`

> **Generated**: 2026-06-07 by `cks-promotion-worksheet`
> **Filter**: status=`needs_verification`, priority=`(all)`
> **Entries in queue**: 29

## Session header (one per session, fill once)

- **Project**: `go-stablenet` @ `<commit-sha>`
- **Session date**: ____________
- **Domain expert**: ____________ · **Operator**: ____________
- **Pre-flight**: all anchors are file+line+symbol consistent (`cks-inventory-check` passes 0 errors).

### Reference catalogs (keep open in another tab)

- Top hallucination risks: `coding-agent/docs/r1-refactor/07-domain-knowledge-curation.md` §9
- T2 trap catalog: `coding-agent/docs/r1-refactor/08-p0c-foundations-t2-and-internalization.md` §4
- Hallucination-risk re-statement: same file, §9
- Status policy: `cks/docs/domain-knowledge/shared/STATUS_LIFECYCLE.md` §verification-checklist

### Decision dictionary

- **APPROVE** → entry promoted to `verified`; run the per-entry promotion command at the bottom of the section.
- **REVISE** → return to author with the reviewer notes filled in; entry stays `needs_verification`.
- **REJECT** → archive entry (rare; only if the trap is wrong, not just imprecise).

---

## Entries

### `A1.concurrency.core_lock_discipline` · `[P0 · A1 · B4 · risk:high]`

**Title**: WBFT Core lock discipline — never read Core.current off currentMutex
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 7** (concurrency (Core.current off mutex, txpool mutation))

**Summary** (read first):

> The WBFT Core's `current` round-state pointer is guarded by
> `currentMutex` (consensus/wbft/core/core.go:106 — a sync.RWMutex).
> Every accessor that reads `c.current` MUST hold currentMutex.RLock
> (read) or currentMutex.Lock (write). Examples in-repo: the
> current-proposal accessor at core.go:155-157 acquires RLock around
> the read; the round-state mutator at core.go:162-166 acquires
> Lock for the entire write region.
> 
> The priorState struct (core.go:72) embeds its own sync.RWMutex —
> this is a SEPARATE lock, NOT a substitute. Reading `c.current.X`
> while holding only priorState's lock (or no lock) is a data race
> against round-change writers and produces non-deterministic
> consensus outcomes (e.g. accepting a Prepare for a stale round).
> 
> The trap: an LLM adding telemetry, a new RPC, or a fast-path early
> return that touches `c.current` outside the locked region. The
> race is subtle — tests pass under low contention — but under real
> network load it shows up as occasional fork or stall. Any new
> read of `c.current` (or its sub-fields like Sequence/Round/
> pendingRequest) MUST be inside currentMutex's read region; any
> write MUST be inside the write region.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/core/core.go:106` · `Core.currentMutex` — Round-state RWMutex — guards every access to c.current
2. `consensus/wbft/core/core.go:155` — Example read region: RLock → read c.current → RUnlock
3. `consensus/wbft/core/core.go:162` — Example write region: Lock → mutate c.current → Unlock
4. `consensus/wbft/core/core.go:72` · `priorState` — priorState carries its own RWMutex — separate from currentMutex, NOT a substitute

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `Core.current` · `RLock` · `RWMutex` · `currentMutex` · `priorState`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Every read of c.current (and sub-fields Sequence/Round/pendingRequest) is inside currentMutex.RLock().*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/core.go:155)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *Every write of c.current is inside currentMutex.Lock(); the write region covers the full mutation, not just the pointer swap.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/core.go:162)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *priorState.RWMutex is a separate lock for prior-state snapshots, not a substitute for currentMutex.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/core.go:72 `priorState`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Reading c.current.Sequence() for telemetry without acquiring currentMutex.RLock — data race.*
  > expected failure mode:
- **P2.** *Acquiring priorState's lock instead of currentMutex — wrong lock, same struct family.*
  > expected failure mode:
- **P3.** *Partial write under Lock (e.g. allocating a new state outside Lock, then swapping the pointer inside) — readers may observe a half-initialized state.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A1.concurrency.core_lock_discipline \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A1.wbft_core.consensus_flow_architecture` · `[P0 · A1 · B1 · risk:low]`

**Title**: WBFT consensus flow — actors and message sequence per height
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 1** (stake-weighted voting / slashing)

**Summary** (read first):

> The WBFT consensus engine drives one block per height through four
> phases inside Core: handleRequest emits a Preprepare from the
> current proposer; every validator broadcasts a Prepare in response;
> once each validator has seen a Prepare quorum it broadcasts a Commit;
> once each validator has seen a Commit quorum the block is finalised
> and Core advances to the next height via startNewRound. RoundChange
> fires on RequestTimeout and selects a new proposer for the same
> height. Backend (consensus/wbft/backend) is the bridge between Core
> and the blockchain — it broadcasts/gossips messages, signs with BLS,
> fetches Validators, and commits sealed blocks back. Engine
> (consensus/wbft/engine) handles header validation, BaseFee
> distribution, and epoch construction. The Istanbul/100 P2P
> subprotocol (A9) carries the messages between nodes.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/core/core.go:167` · `Core.startNewRound` — entry point for each new round/sequence — orchestrates state transitions
2. `consensus/wbft/core/handler.go` — event loop, message routing into handle{Preprepare,Prepare,Commit,RoundChange}Msg
3. `consensus/wbft/core/preprepare.go` — Preprepare generation/validation
4. `consensus/wbft/core/prepare.go` — Prepare validation + quorum check that drives StatePrepared
5. `consensus/wbft/core/commit.go` — Commit validation + quorum check that drives StateCommitted
6. `consensus/wbft/core/roundchange.go` — RoundChange protocol on RequestTimeout
7. `consensus/wbft/backend/backend.go` — Backend interface implementation — blockchain ↔ Core bridge
8. `consensus/wbft/backend/engine.go` — epoch build, validator lookup, BLS signing
9. `consensus/wbft/engine/engine.go` — header verification, processFinalize where system-contract upgrades land

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `Backend` · `Core` · `RequestTimeout` · `StateAcceptRequest` · `StateCommitted` · `StatePrepared` · `StatePreprepared` · `broadcastCommit` · `broadcastPrepare` · `commitWBFT` · `handleCommitMsg` · `handlePrepareMsg` · `handlePreprepareMsg` · `handleRequest` · `handleRoundChangeMsg` · `sendPreprepareMsg` · `startNewRound`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A1.wbft_core.consensus_flow_architecture \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A1.wbft_core.message_handlers` · `[P0 · A1 · B3 · risk:low]`

**Title**: WBFT consensus message handlers: preprepare, prepare, commit
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 4** (quorum reimplementation (ceil(N−F) split-brain))

**Summary** (read first):

> Three handlers drive the per-round state machine. handlePreprepareMsg
> validates the proposer's block and transitions to StatePreprepared;
> handlePrepareMsg accumulates PREPARE votes and transitions to
> StatePrepared at quorum; handleCommitMsg accumulates COMMIT votes and
> finalises the block at quorum. Every handler verifies the message
> signature, the sequence/round view, and the sender's authorisation
> before applying state changes.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/core/preprepare.go:113` · `Core.handlePreprepareMsg` — validates proposer signature and block, transitions to StatePreprepared
2. `consensus/wbft/core/prepare.go:87` · `Core.handlePrepareMsg` — accumulates PREPARE messages, transition to StatePrepared at QuorumSize
3. `consensus/wbft/core/commit.go:90` · `Core.handleCommitMsg` — accumulates COMMIT messages, finalises block at QuorumSize

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `StateCommitted` · `StatePrepared` · `StatePreprepared` · `WBFTCommits` · `WBFTPrepares` · `handleCommitMsg` · `handlePrepareMsg` · `handlePreprepareMsg`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Each handler is idempotent for duplicate messages from the same sender at the same view (sequence, round).*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/prepare.go:87 `Core.handlePrepareMsg`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *Per-round state progresses strictly StatePreprepared -> StatePrepared -> StateCommitted; out-of-order messages are buffered or rejected.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/prepare.go:87 `Core.handlePrepareMsg`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Treating a PREPARE quorum as committable proof: PREPARE quorum only justifies the prepared block inside ROUND-CHANGE messages. Block finalisation requires a COMMIT quorum.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A1.wbft_core.message_handlers \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A1.wbft_core.quorum_calc` · `[P0 · A1 · B3 · risk:high]`

**Title**: WBFT quorum size calculation
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 4** (quorum reimplementation (ceil(N−F) split-brain))

**Summary** (read first):

> WBFT requires QuorumSize = ceil(N - F) where N is the validator set
> size and F = (N-1)/3 evaluated in float64. The quorum is the minimum
> count of Prepare or Commit messages needed before the consensus core
> advances state. State advancement in handlePrepareMsg uses the
> expression WBFTPrepares.Size() >= valSet.QuorumSize().
> 
> ceil(N - F) is the AUTHORITATIVE quorum; any reimplementation MUST
> reproduce it exactly. The dangerous refactor is not int-vs-float (see
> the correctness note below) but swapping the FORMULA: the PBFT-textbook
> 2f+1 or a simple majority N/2+1 DIVERGE from ceil(N - F) for N not of
> the form 3f+1, and a lower quorum finalizes with too few honest nodes
> (a safety violation). Examples: N=5 -> ceil=4 but 2f+1=3; N=6 -> ceil=5
> but 2f+1=3; N=8 -> ceil=6 but 2f+1=5.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/validator/default.go:226` · `defaultSet.F` — F = (N-1)/3 as float64 — integer cast would lose precision
2. `consensus/wbft/validator/default.go:230` · `defaultSet.QuorumSize` — QuorumSize = ceil(N - F) using math.Ceil over float64
3. `consensus/wbft/core/prepare.go:87` · `Core.handlePrepareMsg` — threshold check that gates StatePrepared transition
4. `consensus/wbft/types.go:216` · `ValidatorSet` — interface declaration; F() returns float64 (not int)

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `F()` · `QuorumSize` · `StatePrepared` · `ValidatorSet` · `WBFTPrepares` · `defaultSet` · `math.Ceil`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *QuorumSize is exactly ceil(N - F) with F = (N-1)/3; the live code computes it in float64 via math.Ceil (default.go:226). Any alternative formula must yield the identical value for every N.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:230 `defaultSet.QuorumSize`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *QuorumSize is always >= 1, including N = 1 (ceil(1 - 0) = 1).*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:230 `defaultSet.QuorumSize`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *Prepare/Commit state transitions compare with >= QuorumSize, not >.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:230 `defaultSet.QuorumSize`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Reimplementing the quorum with a DIFFERENT formula. PBFT-textbook 2f+1 and simple-majority N/2+1 diverge from ceil(N-F) for N != 3f+1, producing a LOWER quorum that finalizes with too few honest nodes (safety violation). N=5: ceil=4, 2f+1=3; N=6: ceil=5, 2f+1=3; N=8: ceil=6, 2f+1=5.*
  > expected failure mode:
- **P2.** *Forgetting that QuorumSize covers both Prepare and Commit independently — each phase needs its own quorum.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A1.wbft_core.quorum_calc \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A1.wbft_core.round_change_protocol` · `[P0 · A1 · B3 · risk:low]`

**Title**: WBFT round change: timeout-triggered round bump with prepared-block justification
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 2** (reorg / probabilistic-finality (forker, Td inert under WBFT))

**Summary** (read first):

> WBFT recovers from a stuck round (PRE-PREPARE or COMMIT quorum not
> reached within RequestTimeout) by broadcasting a ROUND-CHANGE message
> for round+1. The message carries the highest prepared block and its
> PREPARE justification from prior rounds; a node accepts the view
> change once it has collected a quorum of ROUND-CHANGE messages for
> the same target round.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/core/roundchange.go:40` · `Core.broadcastNextRoundChange` — entry point fired on RequestTimeout; increments round and broadcasts a signed ROUND-CHANGE
2. `consensus/wbft/core/roundchange.go:52` · `Core.broadcastRoundChange` — builds and signs ROUND-CHANGE for a target round, attaches prepared-block justification
3. `consensus/wbft/core/roundchange.go:103` · `Core.handleRoundChangeMsg` — validates and accumulates incoming ROUND-CHANGE messages, triggers view change at quorum
4. `consensus/wbft/messages/roundchange.go:37` · `RoundChange` — message struct: Sequence, Round, PreparedRound, PreparedBlock
5. `consensus/wbft/messages/roundchange.go:64` · `SignedRoundChangePayload` — signed payload, encodePayloadInternal defines the digest used for signing

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `PreparedBlock` · `PreparedRound` · `RoundChange` · `SignedRoundChangePayload` · `StateNewRound` · `broadcastNextRoundChange` · `broadcastRoundChange` · `handleRoundChangeMsg`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *A ROUND-CHANGE carries the highest prepared block this node has seen, so a quorum of view changes resumes from that prepared state and never from a lower round.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/roundchange.go:52 `Core.broadcastRoundChange`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *ROUND-CHANGE quorum threshold matches the PREPARE/COMMIT quorum (see A1.wbft_core.quorum_calc).*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/roundchange.go:103 `Core.handleRoundChangeMsg`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Bumping rounds without attaching the prepared-block justification when one exists: the cluster can fork by accepting two different blocks at different rounds for the same height.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A1.wbft_core.round_change_protocol \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A10.codegen.contract_regen_procedure` · `[P2 · A10 · B6 · risk:low]`

**Title**: System contract artifact regeneration: solc 0.8.14 to artifacts/v1 and v2
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: (no automatic catalog match — reviewer to assign)

**Summary** (read first):

> System contract bytecode (governance v1 and v2, NativeCoinAdapter) is
> produced by systemcontracts/compile. The tool downloads solc 0.8.14
> to ~/.solc-bin if absent, invokes it on the Solidity sources under
> systemcontracts/solidity/, and writes binaries to
> systemcontracts/artifacts/v1 and artifacts/v2/. Those binaries are
> go:embed'd into contracts.go and consumed at genesis time via
> InjectContracts. The do-not-edit constraint on the artifacts (see
> A10.codegen.no_edit_zones) is enforced by re-running this procedure,
> not by static checks.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `systemcontracts/compile/main.go` — entry point; orchestrates compile and ExportContractCode for v1 and v2 sources
2. `systemcontracts/compile/compiler/compiler.go:37` · `solcVersion` — pinned solc version; bump here when intentionally upgrading the compiler
3. `systemcontracts/compile/compiler/compiler.go:40` · `Compile` — downloads solc to ~/.solc-bin, runs it, parses ParseCombinedJSON output
4. `systemcontracts/compile/compiler/compiler.go:123` · `compiledTy.ExportContractCode` — writes the bytecode files under systemcontracts/artifacts/
5. `systemcontracts/contracts.go` — go:embed lines that consume the artifacts; new versions are wired here

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `CONTRACT_GOV_MINTER` · `Compile` · `ExportContractCode` · `GovMinterContractV1` · `GovMinterContractV2` · `SystemContractCodes` · `go:embed` · `solcVersion`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q5] Procedure steps — does each step still match current tooling/code paths?**

- **S1.** *Edit the Solidity sources under systemcontracts/solidity/v1/ or v2/ as appropriate.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S2.** *Run `go run ./systemcontracts/compile`. The first run downloads solc 0.8.14 into ~/.solc-bin if absent.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S3.** *Verify systemcontracts/artifacts/v1/<ContractName> or v2/<ContractName> were updated; the binary content is hex-encoded bytecode.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S4.** *When adding a new version (e.g. v3), add a go:embed line in systemcontracts/contracts.go pointing at artifacts/v3/<ContractName> and register it in SystemContractCodes.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S5.** *Run go test ./systemcontracts/... and consensus integration tests to confirm the new artifact deploys correctly under InjectContracts.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S6.** *Do not hand-edit anything under systemcontracts/artifacts/; those files are codegen output (see A10.codegen.no_edit_zones).*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A10.codegen.contract_regen_procedure \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A10.codegen.no_edit_zones` · `[P1 · A10 · B4 · risk:low]`

**Title**: Codegen no-edit zones: which files are generated and how to regenerate
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 3** (ETH-denominated assumptions (WKRC, base-fee redistribution))

**Summary** (read first):

> Four categories of files in go-stablenet are produced by tools and
> must never be hand-edited: trezor protobuf bindings under
> accounts/usbwallet/trezor/*.pb.go (regenerated with protoc), JSON
> marshalling helpers core/types/gen_*.go and similar in beacon/,
> tests/, eth/tracers/, cmd/evm/ (regenerated with fjl/gencodec), RLP
> helpers core/types/gen_*_rlp.go (regenerated with rlp/rlpgen), and
> Solidity bytecode artifacts under systemcontracts/artifacts/v{N}/
> (regenerated by go run ./systemcontracts/compile). Direct edits
> drift the file out of sync with its source and the next regenerator
> run produces a noisy CI-breaking diff. Adding a new code-generated
> file (or a new Solidity contract version) requires registering it
> in the corresponding generator config (e.g. compile/main.go for
> Solidity) so subsequent runs preserve it.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `accounts/usbwallet/trezor/messages.pb.go` — protobuf-generated; regenerator command lives at the top of the file
2. `core/types/gen_account.go` — gencodec example — header comment names the type and override file
3. `core/types/gen_account_rlp.go` — rlpgen example — header comment names the source struct
4. `systemcontracts/artifacts/v1/GovValidator` — embedded Solidity bytecode for v1 GovValidator
5. `systemcontracts/compile/main.go` — Solidity compile driver — srcFiles/contractBins index pairs must stay aligned

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `SystemContractCodes` · `artifacts` · `gen_account_rlp` · `gen_account` · `gen_header_json` · `gencodec` · `go:embed` · `protoc` · `rlpgen`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Files in systemcontracts/artifacts/v{N}/ mirror the corresponding Solidity sources in systemcontracts/solidity/v{N}/. Any divergence is a regenerate-or-revert situation.*
  - [ ] ✅ enforced at `__________________` (suggested: systemcontracts/artifacts/v1/GovValidator)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *compile/main.go's srcFiles[i] must align with contractBins[i] index-for-index. A misaligned pair compiles successfully but registers bytecode under the wrong contract name.*
  - [ ] ✅ enforced at `__________________` (suggested: systemcontracts/compile/main.go)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *gen_*.go and gen_*_rlp.go files start with a code-generated header. Editing past the header is allowed only when also updating the source type and re-running the generator.*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/gen_account.go)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I4.** *Lint is configured to exclude core/genesis_alloc.go (large generated file) and crypto/bn256/ (third-party); other generated files are not excluded — they pass lint as-is because the generator emits compliant code.*
  - [ ] ✅ enforced at `__________________` (suggested: accounts/usbwallet/trezor/messages.pb.go)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Hand-editing systemcontracts/artifacts/v{N}/<Contract> to apply a quick fix — the next compile run overwrites the change, and CI catches the resulting diff on every PR.*
  > expected failure mode:
- **P2.** *Adding a Solidity contract without appending to both srcFiles and contractBins in compile/main.go — compile succeeds but go:embed fails at build time because the artifact file is missing.*
  > expected failure mode:
- **P3.** *Editing a gen_*.go file directly instead of updating the source type and rerunning gencodec — the next regen overwrites the edit.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A10.codegen.no_edit_zones \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A12.seals.bls_seal_scheme` · `[P0 · A12 · B2 · risk:high]`

**Title**: WBFT BLS seal scheme — 96-byte aggregate + SealerSet bitpack
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 3** (ETH-denominated assumptions (WKRC, base-fee redistribution))

**Summary** (read first):

> WBFT consensus seals are BLS aggregate signatures. The data structure
> is WBFTAggregatedSeal (core/types/istanbul.go:52) carrying a
> SealerSet bitpack (which validators signed, by ordered index) plus
> the aggregate 96-byte BLS signature. Two such seals live in
> WBFTExtra (core/types/istanbul.go:81): PrevPreparedSeal and
> PrevCommittedSeal, the latter being the previous block's commit
> attestation. BLS primitives are in crypto/bls/.
> 
> The trap: an LLM editing seal encoding, hashing, or verification
> often assumes Ethereum's secp256k1 ECDSA signatures (65 bytes,
> recoverable v/r/s) and mis-sizes buffers, mis-computes hashes (using
> Keccak over the wrong payload), or attempts ecrecover on a BLS
> signature. The SealerSet bitpack also encodes WHO signed; dropping
> or reordering bits changes the attested validator set without
> changing the signature, which a naive verifier will accept. Any
> change to extra-data, seal serialization, or filtered-header hashing
> MUST preserve the WBFTAggregatedSeal byte layout exactly.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/types/istanbul.go:58` · `WBFTAggregatedSeal` — Aggregate seal struct — Sealers (bitpack) + Signature ([96]byte BLS)
2. `core/types/istanbul.go:87` · `WBFTExtra` — Block-header extradata — embeds PrevPreparedSeal + PrevCommittedSeal
3. `core/types/istanbul.go:92` · `WBFTExtra.PrevCommittedSeal` — Previous block's commit attestation — verified during block insert

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `BLS` · `PrevCommittedSeal` · `PrevPreparedSeal` · `SealerSet` · `WBFTAggregatedSeal` · `WBFTExtra`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *WBFT seals are BLS aggregate (96 bytes), not ECDSA (65 bytes).*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/istanbul.go:58 `WBFTAggregatedSeal`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *SealerSet bitpack identifies the validator subset whose votes are aggregated; the bit layout is part of consensus.*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/istanbul.go:58 `WBFTAggregatedSeal`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *Filtered-header hash excludes seal fields, so seals can be added post-hash without changing the block hash.*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/istanbul.go:87 `WBFTExtra`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Calling ecrecover on a BLS signature — wrong primitive, returns garbage.*
  > expected failure mode:
- **P2.** *Hashing the seal payload with Keccak before BLS verify — primitives have their own hash-to-curve.*
  > expected failure mode:
- **P3.** *Reordering SealerSet bits — same signature now claims a different validator subset.*
  > expected failure mode:
- **P4.** *Embedding ECDSA-sized buffers ([65]byte) when serializing extra-data.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A12.seals.bls_seal_scheme \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A12.seals.feepayer_sighash` · `[P0 · A12 · B3 · risk:high]`

**Title**: Fee-delegation sigHash — feepayer signs the wrapped, not the inner, tx
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 5** (feepayer sigHash payload)

**Summary** (read first):

> StableNet's FeeDelegateDynamicFeeTx (tx type 0x16) is DOUBLE-signed:
> the sender signs the inner DynamicFee tx, then the feepayer signs a
> WRAPPER over [[inner-incl-sender-V/R/S], FeePayer]. The feepayer's
> sigHash is therefore computed by RLP-encoding the inner tx WITH the
> sender's V/R/S already populated, followed by the FeePayer address —
> NOT by re-hashing the bare inner tx. See sigHash at
> core/types/tx_fee_delegation.go:158 (the prefix-list payload at line
> 170-178 explicitly includes the inner tx values then FeePayer);
> setSignatureValues at :147 places the feepayer signature on the
> outer struct without touching the sender's signature.
> 
> The trap: a naive sigHash implementation that hashes only the inner
> payload (omitting V/R/S) or omits the FeePayer trailing field will
> produce a different digest, accept a valid-looking but unrelated
> signature, and allow a feepayer to be substituted server-side
> without the sender's consent. Worse, the chain will still accept the
> malformed tx if the verifier follows the same broken hash. Any
> refactor of fee-delegation signing must preserve the exact
> payload-shape (inner-with-VRS, then FeePayer) byte for byte.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/types/tx_fee_delegation.go:158` · `FeeDelegateDynamicFeeTx.sigHash` — Feepayer sigHash construction — payload is [[inner incl. sender V/R/S], FeePayer]
2. `core/types/tx_fee_delegation.go:147` · `FeeDelegateDynamicFeeTx.setSignatureValues` — Feepayer signature setter on the outer struct
3. `core/types/tx_fee_delegation.go:29` · `FeeDelegateDynamicFeeTx.FeePayer` — FeePayer field — must be the trailing element in the sigHash payload

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `FeeDelegateDynamicFeeTx` · `FeePayer` · `feePayer` · `setSignatureValues` · `sigHash`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Feepayer sigHash payload = [[inner-tx-fields including sender V/R/S], FeePayer].*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/tx_fee_delegation.go:158 `FeeDelegateDynamicFeeTx.sigHash`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *The sender's signature is part of the feepayer's signed payload — feepayer cannot be substituted without invalidating the chain.*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/tx_fee_delegation.go:158 `FeeDelegateDynamicFeeTx.sigHash`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *FeePayer is the trailing field; its position is part of the consensus hash.*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/tx_fee_delegation.go:29 `FeeDelegateDynamicFeeTx.FeePayer`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Hashing only the inner tx (omitting sender V/R/S) — feepayer substitution becomes possible.*
  > expected failure mode:
- **P2.** *Omitting FeePayer from the payload — wrapper signature collides with inner-only tx hash.*
  > expected failure mode:
- **P3.** *Reordering payload fields to 'match' DynamicFeeTx — breaks signature compatibility.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A12.seals.feepayer_sighash \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A13.sealing.reorg_serialization` · `[P0 · A13 · B4 · risk:high]`

**Title**: Txpool mutation must serialize through the reorg loop
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 7** (concurrency (Core.current off mutex, txpool mutation))

**Summary** (read first):

> The legacy txpool runs a single goroutine — `pool.loop()` (started at
> core/txpool/legacypool/legacypool.go:366 `go pool.loop()`) — that
> owns the txpool's internal maps (pending/queue/all). All mutation
> events (head change, new tx, reorg) are queued and applied INSIDE
> that loop, which is why the txpool data structures are not
> individually locked: the loop is the serializer.
> 
> The sealer (miner/) and the txpool reorg loop interact across this
> boundary: when the sealer requests a payload, it observes the
> txpool's state at a point AFTER a reorg-run; mutating txpool maps
> directly from the sealer (or from a tx-submission handler) WITHOUT
> going through the loop is a data race that can either lose pending
> txs or double-include them.
> 
> The trap: an LLM adding a "fast path" that mutates pool.pending /
> pool.queue / pool.all from outside the loop (e.g. inside an RPC
> handler, or inside the sealer's payload-building hook). Symptoms are
> intermittent: tx drops, double-spend rejection on a tx that was
> validly removed, occasional miner crash on map iteration. Any new
> txpool mutation MUST enqueue into the loop's channel, not call the
> map mutators directly.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/txpool/legacypool/legacypool.go:378` · `LegacyPool.loop` — The single serializer goroutine that owns txpool maps
2. `core/txpool/legacypool/legacypool.go:366` — `go pool.loop()` — sole owner of txpool mutation
3. `core/txpool/legacypool/legacypool.go:1289` · `queueTxEvent` — Correct enqueue path: requests reorg-run to apply the mutation

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `LegacyPool` · `changesSinceReorg` · `pool.loop` · `queueTxEvent` · `reorg`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Txpool maps (pending/queue/all) are mutated ONLY by `pool.loop()`.*
  - [ ] ✅ enforced at `__________________` (suggested: core/txpool/legacypool/legacypool.go:366)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *External producers (RPC, sealer, p2p) enqueue events; they do NOT touch the maps directly.*
  - [ ] ✅ enforced at `__________________` (suggested: core/txpool/legacypool/legacypool.go:378 `LegacyPool.loop`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *The reorg loop is the serialization boundary — there is no per-map lock to fall back on.*
  - [ ] ✅ enforced at `__________________` (suggested: core/txpool/legacypool/legacypool.go:366)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Mutating pool.pending / pool.queue from a non-loop goroutine — data race on map iteration.*
  > expected failure mode:
- **P2.** *Adding a sealer fast-path that filters pool.all without going through the loop's snapshot mechanism.*
  > expected failure mode:
- **P3.** *Assuming a per-map sync.Mutex exists — the design uses the loop as the lock, not per-map mutexes.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A13.sealing.reorg_serialization \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.foundations.base_fee_redistribution` · `[P0 · A14 · B1 · risk:high]`

**Title**: Base fee is redistributed to validators (not burned)
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 3** (ETH-denominated assumptions (WKRC, base-fee redistribution))

**Summary** (read first):

> Ethereum EIP-1559 BURNS the base fee. go-stablenet does NOT — under
> WBFT the base fee is REDISTRIBUTED to validators as part of block
> finalization, because StableNet's economic model treats validators
> as the operational counterparty rather than burning the native
> stablecoin (WKRC) supply away. The redistribution happens in the WBFT
> engine's Finalize path: consensus/wbft/backend/engine.go:174
> Backend.Finalize delegates to the underlying Engine().Finalize(),
> which performs the per-validator base-fee crediting (see also A3
> validator-set + A6 fee-delegation for the tip path).
> 
> The trap: an LLM editing finalization, fee accounting, or block
> reward code may apply Ethereum's burn assumption — leaving the base
> fee unaccounted, or worse, calling state.SubBalance against a "burn
> address." Both break WKRC supply integrity and the validator
> incentive contract. Any change to fee accounting must preserve the
> redistribution invariant and be cross-checked against the WBFT
> Finalize implementation, not against Ethereum EIP-1559 references.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/backend/engine.go:174` · `Backend.Finalize` — Finalize entry point — delegates to Engine().Finalize(); base-fee redistribution lives in that path (StableNet diverges from EIP-1559 burn here)
2. `params/config.go:100` — WKRC native-coin definition (name/symbol/currency) — base fee is denominated in WKRC, not ETH

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `Backend` · `BaseFee` · `Engine` · `Finalize` · `WKRC`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Base fee is redistributed to validators on Finalize; it is NOT burned.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/backend/engine.go:174 `Backend.Finalize`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *Total WKRC supply moved by Finalize sums to zero across {fee debit, validator credit} (no burn sink).*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/backend/engine.go:174 `Backend.Finalize`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Applying EIP-1559 burn semantics ('basefee → 0x0...0') in fee accounting.*
  > expected failure mode:
- **P2.** *Skipping the base-fee credit path during Finalize refactors.*
  > expected failure mode:
- **P3.** *Treating base fee as protocol revenue rather than validator payout.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.foundations.base_fee_redistribution \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.foundations.cherry_pick_principle` · `[P0 · A14 · B6 · risk:high]`

**Title**: Isolate StableNet code so upstream geth fixes stay cherry-pickable
**Source of truth**: `docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 8** (breaking cherry-pick-ability (interleave StableNet into geth))

**Summary** (read first):

> go-stablenet keeps an active "cherry-pick channel" from upstream
> go-ethereum: when upstream lands a security or correctness fix, the
> StableNet team wants to cherry-pick it cleanly. To preserve that,
> StableNet-unique code is ISOLATED into new files, not interleaved
> into geth files. Canonical example: WBFT P2P glue lives in
> `eth/handler_istanbul.go` (a NEW file), not as edits inside
> `eth/handler.go` (geth's file). The "StableNet 고유" column in
> `.claude/docs/build-source-files.md` (table at :114, :140, :176,
> :296) enumerates which files are unique to StableNet vs. inherited
> from geth; that mapping IS the cherry-pick boundary.
> 
> The trap: an LLM "fixing" or "tidying" geth-origin code by inlining
> StableNet logic into it (e.g. adding a feepayer-sigHash branch
> inside `core/types/transaction_signing.go` instead of the existing
> `tx_fee_delegation.go`) silently destroys cherry-pickability — the
> next upstream merge will conflict in subtle, security-relevant
> spots. Any new logic that is StableNet-specific must live in a
> StableNet-owned file; only minimal hook points (interface
> satisfaction, dispatch glue) belong in geth files.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `.claude/docs/build-source-files.md:362` · `StableNet 고유 코드 식별` — First StableNet-unique file table — the canonical cherry-pick boundary mapping
2. `CLAUDE.md:29` — Top-level guidance listing StableNet-unique files (project-wide convention)

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `cherry-pick` · `handler_istanbul.go` · `stablenet_genesis` · `upstream`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Inlining StableNet-specific branches into a geth-origin file 'because it's simpler' — creates merge conflicts on the next upstream pick.*
  > expected failure mode:
- **P2.** *Renaming or restructuring a geth file because the StableNet diff has grown — same effect.*
  > expected failure mode:
- **P3.** *Putting new feature logic in `core/blockchain.go` / `eth/handler.go` instead of in a sibling _istanbul.go file.*
  > expected failure mode:

**[Q5] Procedure steps — does each step still match current tooling/code paths?**

- **S1.** *Before editing a geth-origin file, check `.claude/docs/build-source-files.md` — is the file marked 'StableNet 고유'? If not, your StableNet-specific logic belongs in a NEW file.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S2.** *Place StableNet-unique logic in a sibling file named `*_istanbul.go` / `*_stablenet.go` / `tx_fee_delegation.go` etc.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S3.** *Restrict edits to geth-origin files to MINIMAL hook points (interface dispatch, registration).*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S4.** *When adding a new StableNet-unique file, update the table in `.claude/docs/build-source-files.md`.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.foundations.cherry_pick_principle \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.foundations.equal_power` · `[P0 · A14 · B4 · risk:high]`

**Title**: Equal-power validators — every validator has voting power = 1
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 1** (stake-weighted voting / slashing)

**Summary** (read first):

> go-stablenet is PoA with equal-weight validators: every validator's
> voting power is 1, with no stake field, no weight field, no slashing.
> The defaultValidator struct (consensus/wbft/validator/default.go:32)
> carries only Address + BLSPublicKey — no Power/Weight/Stake field
> exists anywhere in the validator set. Quorum is computed by COUNT
> (ceil(N − F), see A1.wbft_core.quorum_calc), not by stake sum.
> 
> Any change that introduces stake-weighted voting, weighted quorum,
> proposer selection biased by stake, or stake-conditioned rewards is
> a STABILITY-VIOLATING bug — even if the test suite passes — because
> it imports Ethereum/WEMIX validator economics that go-stablenet
> explicitly does NOT follow. This trap fires when an LLM extrapolates
> from generic PBFT / PoS literature without checking the validator
> type.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/validator/default.go:36` · `defaultValidator` — Validator struct — Address + BLSPublicKey only; no Power/Weight/Stake field
2. `consensus/wbft/validator/default.go:65` · `defaultSet` — Validator set struct — list + policy; no stake aggregation
3. `consensus/wbft/validator/default.go:230` · `defaultSet.QuorumSize` — Quorum computed by count via ceil(N − F), not by stake

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `BLSPublicKey` · `QuorumSize` · `ValidatorSet` · `defaultSet` · `defaultValidator`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Every active validator has voting power = 1.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:36 `defaultValidator`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *Quorum is computed as ceil(N − F) over counts, not over stake sums.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:230 `defaultSet.QuorumSize`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *No validator struct, set struct, or selection routine reads a Power/Weight/Stake field.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:36 `defaultValidator`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Adding stake-weighted voting or weighted proposer selection — there is no stake to weight.*
  > expected failure mode:
- **P2.** *Implementing slashing — there is no slashable stake; misbehavior is handled by governance removal.*
  > expected failure mode:
- **P3.** *Extrapolating from Ethereum PoS / WEMIX governance assumptions.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.foundations.equal_power \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.foundations.instant_finality_inert_reorg` · `[P0 · A14 · B5 · risk:high]`

**Title**: Instant BFT finality — geth reorg / Td code is inert (a trap)
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 2** (reorg / probabilistic-finality (forker, Td inert under WBFT))

**Summary** (read first):

> Under WBFT consensus a Commit-quorum block is FINAL the moment it is
> written; there is no probabilistic finality and no honest reorg.
> However, geth's fork-choice machinery is still present in the tree:
> ForkChoice (core/forkchoice.go) compares TotalDifficulty, BlockChain
> has a forker field (core/blockchain.go:260), and ReorgNeeded is
> called at ~6 call sites (core/blockchain.go:1143, :1459, :1604,
> :1819, :1983, …). Under WBFT, ReorgNeeded effectively never returns
> true on honest input, but the code paths exist and compile.
> 
> The trap: an LLM editing block-import, finalization, or
> consensus-glue code may "fix" or extend these reorg/Td paths on the
> assumption that they govern real behaviour. They do not — touching
> them invites correctness drift (e.g. accidentally accepting a
> competing chain on the wrong side of finality). Treat the forker / Td
> code as inert geth residue; do not extend it for StableNet logic. Any
> consensus-related fork-choice change belongs in consensus/wbft/, not
> in core/forkchoice.go.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/blockchain.go:265` · `BlockChain.forker` — forker field — present but inert under WBFT instant finality
2. `core/blockchain.go:1459` — ReorgNeeded call site in writeBlockAndSetHead path
3. `core/forkchoice.go:77` · `ForkChoice.ReorgNeeded` — TerminalTotalDifficulty / externTd comparison — Ethereum-PoW heritage

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `ForkChoice` · `ReorgNeeded` · `TerminalTotalDifficulty` · `TotalDifficulty` · `forker` · `writeBlockAndSetHead`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Once a WBFT-Committed block is written, it is final; there is no honest reorg path.*
  - [ ] ✅ enforced at `__________________` (suggested: core/blockchain.go:265 `BlockChain.forker`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *ForkChoice / TotalDifficulty code paths are inert geth residue, not StableNet truth.*
  - [ ] ✅ enforced at `__________________` (suggested: core/blockchain.go:265 `BlockChain.forker`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *Reorg-related changes that affect consensus belong in consensus/wbft/, not in core/forkchoice.go.*
  - [ ] ✅ enforced at `__________________` (suggested: core/blockchain.go:265 `BlockChain.forker`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Extending core/forkchoice.go to influence StableNet block selection.*
  > expected failure mode:
- **P2.** *Reading TotalDifficulty as a meaningful chain-quality metric.*
  > expected failure mode:
- **P3.** *Adding logic that depends on honest reorg ever happening (e.g. retrying tx inclusion on a different fork).*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.foundations.instant_finality_inert_reorg \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.foundations.wkrc_not_eth` · `[P0 · A14 · B4 · risk:high]`

**Title**: Native asset is WKRC (KRW stablecoin), not ETH
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 3** (ETH-denominated assumptions (WKRC, base-fee redistribution))

**Summary** (read first):

> The native coin on go-stablenet is **WKRC** — a KRW-pegged
> stablecoin — not ETH. Source of truth: params/config.go genesis
> alloc carries name="WKRC", symbol="WKRC", currency="KRW", decimals=18;
> core/genesis_mainnet.json:58 mirrors the same. Mint/burn is
> bank-backed via GovMinter (A4) and the NativeCoinManager precompile
> (A5), tied to off-chain deposit/withdrawal.
> 
> ETH-denominated assumptions baked into geth (`Ether=1e18`,
> `GasFloor`, gas-price units assumed to be ETH-wei) survive in the
> tree as residue but do NOT describe StableNet semantics — the unit
> is WKRC-wei, the asset is a KRW stablecoin, and supply changes are
> authorized real-asset events, not protocol issuance. An LLM that
> designs fee/reward/transfer logic against "ETH" assumptions will
> emit code that is silently wrong for regulatory reporting, fee
> accounting, and user-facing balances.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `params/config.go:100` — WKRC native-coin definition: name/symbol='WKRC', currency='KRW', decimals=18 in NativeCoinAdapter Params
2. `core/genesis_mainnet.json:58` — Mainnet genesis carries the same WKRC name/symbol pair

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `KRW` · `NativeCoinAdapter` · `NativeCoinManager` · `WKRC` · `decimals`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Native asset is WKRC (symbol/name='WKRC', currency='KRW', decimals=18).*
  - [ ] ✅ enforced at `__________________` (suggested: params/config.go:100)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *Native supply changes are bank-backed mint/burn through GovMinter + NativeCoinManager, not protocol issuance.*
  - [ ] ✅ enforced at `__________________` (suggested: params/config.go:100)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *Ethereum 'Ether'/'wei' identifiers in the geth tree are residue; the unit is WKRC-wei.*
  - [ ] ✅ enforced at `__________________` (suggested: params/config.go:100)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Writing fee/reward code that says 'ETH' in comments or identifiers — incorrect for accounting and audit.*
  > expected failure mode:
- **P2.** *Hardcoding ETH-style fee floors / tip defaults instead of consulting WKRC params.*
  > expected failure mode:
- **P3.** *Treating native supply as protocol-issued (e.g. block reward minting).*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.foundations.wkrc_not_eth \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.theory.3f1_intersection` · `[P0 · A14 · B4 · risk:high]`

**Title**: BFT 3f+1 quorum intersection — why ceil(N − F) is the safety floor
**Source of truth**: `paper` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 4** (quorum reimplementation (ceil(N−F) split-brain))

**Summary** (read first):

> Classical BFT (Castro–Liskov PBFT, QBFT) proves SAFETY under up to F
> byzantine nodes only if any two quorums intersect in at least one
> honest node. With N validators and F = floor((N−1)/3), the minimum
> quorum that satisfies the intersection lemma is Q = ceil(N − F).
> WBFT instantiates exactly this Q in defaultSet.QuorumSize
> (consensus/wbft/validator/default.go:226) using F() = (N−1)/3 as
> float64 (:222). Examples that surprise int-only refactors:
> N=5 → Q=4 (not 2f+1=3); N=6 → Q=5 (not 3); N=8 → Q=6 (not 5).
> 
> Implication for designs: any code path that "approves" something on
> fewer than Q distinct validators violates the intersection lemma and
> may finalize on an unsafe quorum (a safety violation — two
> conflicting blocks both reach quorum). This applies not only to the
> consensus state machine but to ANY validator-signature aggregation
> added later (e.g. a custom-precompile attestation, a system-contract
> vote count). Use valSet.QuorumSize(), do not hardcode 2f+1.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/validator/default.go:226` · `defaultSet.F` — F = (N−1)/3 as float64 — the byzantine bound
2. `consensus/wbft/validator/default.go:230` · `defaultSet.QuorumSize` — Q = ceil(N − F) — the intersection-safe quorum
3. `consensus/wbft/core/prepare.go:87` · `Core.handlePrepareMsg` — Threshold check using QuorumSize — the StatePrepared gate

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `F()` · `QuorumSize` · `ceil` · `intersection`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Any quorum claimed by protocol code must equal valSet.QuorumSize() — never hardcoded 2f+1 or N/2+1.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:230 `defaultSet.QuorumSize`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *F = floor((N−1)/3); Q = ceil(N − F). The two quantities together encode the safety bound.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:226 `defaultSet.F`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *Validator-signature aggregations added outside the consensus state machine MUST honor the same Q.*
  - [ ] ✅ enforced at `__________________`
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Using PBFT-textbook 2f+1 — diverges from ceil(N−F) for N not of form 3f+1; under-counts the safe quorum.*
  > expected failure mode:
- **P2.** *Using simple majority N/2+1 — far too low; trivially unsafe.*
  > expected failure mode:
- **P3.** *Treating F as int — loses the (N−1)/3 fractional part that ceil compensates for.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.theory.3f1_intersection \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.theory.equivocation` · `[P0 · A14 · B5 · risk:high]`

**Title**: Equivocation — byzantine validators may send conflicting messages
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 1** (stake-weighted voting / slashing)

**Summary** (read first):

> EQUIVOCATION is the byzantine behaviour where one validator emits
> two (or more) DIFFERENT messages for the same (Sequence, Round,
> MessageType) — e.g. two different Prepare votes for the same view.
> Honest BFT must tolerate this: a validator's "vote" is counted at
> most ONCE per view, and the second message either replaces the
> first deterministically or is dropped. WBFT inherits QBFT/PBFT's
> message-handling discipline in consensus/wbft/core/handler.go
> (signature verification + per-sender deduplication in the message
> set) and in the typed Set structures behind WBFTPrepares /
> WBFTCommits used by handlePrepareMsg (consensus/wbft/core/prepare.go
> :121) and the analogous commit path.
> 
> Implication: code that aggregates validator signatures (consensus
> itself, but also any later-added precompile vote / system-contract
> attestation / off-chain bridge proof) MUST deduplicate by sender
> before counting toward quorum. A naive `count := len(messages)`
> lets a single byzantine validator stuff the set and forge quorum.
> The flip side is liveness: when an equivocator is detected, WBFT
> does NOT slash (validators have no stake — see
> A14.foundations.equal_power); the protocol relies on the byzantine
> bound F to bound the damage, not on punishment.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/core/prepare.go:87` · `Core.handlePrepareMsg` — Prepare-count threshold check; per-sender dedup happens before this point in the Set
2. `consensus/wbft/core/handler.go:265` — Signature verification — equivocation detection precondition
3. `consensus/wbft/core/preprepare.go:114` · `Core.handlePreprepareMsg` — Justification check; per-sender dedup in JustificationPrepares set

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `WBFTCommits` · `WBFTPrepares` · `dedup` · `equivocation` · `handlePrepareMsg`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Vote counts toward quorum are per SENDER, not per MESSAGE — duplicates from the same sender count once.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/prepare.go:87 `Core.handlePrepareMsg`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *Equivocation is tolerated within the byzantine bound F; there is no slashing under WBFT (PoA with equal power = 1).*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/handler.go:265)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *Signature verification gates message acceptance; unsigned/forged messages are dropped before dedup.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/prepare.go:87 `Core.handlePrepareMsg`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Counting messages instead of distinct signers — single byzantine equivocator can stuff quorum.*
  > expected failure mode:
- **P2.** *Assuming a slashing mechanism exists — there is none under StableNet's PoA equal-power model.*
  > expected failure mode:
- **P3.** *Aggregating validator signatures off-chain (e.g. bridge proofs) without per-sender dedup — same vulnerability re-introduced.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.theory.equivocation \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.theory.flp_partial_synchrony` · `[P0 · A14 · B4 · risk:high]`

**Title**: FLP impossibility — WBFT trades termination for partial synchrony
**Source of truth**: `paper` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 4** (quorum reimplementation (ceil(N−F) split-brain))

**Summary** (read first):

> The FLP result (Fischer–Lynch–Paterson, 1985) proves no deterministic
> consensus algorithm can guarantee both SAFETY and LIVENESS in a
> purely asynchronous system with even one crash. Practical BFT
> algorithms (PBFT, QBFT, WBFT) escape FLP by assuming PARTIAL
> SYNCHRONY: there exists a Global Stabilization Time (GST) after which
> message delays are bounded by a known Δ. WBFT implements this
> assumption via the round-change timer (consensus/wbft/core/core.go:
> 110 `roundChangeTimer`, :409 `time.AfterFunc(timeout, ...)`); when a
> round times out before quorum is reached, the protocol advances to
> the next round with the same view-change machinery PBFT/QBFT use.
> 
> Implication: LIVENESS is NOT unconditional — it requires GST + a
> correct timeout. Setting RequestTimeout too low can cause perpetual
> round-changes (no round ever finishes before the timer fires);
> setting it too high stalls progress under transient asymmetry. The
> default 1000ms (see A3.timing.wbft_config_defaults) is the
> calibrated value; design changes that touch timer scheduling,
> RequestTimeout, or round-change backoff MUST reason about
> partial-synchrony implications, not just throughput.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/core/core.go:116` · `Core.roundChangeTimer` — Round-change timer field — instantiates partial-synchrony assumption
2. `consensus/wbft/core/core.go:409` — time.AfterFunc(timeout, ...) — the timer that fires the next round
3. `consensus/wbft/validator/default.go:230` · `defaultSet.QuorumSize` — Safety floor that holds even when liveness is suspended

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `RequestTimeout` · `roundChangeTimer` · `time.AfterFunc` · `timerMu`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Safety holds under asynchrony; liveness holds only after Global Stabilization Time + bounded Δ.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/validator/default.go:230 `defaultSet.QuorumSize`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *Round-change timeout is the implementation of the partial-synchrony assumption — its scheduling discipline is consensus-critical.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/core.go:116 `Core.roundChangeTimer`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *Lowering RequestTimeout below network round-trip + processing latency causes perpetual round-changes (no liveness).*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/core.go:116 `Core.roundChangeTimer`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Tuning RequestTimeout for benchmark throughput without modeling wide-area network latency.*
  > expected failure mode:
- **P2.** *Removing or gating the round-change timer in a 'happy path' optimization — safety still holds but liveness can stall indefinitely.*
  > expected failure mode:
- **P3.** *Conflating 'asynchronous' (no timing assumption) with 'partial synchrony' (eventual bounded delays) when reading or writing protocol logic.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.theory.flp_partial_synchrony \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A14.theory.justification_locking` · `[P0 · A14 · B4 · risk:high]`

**Title**: Justification + locking — why view changes preserve safety
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 4** (quorum reimplementation (ceil(N−F) split-brain))

**Summary** (read first):

> In PBFT/QBFT/WBFT, a PRE-PREPARE proposed in round r > 0 MUST carry
> a JUSTIFICATION proving it is consistent with what previous rounds
> may have committed: a quorum of ROUND-CHANGE messages plus, if any
> validator was already "locked" on a value, the PREPARE certificate
> for that locked value. WBFT enforces this in preprepare.go: a new
> proposal attaches `JustificationRoundChanges` and `Justification
> Prepares` (preprepare.go:69, :79); the receiver validates them via
> `isJustified(...)` (preprepare.go:135) before accepting the
> preprepare. The locking discipline says: once you Prepare on a
> value, you stay locked until a higher round shows a quorum-justified
> different value — this is what guarantees that a view change cannot
> silently overwrite a previously Committed block.
> 
> Implication: any change to the preprepare validator, round-change
> payload format, or the message handler that handles justification
> payloads (consensus/wbft/core/handler.go:265–292) directly affects
> the SAFETY proof. Skipping `isJustified` for a "fast path", or
> loosening which payloads count as justification, is a SAFETY
> violation — two conflicting blocks can both reach quorum. This is
> the single most subtle place in WBFT and the most common
> hand-rolled-PBFT bug.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/core/preprepare.go:114` · `Core.handlePreprepareMsg` — isJustified() — the safety-critical justification check
2. `consensus/wbft/core/preprepare.go:69` — Attach ROUND-CHANGE justification to outgoing PRE-PREPARE (round > 0)
3. `consensus/wbft/core/preprepare.go:79` — Attach PREPARE justification (locking certificate)
4. `consensus/wbft/core/handler.go:265` — Verifies signature of message and of all justification payloads

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `JustificationPrepares` · `JustificationRoundChanges` · `SignedRoundChangePayload` · `isJustified` · `locked`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *PRE-PREPARE for round r > 0 carries a JUSTIFICATION; receivers verify it before accepting the proposal.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/preprepare.go:69)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *A validator that has Prepared on value v in round r stays 'locked' on v until a higher round produces a quorum-justified different value.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/preprepare.go:69)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *Justification payload format (RoundChange + Prepare signatures) is part of the consensus contract — changes affect cross-version interop.*
  - [ ] ✅ enforced at `__________________` (suggested: consensus/wbft/core/preprepare.go:69)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Skipping `isJustified` for a 'happy path' on round 0 only — and then enabling the skip path for round > 0 by accident.*
  > expected failure mode:
- **P2.** *Loosening signature checks on piggybacked justification payloads (handler.go:265) — accepts unjustified pre-prepares.*
  > expected failure mode:
- **P3.** *Changing JustificationPrepares aggregation rule without re-proving safety — same surface, different semantics, silent fork.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A14.theory.justification_locking \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A2.block_encoding.filtered_header_hash` · `[P0 · A2 · B3 · risk:low]`

**Title**: Block hash computation requires WBFTFilteredHeader first
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 5** (feepayer sigHash payload)

**Summary** (read first):

> WBFT block hashing has a non-obvious requirement: before computing a
> block hash you must strip the round number and the PreparedSeal /
> CommittedSeal from the header's WBFTExtra, because those fields are
> filled in by the consensus layer after the header is created and
> therefore differ between proposer and recipient. WBFTFilteredHeader
> returns a copy of the header with Round set to 0 and Seal fields
> cleared; WBFTFilteredHeaderWithRound does the same but preserves a
> caller-supplied round. Hashing the raw header instead of the filtered
> one produces a hash nobody else can reproduce, breaking signature
> verification and block lookup.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/types/istanbul.go:263` · `WBFTFilteredHeader` — thin wrapper that calls WBFTFilteredHeaderWithRound with round = 0
2. `core/types/istanbul.go:269` · `WBFTFilteredHeaderWithRound` — actual filtering implementation — re-encodes WBFTExtra with round/seals cleared
3. `core/types/istanbul.go:251` · `ExtractWBFTExtra` — called inside the filter to decode-then-re-encode the Extra

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `CommittedSeal` · `ExtractWBFTExtra` · `PreparedSeal` · `Round` · `WBFTFilteredHeaderWithRound` · `WBFTFilteredHeader`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Block hashes are computed over WBFTFilteredHeader, not the raw header.*
  - [ ] ✅ enforced at `__________________`
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *WBFTFilteredHeader zeros Round and removes PreparedSeal / CommittedSeal, but leaves PrevRound / PrevPreparedSeal / PrevCommittedSeal untouched (those are part of the canonical block).*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/istanbul.go:263 `WBFTFilteredHeader`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *IstanbulExtraVanity (32) and IstanbulExtraSeal (96) are fixed — changing them invalidates every existing block.*
  - [ ] ✅ enforced at `__________________`
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Hashing the raw header produces a hash that disagrees with every other node and breaks signature verification.*
  > expected failure mode:
- **P2.** *Forgetting to strip Round means the proposer's hash differs from each follower's once they observe their own round value.*
  > expected failure mode:
- **P3.** *Confusing PrevRound (kept) with Round (stripped); both live in WBFTExtra but play different roles in hashing.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A2.block_encoding.filtered_header_hash \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A2.block_encoding.wbft_extra_struct` · `[P0 · A2 · B2 · risk:low]`

**Title**: WBFTExtra struct layout and contained types
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 5** (feepayer sigHash payload)

**Summary** (read first):

> WBFTExtra is the RLP-encoded payload that lives inside every WBFT
> block's Header.Extra. It carries vanity bytes, randao reveal, the
> previous and current round numbers, the previous and current
> PreparedSeal / CommittedSeal aggregated BLS signatures, the
> governance-voted GasTip for this block, and (on epoch-boundary
> blocks only) the EpochInfo that pins the next epoch's validator
> set. AggregatedSeal pairs a SealerSet bit-packed bitmap with a 96-byte
> BLS aggregate signature; the bit index in SealerSet corresponds to
> the validator index in the active ValidatorSet. EpochInfo lists
> Candidate {Addr, Diligence} entries plus a Validators slice of
> *indices* (not addresses) into Candidates, and a parallel
> BLSPublicKeys slice.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/types/istanbul.go:87` · `WBFTExtra` — main struct definition
2. `core/types/istanbul.go:58` · `WBFTAggregatedSeal` — PreparedSeal / CommittedSeal payload (BLS aggregate)
3. `core/types/istanbul.go:296` · `SealerSet` — bit-packed bitmap; SetSealer / IsSealer / GetSealers helpers
4. `core/types/istanbul.go:105` · `EpochInfo` — next-epoch validator manifest
5. `core/types/istanbul.go:100` · `Candidate` — {Addr, Diligence} entry referenced by EpochInfo.Validators indices
6. `core/types/istanbul.go:34` · `IstanbulExtraVanity` — 32-byte fixed vanity prefix
7. `core/types/istanbul.go:35` · `IstanbulExtraSeal` — 96-byte fixed BLS signature length

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `Candidate` · `CommittedSeal` · `EpochInfo` · `ExtractWBFTExtra` · `GasTip` · `GetSealers` · `IsSealer` · `IstanbulExtraSeal` · `IstanbulExtraVanity` · `PreparedSeal` · `PrevCommittedSeal` · `PrevPreparedSeal` · `PrevRound` · `RandaoReveal` · `SealerSet` · `SetSealer` · `VanityData` · `WBFTAggregatedSeal` · `WBFTExtra`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q6] Constants — do the values match what's in source today?**

- **IstanbulExtraVanity** = `32` bytes — cite: `core/types/istanbul.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed
- **IstanbulExtraSeal** = `96` bytes — cite: `core/types/istanbul.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed
- **DefaultDiligence** = `1900000` ratio (1e-6 units; 95% of max 2_000_000) — cite: `core/types/istanbul.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A2.block_encoding.wbft_extra_struct \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A3.validator_set.epoch_transition` · `[P0 · A3 · B3 · risk:low]`

**Title**: Epoch transition: how the next epoch's validator set is selected and pinned
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 1** (stake-weighted voting / slashing)

**Summary** (read first):

> go-stablenet's validator set is fixed for the duration of an epoch
> (10 blocks on Mainnet, 140 on Testnet). On the last block of each
> epoch, the engine calls buildEpochInfo: it queries GovValidator
> (system contract 0x1001) for the new candidate list and validator
> indices, then writes an EpochInfo into that block's
> Header.Extra.WBFTExtra.EpochInfo. From the next block onward,
> GetValidators returns the new set; intra-epoch blocks omit
> EpochInfo. Backend.GetValidatorsForVerifying provides the previous
> epoch's set in parallel so cross-epoch block verification can match
> signers against the correct validator set.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/backend/engine.go:130` · `Backend.GetValidators` — primary validator lookup at a given header
2. `consensus/wbft/backend/engine.go:426` · `Backend.GetValidatorsForVerifying` — returns (current, previous) sets for cross-epoch boundary verification
3. `core/types/istanbul.go:105` · `EpochInfo` — the manifest written into the last block of each epoch
4. `params/config_wbft.go` · `WBFTConfig.Epoch` — epoch length parameter (Mainnet 10, Testnet 140)
5. `systemcontracts/gov_validator.go` — GovValidator contract (0x1001) — source of truth for next-epoch candidates

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `BLSPublicKeys` · `Candidate` · `EpochInfo` · `GetValidatorsForVerifying` · `GetValidators` · `GovValidator` · `IsEpochBlockNumber` · `Validators` · `WBFTConfig` · `buildEpochInfo`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *EpochInfo is non-nil only on the last block of an epoch. Any other block carrying EpochInfo is a protocol violation.*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/istanbul.go:105 `EpochInfo`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *EpochInfo.Validators contains indices into EpochInfo.Candidates — never addresses. Direct address access requires looking up Candidates[Validators[i]].Addr.*
  - [ ] ✅ enforced at `__________________` (suggested: systemcontracts/gov_validator.go)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *BLSPublicKeys has the same length as Validators.*
  - [ ] ✅ enforced at `__________________` (suggested: params/config_wbft.go `WBFTConfig.Epoch`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I4.** *The validator set for block N is determined by the most recent block <= N that carried an EpochInfo; the genesis block seeds the first epoch via WBFTInit.*
  - [ ] ✅ enforced at `__________________` (suggested: core/types/istanbul.go:105 `EpochInfo`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Writing EpochInfo on a non-epoch-boundary block breaks header validation on every other node — symptoms are widespread block rejections from peers.*
  > expected failure mode:
- **P2.** *Treating EpochInfo.Validators as addresses instead of indices retrieves wrong validators and breaks signature verification.*
  > expected failure mode:
- **P3.** *Forgetting to query GetValidatorsForVerifying on a boundary block (using only GetValidators) misses the previous-epoch set needed to verify the seals on that boundary block itself.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A3.validator_set.epoch_transition \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A4.system_contracts.gov_minter` · `[P1 · A4 · B1 · risk:low]`

**Title**: GovMinter (0x1003): native coin minting authority with v1/v2 versions
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 3** (ETH-denominated assumptions (WKRC, base-fee redistribution))

**Summary** (read first):

> GovMinter is the system contract at address 0x1003 that holds the
> mint authority for native coin. The MasterMinter (0x1002) approves
> mint proposals which GovMinter executes against NativeCoinAdapter
> (0x1000). Two versioned implementations exist: v1 (initial) and v2
> (current). ChainConfig.Anzeon.SystemContracts.GovMinter selects the
> version per chain. Storage tracks the fiat-token reference, mint
> proposals, per-account burn balances, refundable balances, and
> emergency-pause state.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `systemcontracts/gov_minter.go:87` · `initializeMinter` — Go initializer writes fiatToken from genesis param into contract storage at deploy time
2. `systemcontracts/gov_minter.go:122` · `GetMintProposalAmount` — reads the reserved mint amount for a given proposalId from contract storage
3. `systemcontracts/gov_minter.go:130` · `GetBurnBalance` — reads per-address burn balance from contract storage
4. `systemcontracts/contracts.go:44` · `GovMinterContractV1` — go:embed of compiled v1 bytecode at artifacts/v1/GovMinter
5. `systemcontracts/contracts.go:53` · `GovMinterContractV2` — go:embed of compiled v2 bytecode at artifacts/v2/GovMinter
6. `systemcontracts/solidity/v1/GovMinter.sol` — Solidity source for v1 (regenerate artifacts via systemcontracts/compile)
7. `systemcontracts/solidity/v2/GovMinter.sol` — Solidity source for v2

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `CONTRACT_GOV_MINTER` · `GetBurnBalance` · `GetMintProposalAmount` · `GetRefundableBalance` · `GovMinterContractV1` · `GovMinterContractV2` · `GovMinter` · `initializeMinter`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A4.system_contracts.gov_minter \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A6.fee_delegation.signing_model` · `[P0 · A6 · B5 · risk:medium]`

**Title**: Fee delegation signing model: setSignatureValues writes the FeePayer, not the Sender
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 5** (feepayer sigHash payload)

**Summary** (read first):

> FeeDelegateDynamicFeeTx (tx type 0x16) is a double-signed transaction:
> the Sender signs the inner DynamicFeeTx, then the FeePayer signs the
> whole envelope. The method names on the outer type are easy to
> misread. setSignatureValues writes the FeePayer's V/R/S
> (tx.FV/FR/FS); rawSignatureValues returns the Sender's signature by
> delegating to SenderTx.rawSignatureValues; rawFeePayerSignatureValues
> returns the FeePayer's signature. Confusing these breaks both
> signature verification paths and trips the blacklist guard for the
> FeePayer in TransitionDb.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/types/tx_fee_delegation.go:147` · `FeeDelegateDynamicFeeTx.setSignatureValues` — writes V/R/S into tx.FV/FR/FS — FeePayer signature target
2. `core/types/tx_fee_delegation.go:143` · `FeeDelegateDynamicFeeTx.rawSignatureValues` — returns SenderTx.rawSignatureValues — Sender signature target
3. `core/types/tx_fee_delegation.go:128` · `FeeDelegateDynamicFeeTx.rawFeePayerSignatureValues` — returns FeePayer V/R/S directly
4. `core/state_transition.go` — FeePayer blacklist verification point (stablenet-features.md §10 verification ③)

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `FR` · `FS` · `FV` · `FeeDelegateDynamicFeeTxType` · `FeeDelegateDynamicFeeTx` · `FeePayer` · `rawFeePayerSignatureValues` · `rawSignatureValues` · `setSignatureValues`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Adding logic to setSignatureValues thinking it covers the Sender signature — Sender's V/R/S lives on the embedded SenderTx and is set by the embedded DynamicFeeTx's setSignatureValues, not the wrapper.*
  > expected failure mode:
- **P2.** *Forgetting to verify both Sender and FeePayer signatures during transaction validation — verifying only Sender allows a forged FeePayer to be inserted.*
  > expected failure mode:
- **P3.** *Computing gas pricing from FeePayer's GasFeeCap/GasTipCap — there is no such field. The FeePayer only signs; gas price comes from SenderTx (the inner DynamicFeeTx).*
  > expected failure mode:
- **P4.** *Missing the FeePayer blacklist check in TransitionDb (verification ③ in stablenet-features.md §10) — a blacklisted FeePayer can otherwise still pay for a non-blacklisted Sender's transaction.*
  > expected failure mode:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A6.fee_delegation.signing_model \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A7.hardfork.add_new_fork_procedure` · `[P0 · A7 · B6 · risk:low]`

**Title**: Procedure: adding a new hardfork to go-stablenet
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 2** (reorg / probabilistic-finality (forker, Td inert under WBFT))

**Summary** (read first):

> Adding a hardfork requires changes in seven distinct sites:
> ChainConfig field, isFork check method, Rules struct + Rules() build,
> runtime branch in the relevant subsystem, optional NativeManager
> switch update, optional system contract version pinning via
> CollectUpgrades, and updates to SetConfigFromChainConfig when WBFT
> parameters change. The block number must be >= the previous fork's
> block number; switch-case ordering must place the newest fork on top
> because the first matching case wins.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `params/config.go` · `ChainConfig` — fork block field and IsXxx methods live here
2. `params/config.go` · `ChainConfig.CollectUpgrades` — ascending-block-order upgrade registry
3. `params/config.go` · `ChainConfig.CheckConfigForkOrder` — enforces that each fork's block is >= the previous
4. `eth/ethconfig/config.go` · `SetConfigFromChainConfig` — binds ChainConfig hardforks into the WBFT runtime config
5. `core/vm/native_manager.go` · `ActiveNativeManagers` — newest-fork-on-top switch ordering rule

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `ActiveNativeManagers` · `AnzeonConfig` · `ChainConfig` · `CheckConfigForkOrder` · `CollectUpgrades` · `Rules` · `SetConfigFromChainConfig` · `SystemContracts` · `Transition` · `Upgrade` · `isBlockForked`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q3] Invariants — does each one actually hold, and which code line enforces it?**

- **I1.** *Each new fork's block number must be >= the previous fork's block number (enforced by CheckConfigForkOrder).*
  - [ ] ✅ enforced at `__________________` (suggested: params/config.go `ChainConfig.CheckConfigForkOrder`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I2.** *CollectUpgrades must return entries in ascending block order; the WBFT runtime breaks on the first block > target, so out-of-order entries silently lose later upgrades.*
  - [ ] ✅ enforced at `__________________` (suggested: params/config.go `ChainConfig.CollectUpgrades`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced
- **I3.** *In ActiveNativeManagers, the newest fork case must appear first because Go switch-case picks the first matching branch.*
  - [ ] ✅ enforced at `__________________` (suggested: core/vm/native_manager.go `ActiveNativeManagers`)
  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)
  - [ ] ❌ not enforced

**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**

- **P1.** *Adding the new ChainConfig field but forgetting Rules + Rules() update — runtime branches never fire because rules.IsMyNewFork is always false.*
  > expected failure mode:
- **P2.** *Skipping CollectUpgrades registration — the fork's system contracts never deploy at the activation block.*
  > expected failure mode:
- **P3.** *Skipping SetConfigFromChainConfig update — WBFT parameter changes never reach the consensus engine.*
  > expected failure mode:
- **P4.** *Adding the new switch case at the bottom of ActiveNativeManagers — an older fork above matches first and the new addresses are ignored.*
  > expected failure mode:

**[Q5] Procedure steps — does each step still match current tooling/code paths?**

- **S1.** *Step 1 — params/config.go: add a new field to ChainConfig (e.g. MyNewForkBlock *big.Int).*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S2.** *Step 2 — params/config.go: add an activation check method (IsMyNewFork) that delegates to isBlockForked.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S3.** *Step 3 — params/config.go: add IsMyNewFork to the Rules struct and to the Rules() builder so runtime sites can branch on it.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S4.** *Step 4 — runtime sites: add 'if rules.IsMyNewFork { ... } else { ... }' branches wherever behaviour diverges.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S5.** *Step 5 — core/vm/native_manager.go: update ActiveNativeManagers switch if the fork rotates native manager addresses. Put the newest fork's case at the top.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S6.** *Step 6 — params/config.go: define MyNewFork *AnzeonConfig with SystemContracts overrides, then register the upgrade in CollectUpgrades (append a Upgrade{Block: MyNewForkBlock, SystemContracts: MyNewFork.SystemContracts}).*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S7.** *Step 7 — eth/ethconfig/config.go: extend SetConfigFromChainConfig to append a Transition{Block: MyNewForkBlock, WBFTConfig: ...} when the fork changes WBFT parameters; record the block in hfTransitionBlocks to detect duplicates.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S8.** *Step 8 (verification) — run 'go test ./params/... ./consensus/wbft/... ./core/...' and check that CheckConfigForkOrder accepts the new fork's block ordering.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A7.hardfork.add_new_fork_procedure \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A8.genesis.bootstrap_architecture` · `[P0 · A8 · B1 · risk:low]`

**Title**: Genesis bootstrap architecture: from JSON to first block on disk
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 1** (stake-weighted voting / slashing)

**Summary** (read first):

> Node startup with an empty DB takes a Genesis struct (parsed from
> embedded JSON for Mainnet/Testnet) through three sequential stages:
> (1) validateAnzeonGenesisConfig enforces that the Anzeon section is
> well-formed; (2) initializeAnzeonGenesis builds the initial block
> Extra (vanity + initial validator set + initial BLS keys) and calls
> InjectContracts which deploys every system contract listed in
> config.Anzeon.SystemContracts plus any block-0 hardfork overlays
> collected from CollectUpgrades; (3) genesis.ToBlock writes the
> resulting Alloc to the state DB, computes the state root, and
> embeds it in the block header; finally genesis.Commit flushes the
> trie to disk. The genesis_generator CLI uses the same path with
> user-supplied parameters and serialises the result to JSON for
> bootstrapping a new network.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/genesis.go:282` · `SetupGenesisBlockWithOverride` — top-level entry point invoked by node init
2. `core/genesis.go:231` · `validateAnzeonGenesisConfig` — stage 1 — schema/shape validation
3. `core/genesis.go:242` · `initializeAnzeonGenesis` — stage 2 — initial Extra + InjectContracts
4. `core/genesis.go:510` · `Genesis.ToBlock` — stage 3a — Alloc → stateDB, computes state root
5. `core/genesis.go:575` · `Genesis.Commit` — stage 3b — flush state trie to disk
6. `core/genesis.go:735` · `InjectContracts` — deploys the 5 system contracts + applies any block-0 hardfork overlays
7. `core/genesis.go:617` · `DefaultStableNetMainnetGenesisBlock` — embedded Mainnet/Testnet genesis builders (also DefaultStableNetTestnetGenesisBlock); consumed by cmd/utils/flags.go to bootstrap a node with the canonical chain
8. `cmd/genesis_generator/` — CLI that generates a fresh network's genesis JSON

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `CollectUpgrades` · `Commit` · `CreateInitialExtraData` · `InjectContracts` · `SetupGenesisBlockWithOverride` · `ToBlock` · `applyUpgradeOverlay` · `genesis_generator` · `initializeAnzeonGenesis` · `validateAnzeonGenesisConfig`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A8.genesis.bootstrap_architecture \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A8.genesis.json_authoring_checklist` · `[P1 · A8 · B6 · risk:low]`

**Title**: Genesis JSON authoring checklist for stablenet chains
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 1** (stake-weighted voting / slashing)

**Summary** (read first):

> validateAnzeonGenesisConfig is the gate every stablenet genesis JSON
> passes through before initializeAnzeonGenesis materialises the
> block-zero state. The authoring checklist captures every section the
> validator requires: ChainConfig with the Anzeon hardfork block, WBFT
> consensus parameters, the Init section (validators plus matching BLS
> keys), the SystemContracts section selecting versions for each of
> the five governance contracts, and per-account Extra bits that must
> satisfy ValidateExtra. cmd/genesis_generator provides an interactive
> tool for filling these in.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `core/genesis.go:231` · `validateAnzeonGenesisConfig` — the gate; calls ChainConfig.Anzeon.CheckValidity on every required section
2. `core/genesis.go:242` · `initializeAnzeonGenesis` — after validate succeeds, sets up WBFT extra data and calls InjectContracts
3. `cmd/genesis_generator/genesis_generator.go` — interactive CLI that produces a valid genesis JSON skeleton

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `CheckValidity` · `InjectContracts` · `ValidateExtra` · `genesis_generator` · `initializeAnzeonGenesis` · `validateAnzeonGenesisConfig`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q5] Procedure steps — does each step still match current tooling/code paths?**

- **S1.** *Set ChainConfig.Anzeon (block number, hardfork transitions). CheckValidity rejects a missing Anzeon section, so everything below presumes it is present.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S2.** *Fill ChainConfig.Anzeon.WBFT with RequestTimeoutSeconds, BlockPeriodSeconds, and EpochLength (see A3.timing.wbft_config_defaults for canonical defaults).*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S3.** *Fill ChainConfig.Anzeon.Init.Validators with every initial validator address and matching BLS public key in BLSPublicKeys; the two lists must align by index.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S4.** *Fill ChainConfig.Anzeon.SystemContracts: GovValidator, NativeCoinAdapter, GovMasterMinter, GovMinter, GovCouncil. Each entry pins the version (V1 or V2). See A4 entries for canonical addresses and version semantics.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S5.** *For every account in Alloc that carries authorisation bits, set Extra so it passes ValidateExtra (see A5.account_extra.bit_layout).*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete
- **S6.** *Run cmd/genesis_generator to scaffold the JSON, or hand-edit and rely on validateAnzeonGenesisConfig at first start to catch missing fields.*
  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A8.genesis.json_authoring_checklist \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A9.istanbul_p2p.protocol_architecture` · `[P1 · A9 · B1 · risk:low]`

**Title**: istanbul/100 subprotocol architecture: piggy-backed on eth peers
**Source of truth**: `code+docs` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 3** (ETH-denominated assumptions (WKRC, base-fee redistribution))

**Summary** (read first):

> WBFT consensus messages travel over a dedicated P2P subprotocol
> named istanbul, version 100, registered alongside the standard
> eth protocol. The 22-message subprotocol does not create its own
> peer set — every istanbul peer is an existing eth Peer whose
> EthPeerRegistered channel signals when it is ready to carry
> consensus traffic. When an istanbul message arrives, the handler
> dispatches it into the consensus engine's HandleMsg, which routes
> it to Core.handleEvents. Messages exceeding protocolMaxMsgSize
> are rejected before any consensus parsing. wbft.ErrStoppedEngine
> during peer-receive while the eth downloader is synchronising is
> expected and downgraded to debug-level logging — the engine starts
> consuming messages only after sync completes.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `eth/quorum_protocol.go:39` · `Istanbul100` — protocol version constant
2. `eth/quorum_protocol.go:41` · `quorumConsensusProtocolName` — protocol name literal — "istanbul"
3. `eth/quorum_protocol.go:54` · `quorumConsensusProtocolLengths` — 22 messages on Istanbul100
4. `eth/handler_istanbul.go:82` · `EthPeerRegistered` — eth-peer-ready signal — istanbul reuses eth peers, never creates its own
5. `eth/handler_istanbul.go:138` · `protocolMaxMsgSize` — message size gate — rejection happens before consensus parsing
6. `eth/handler_istanbul.go:120` · `wbft.ErrStoppedEngine` — sync-time error path — debug log only, not warn/error
7. `consensus/wbft/backend/handler.go` — HandleMsg implementation that bridges P2P to consensus Core

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `ErrStoppedEngine` · `EthPeerRegistered` · `HandleMsg` · `Istanbul100` · `protocolMaxMsgSize` · `quorumConsensusProtocolLengths` · `quorumConsensusProtocolName` · `quorumConsensusProtocolVersions`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q6] Constants — do the values match what's in source today?**

- **Istanbul100** = `100` protocol_version — cite: `eth/quorum_protocol.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed
- **quorumConsensusProtocolName** = `istanbul`  — cite: `eth/quorum_protocol.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed
- **Istanbul100 message count** = `22` messages — cite: `eth/quorum_protocol.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A9.istanbul_p2p.protocol_architecture \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

### `A9.istanbul_rpc.api_reference` · `[P1 · A9 · B7 · risk:low]`

**Title**: WBFT custom RPC API (istanbul namespace) — method reference
**Source of truth**: `code` · **Status**: `needs_verification`
**Maps to**: **07 §9 item 1** (stake-weighted voting / slashing)

**Summary** (read first):

> The WBFT consensus engine exposes a JSON-RPC API under the istanbul
> namespace via consensus/wbft/backend/api.go. The methods cover
> node identity (NodeAddress, IsValidator), block-attestation lookup
> (GetCommitSignersFromBlock / ByHash), validator-set introspection
> (GetValidators / AtHash), aggregate activity stats over a range
> (Status), and raw WBFTExtra inspection (GetWbftExtraInfo). These
> are the operational entry points for monitoring tools and any
> external client that needs to confirm who signed which block.

**Anchors** (mechanical ✅ — already file+line+symbol consistent):

1. `consensus/wbft/backend/api.go:84` · `API.NodeAddress`
2. `consensus/wbft/backend/api.go:90` · `API.GetCommitSignersFromBlock`
3. `consensus/wbft/backend/api.go:107` · `API.GetCommitSignersFromBlockByHash`
4. `consensus/wbft/backend/api.go:136` · `API.GetValidators`
5. `consensus/wbft/backend/api.go:156` · `API.GetValidatorsAtHash`
6. `consensus/wbft/backend/api.go:168` · `API.Status` — validator activity statistics over a range
7. `consensus/wbft/backend/api.go` · `BlockSigners` — return type for GetCommitSignersFromBlock
8. `consensus/wbft/backend/api.go:76` · `Status` — return type for Status method

---

#### Substantive review — STATUS_LIFECYCLE §verification-checklist

**[Q1] Is the `summary` factually correct against current code?**

- [ ] ✅ correct
- [ ] ⚠️ partially (note what to revise)
- [ ] ❌ wrong (explain)

> reviewer note:

**[Q2] Do the `code_keywords` match identifiers actually used in source?**

Keywords: `AuthorCounts` · `BlockSigners` · `GetCommitSignersFromBlockByHash` · `GetCommitSignersFromBlock` · `GetValidatorsAtHash` · `GetValidators` · `GetWbftExtraInfo` · `IsValidator` · `NodeAddress` · `RoundStats` · `SealerActivity` · `Status` · `istanbul`

- [ ] ✅ all match
- [ ] ⚠️ partial (list which are stale)
- [ ] ❌ all stale

> reviewer note:

**[Q6] Constants — do the values match what's in source today?**

- **rpc_namespace** = `istanbul`  — cite: `consensus/wbft/backend/api.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed
- **NodeAddress_params** = `0` parameters — cite: `consensus/wbft/backend/api.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed
- **GetValidators_params** = `*rpc.BlockNumber`  — cite: `consensus/wbft/backend/api.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed
- **Status_params** = `*rpc.BlockNumber x2 (start, end)`  — cite: `consensus/wbft/backend/api.go`
  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed

**Decision**:

- [ ] **APPROVE** → run:
  ```bash
  go run ./cmd/cks-entry-verify \
      -project docs/domain-knowledge/projects/go-stablenet \
      -entry   A9.istanbul_rpc.api_reference \
      -by      <handle>
  ```
- [ ] **REVISE** → write notes above; entry stays `needs_verification`.
- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.

---

## Footer — post-session sync (runs once after all APPROVE entries are promoted)

```bash
# Re-emit policy (ckg governs) + glossary (ckv vocab) for downstream consumers
./bin/cks-domain-sync   -entries docs/domain-knowledge/projects/go-stablenet
./bin/cks-glossary-gen  -project docs/domain-knowledge/projects/go-stablenet -status verified

# Refresh ckv (channel ② docs) + ckg (governance edges) in lockstep.
# Operator's MCP client: cks.ops.index { mode: "full" }
```

## Notes on generator behaviour

- The `Maps to:` line uses a token-overlap heuristic against the 07 §9 catalog. If the suggested item is wrong, the reviewer should correct it in-line — the worksheet is treated as a working document, not as authoritative metadata.
- The `suggested: …` hint on each invariant is the anchor whose `reason` field shares the most words with the invariant text. A hint is shown only when at least one anchor scores > 0; otherwise the slot is left blank.
- B6 entries (`procedure_steps`) and B7 entries (`constants`) get extra question blocks (Q5/Q6); other knowledge types omit them.
