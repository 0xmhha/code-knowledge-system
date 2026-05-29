# Knowledge Types (B1~B7)

> Knowledge types classify *what kind of question an entry answers*.
> They are domain-agnostic: every blockchain, every codebase, every
> subsystem has the same seven shapes of question. The shape changes how
> the entry is written and how retrieval should route to it.

## B1 — Architecture

**Answers**: "How is this assembled? Who talks to whom?"

**Shape**: Block diagram in prose, packages/modules and their
responsibilities, data flow between them.

**Examples**:
- WBFT consensus flow: Core ⇄ Backend ⇄ Engine ⇄ EVM ⇄ EthHandler
- System contract upgrade path: ChainConfig → CollectUpgrades →
  SetConfigFromChainConfig → wbftCfg.SystemContractUpgrades → processFinalize
- P2P stack: eth/Peer ⇄ istanbul/100 ⇄ HandleMsg → consensus.Engine

**Write style**: Sequenced, name actors and verbs ("Backend calls
Verify before Commit"). Avoid implementation detail; link to B3 entries
for the algorithm.

## B2 — Data Structure

**Answers**: "What does this struct hold? What does each field mean?"

**Shape**: Struct definition with per-field semantics, slot/byte/bit
layout for storage, enum values and their meaning.

**Examples**:
- WBFTExtra fields and their RLP order
- StateAccount.Extra bit allocation (bit 63 = Blacklisted, bit 62 = Authorized)
- SystemContract fields (Address, Version, Params)
- Solidity storage slot map for a system contract

**Write style**: Field-by-field. Pull the actual Go/Solidity definition;
do not paraphrase types.

## B3 — Algorithm / Flow

**Answers**: "How does this operation execute step by step?"

**Shape**: Numbered steps, state transitions, branching conditions.

**Examples**:
- Quorum calculation: N → F = (N-1)/3 as float64 → QuorumSize = ceil(N-F)
- Blacklist check in TransitionDb (4 verification points)
- applyUpgradeOverlay code-vs-state handling at block 0
- BLS aggregated seal construction during Commit

**Write style**: Numbered. Explicit branch points. Each step names the
file/symbol it lives in.

## B4 — Invariant / Constraint

**Answers**: "What must always be true? What ordering/range must hold?"

**Shape**: List of plain assertions, each phrased so a reader can verify
it against code or by reasoning.

**Examples**:
- `F()` returns float64, not int — integer cast loses precision
- EpochInfo is non-nil only on the last block of an epoch
- `CollectUpgrades` must return entries in ascending block order
- Bitmask flags must not overlap with bits 62, 63 (Authorized, Blacklisted)

**Write style**: One invariant per bullet. Begin with the subject and
the must-hold verb. Reference the code location that enforces it.

## B5 — Pitfall / Anti-pattern

**Answers**: "What's the common mistake here? How does it go wrong?"

**Shape**: For each pitfall: the wrong action, what happens
(observable symptom), the right alternative.

**Examples**:
- Forgetting to update `GetSystemContracts` merge lambda → runtime
  upgrade silently skipped
- Calling `WBFTFilteredHeader` after hash computation → invalid hash
- Adding new SystemContracts field without genesis JSON guard → nil
  deref at node startup

**Write style**: Each pitfall is its own bullet. Always include symptom
so retrieval matches the user's error message.

## B6 — Procedure / Checklist

**Answers**: "What's the step-by-step process for doing X?"

**Shape**: Ordered list of imperative steps. Each step names the file
to edit and what to add. End with a verification step.

**Examples**:
- Adding a new hardfork (7 steps in CLAUDE_DEV_GUIDE §9)
- Adding a new system contract (9 steps in review-test-result §2)
- Upgrading an existing system contract version (8 steps)
- Adding a new transaction type with blacklist coverage (4 points)

**Write style**: Imperative voice ("Add X to params/config.go"). Include
post-conditions ("→ artifacts/v3/MyContract generated"). Reference the
existing canonical doc when one exists.

## B7 — Reference (Constants / Addresses / Parameters)

**Answers**: "What's the exact value of X?"

**Shape**: Table or list of `{name, value, unit, source_file}` tuples.
Pure look-up.

**Examples**:
- System contract addresses (0x1000 = NativeCoinAdapter, 0x1001 = GovValidator, …)
- WBFT message codes (0x12 = Preprepare, 0x13 = Prepare, …)
- Gas constants (UpdateBalanceGas = 4500, CallNewAccountGas = 25000)
- WBFT timing defaults (RequestTimeoutSeconds = 2, BlockPeriodSeconds = 1)

**Write style**: Compact. No prose. Use the `constants:` field on the
entry instead of free text so values can be machine-extracted.

## Why this matters for retrieval

- B1 entries should rank high for "how does X work" / "architecture"
  queries.
- B3 entries should rank high for "step by step" / "flow" / specific
  function questions.
- B4 + B5 entries should rank high when the user is *changing* code —
  they prevent regressions.
- B6 entries should rank high for "how do I add" / "how do I upgrade".
- B7 entries should rank high for value lookups.

CKV does not enforce this routing today, but the type annotation lets
downstream rerankers and the coding-agent planner weight entries by
intent (e.g. PLANNING phase favours B4/B5, IMPLEMENTATION phase favours
B6/B7).
