# go-stablenet — Domain Knowledge Inventory

Project: `go-stablenet`
Schema version: 1
Code root: `/Users/wm-it-22-00661/Work/github/stable-net/go-stablenet-latest`

## Status summary

| Status | Count |
|---|---|
| verified | 0 |
| needs_verification | 6 |
| draft | 0 |
| needs_author | 0 |

## Subsystem coverage

| Subsystem | Name | Entries (verified / total) |
|---|---|---|
| A1 | WBFT Consensus Core | 0 / 1 |
| A2 | WBFT Block Encoding & Seal | 0 / 0 |
| A3 | WBFT Validator Set & Epoch | 0 / 0 |
| A4 | System Contracts (Governance × 5) | 0 / 1 |
| A5 | Native Coin & Account Extra | 0 / 1 |
| A6 | Fee Delegation & Anzeon Gas | 0 / 1 |
| A7 | Hardfork & Chain Config | 0 / 1 |
| A8 | Genesis & Bootstrap | 0 / 0 |
| A9 | P2P, Istanbul Protocol & Custom RPC | 0 / 0 |
| A10 | Build, Tooling & Codegen | 0 / 0 |
| A11 | Transaction Types, Mempool & State Transition | 0 / 1 |

## Knowledge-type coverage

| Type | Description | Count |
|---|---|---|
| B1 | Architecture | 0 |
| B2 | Data Structure | 0 |
| B3 | Algorithm / Flow | 2 |
| B4 | Invariant / Constraint | 1 |
| B5 | Pitfall / Anti-pattern | 1 |
| B6 | Procedure / Checklist | 1 |
| B7 | Reference (constants) | 1 |

## Conventions

- All entries follow `shared/SCHEMA.md`.
- Status transitions follow `shared/STATUS_LIFECYCLE.md`.
- Knowledge types follow `shared/KNOWLEDGE_TYPES.md`.

## Pending work

Once the six sample entries below are reviewer-approved, this section
becomes the next-target list: P0 entries in subsystems that still have
zero coverage (A2, A3, A8, A9, A10). Build them out before any P1/P2.

## Current entries

| ID | Subsystem | Type | Title | Status |
|---|---|---|---|---|
| A1.wbft_core.quorum_calc | A1 | B3 | Quorum size calculation | needs_verification |
| A4.system_contracts.addresses | A4 | B7 | System contract canonical addresses | needs_verification |
| A5.account_extra.bit_layout | A5 | B4 | StateAccount.Extra bit allocation invariants | needs_verification |
| A6.fee_delegation.signing_model | A6 | B5 | Fee delegation: setSignatureValues is FeePayer, not Sender | needs_verification |
| A7.hardfork.add_new_fork_procedure | A7 | B6 | Procedure: adding a new hardfork | needs_verification |
| A11.state_transition.blacklist_check_points | A11 | B3 | Blacklist check points along the state transition path | needs_verification |
