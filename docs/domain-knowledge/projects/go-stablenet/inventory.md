# go-stablenet — Domain Knowledge Inventory

Project: `go-stablenet`
Schema version: 1
Code root: `/Users/wm-it-22-00661/Work/github/stable-net/go-stablenet-latest`

## Status summary

| Status | Count |
|---|---|
| verified | 0 |
| needs_verification | 15 |
| draft | 0 |
| needs_author | 0 |

## Subsystem coverage

| Subsystem | Name | Entries (verified / total) |
|---|---|---|
| A1 | WBFT Consensus Core | 0 / 2 |
| A2 | WBFT Block Encoding & Seal | 0 / 2 |
| A3 | WBFT Validator Set & Epoch | 0 / 1 |
| A4 | System Contracts (Governance × 5) | 0 / 1 |
| A5 | Native Coin & Account Extra | 0 / 1 |
| A6 | Fee Delegation & Anzeon Gas | 0 / 1 |
| A7 | Hardfork & Chain Config | 0 / 1 |
| A8 | Genesis & Bootstrap | 0 / 2 |
| A9 | P2P, Istanbul Protocol & Custom RPC | 0 / 2 |
| A10 | Build, Tooling & Codegen | 0 / 1 |
| A11 | Transaction Types, Mempool & State Transition | 0 / 1 |

## Knowledge-type coverage

| Type | Description | Count |
|---|---|---|
| B1 | Architecture | 3 |
| B2 | Data Structure | 1 |
| B3 | Algorithm / Flow | 5 |
| B4 | Invariant / Constraint | 2 |
| B5 | Pitfall / Anti-pattern | 1 |
| B6 | Procedure / Checklist | 1 |
| B7 | Reference (constants) | 2 |

## Conventions

- All entries follow `shared/SCHEMA.md`.
- Status transitions follow `shared/STATUS_LIFECYCLE.md`.
- Knowledge types follow `shared/KNOWLEDGE_TYPES.md`.

## Pending work

All 11 subsystems now have at least one entry; every knowledge type
(B1–B7) is exercised. Next step is reviewer verification — moving
entries from `needs_verification` to `verified` lets `ckv build`
pick them up.

After the first wave is verified, follow-up entries to consider:
- A1: round-change protocol (B3), preprepare/prepare/commit handlers (B3 each)
- A3: WBFT timing/config defaults (B7)
- A4: per-contract entries (gov_minter / gov_council semantics)
- A5: native coin issuance/burn flow (B3)
- A6: Anzeon authorized-account tip override (B3)
- A8: genesis JSON authoring checklist (B6)
- A10: contract artifact regeneration procedure (B6)
- A11: transaction taxonomy by type code (B2/B7), mempool ordering with AnzeonTipEnv (B3)

## Current entries

| ID | Subsystem | Type | Title | Status | Priority |
|---|---|---|---|---|---|
| A1.wbft_core.consensus_flow_architecture | A1 | B1 | WBFT consensus flow — actors and message sequence per height | needs_verification | P0 |
| A1.wbft_core.quorum_calc | A1 | B3 | WBFT quorum size calculation | needs_verification | P0 |
| A2.block_encoding.wbft_extra_struct | A2 | B2 | WBFTExtra struct layout and contained types | needs_verification | P0 |
| A2.block_encoding.filtered_header_hash | A2 | B3 | Block hash computation requires WBFTFilteredHeader first | needs_verification | P0 |
| A3.validator_set.epoch_transition | A3 | B3 | Epoch transition: how the next epoch's validator set is selected and pinned | needs_verification | P0 |
| A4.system_contracts.addresses | A4 | B7 | System contract canonical addresses (Anzeon) | needs_verification | P0 |
| A5.account_extra.bit_layout | A5 | B4 | StateAccount.Extra bit layout and immutability invariants | needs_verification | P0 |
| A6.fee_delegation.signing_model | A6 | B5 | Fee delegation signing model: setSignatureValues writes the FeePayer | needs_verification | P0 |
| A7.hardfork.add_new_fork_procedure | A7 | B6 | Procedure: adding a new hardfork to go-stablenet | needs_verification | P0 |
| A8.genesis.bootstrap_architecture | A8 | B1 | Genesis bootstrap architecture: from JSON to first block on disk | needs_verification | P0 |
| A8.genesis.inject_contracts_two_phase | A8 | B3 | InjectContracts two-phase deploy: baseline then block-0 hardfork overlay | needs_verification | P0 |
| A9.istanbul_p2p.protocol_architecture | A9 | B1 | istanbul/100 subprotocol architecture: piggy-backed on eth peers | needs_verification | P1 |
| A9.istanbul_rpc.api_reference | A9 | B7 | WBFT custom RPC API (istanbul namespace) — method reference | needs_verification | P1 |
| A10.codegen.no_edit_zones | A10 | B4 | Codegen no-edit zones: which files are generated and how to regenerate | needs_verification | P1 |
| A11.state_transition.blacklist_check_points | A11 | B3 | Blacklist check points along the state transition path | needs_verification | P0 |
