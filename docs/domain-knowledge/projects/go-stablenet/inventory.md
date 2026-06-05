# go-stablenet — Domain Knowledge Inventory

Project: `go-stablenet`
Schema version: 1
Code root: resolved at runtime from `$CKS_CODE_ROOT` (override) or `$GO_STABLENET_ROOT` — see `project.yaml`; no machine path is committed.
Verification baseline: go-stablenet `@9978930ba` (dev — WBFT justification fix #84).

## Status summary

| Status | Count |
|---|---|
| verified | 7 |
| needs_verification | 16 |
| draft | 0 |
| needs_author | 0 |

## Subsystem coverage

| Subsystem | Name | Entries (verified / total) |
|---|---|---|
| A1 | WBFT Consensus Core | 0 / 4 |
| A2 | WBFT Block Encoding & Seal | 0 / 2 |
| A3 | WBFT Validator Set & Epoch | 1 / 2 |
| A4 | System Contracts (Governance × 5) | 1 / 2 |
| A5 | Native Coin & Account Extra | 2 / 2 |
| A6 | Fee Delegation & Anzeon Gas | 1 / 2 |
| A7 | Hardfork & Chain Config | 0 / 1 |
| A8 | Genesis & Bootstrap | 1 / 3 |
| A9 | P2P, Istanbul Protocol & Custom RPC | 0 / 2 |
| A10 | Build, Tooling & Codegen | 0 / 2 |
| A11 | Transaction Types, Mempool & State Transition | 1 / 1 |

## Knowledge-type coverage

| Type | Description | Count |
|---|---|---|
| B1 | Architecture | 4 |
| B2 | Data Structure | 1 |
| B3 | Algorithm / Flow | 9 |
| B4 | Invariant / Constraint | 2 |
| B5 | Pitfall / Anti-pattern | 1 |
| B6 | Procedure / Checklist | 3 |
| B7 | Reference (constants) | 3 |

## Conventions

- All entries follow `shared/SCHEMA.md`.
- Status transitions follow `shared/STATUS_LIFECYCLE.md`.
- Knowledge types follow `shared/KNOWLEDGE_TYPES.md`.

## Pending work

All 11 subsystems and every knowledge type (B1–B7) are covered, and
the second-wave entries (round change, message handlers, timing
defaults, GovMinter, native coin flow, Anzeon tip override, genesis
checklist, contract regen) have landed alongside the first wave. The
inventory is now in the reviewer-verification phase — moving entries
from `needs_verification` to `verified` is what lets `ckv build` pick
them up.

Remaining candidates worth adding once the current waves are verified:

- A4: per-contract entries for GovCouncil and GovMasterMinter
- A11: transaction taxonomy by type code (B2/B7), mempool ordering with AnzeonTipEnv (B3), fee-delegation (0x16) signing path
- A2: WBFTExtra round-trip examples (B5)
- A9: istanbul peer handshake details (B3)

## Current entries

| ID | Subsystem | Type | Title | Status | Priority |
|---|---|---|---|---|---|
| A1.wbft_core.consensus_flow_architecture | A1 | B1 | WBFT consensus flow — actors and message sequence per height | needs_verification | P0 |
| A1.wbft_core.message_handlers | A1 | B3 | WBFT consensus message handlers: preprepare, prepare, commit | needs_verification | P0 |
| A1.wbft_core.quorum_calc | A1 | B3 | WBFT quorum size calculation | needs_verification | P0 |
| A1.wbft_core.round_change_protocol | A1 | B3 | WBFT round change: timeout-triggered round bump with prepared-block justification | needs_verification | P0 |
| A10.codegen.contract_regen_procedure | A10 | B6 | System contract artifact regeneration: solc 0.8.14 to artifacts/v1 and v2 | needs_verification | P2 |
| A10.codegen.no_edit_zones | A10 | B4 | Codegen no-edit zones: which files are generated and how to regenerate | needs_verification | P1 |
| A11.state_transition.blacklist_check_points | A11 | B3 | Blacklist check points along the state transition path | verified | P0 |
| A2.block_encoding.filtered_header_hash | A2 | B3 | Block hash computation requires WBFTFilteredHeader first | needs_verification | P0 |
| A2.block_encoding.wbft_extra_struct | A2 | B2 | WBFTExtra struct layout and contained types | needs_verification | P0 |
| A3.timing.wbft_config_defaults | A3 | B7 | WBFT timing and config defaults: RequestTimeout, BlockPeriod, EpochLength | verified | P1 |
| A3.validator_set.epoch_transition | A3 | B3 | Epoch transition: how the next epoch's validator set is selected and pinned | needs_verification | P0 |
| A4.system_contracts.addresses | A4 | B7 | System contract canonical addresses (Anzeon) | verified | P0 |
| A4.system_contracts.gov_minter | A4 | B1 | GovMinter (0x1003): native coin minting authority with v1/v2 versions | needs_verification | P1 |
| A5.account_extra.bit_layout | A5 | B4 | StateAccount.Extra bit layout and immutability invariants | verified | P0 |
| A5.native_coin.issuance_burn_flow | A5 | B3 | Native coin issuance and burn via NativeCoinManager precompile | verified | P1 |
| A6.anzeon_gas.tip_override | A6 | B3 | Anzeon authorized-account gas tip override | verified | P1 |
| A6.fee_delegation.signing_model | A6 | B5 | Fee delegation signing model: setSignatureValues writes the FeePayer, not the Sender | needs_verification | P0 |
| A7.hardfork.add_new_fork_procedure | A7 | B6 | Procedure: adding a new hardfork to go-stablenet | needs_verification | P0 |
| A8.genesis.bootstrap_architecture | A8 | B1 | Genesis bootstrap architecture: from JSON to first block on disk | needs_verification | P0 |
| A8.genesis.inject_contracts_two_phase | A8 | B3 | InjectContracts two-phase deploy: baseline then block-0 hardfork overlay | verified | P0 |
| A8.genesis.json_authoring_checklist | A8 | B6 | Genesis JSON authoring checklist for stablenet chains | needs_verification | P1 |
| A9.istanbul_p2p.protocol_architecture | A9 | B1 | istanbul/100 subprotocol architecture: piggy-backed on eth peers | needs_verification | P1 |
| A9.istanbul_rpc.api_reference | A9 | B7 | WBFT custom RPC API (istanbul namespace) — method reference | needs_verification | P1 |
