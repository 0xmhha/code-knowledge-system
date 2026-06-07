# go-stablenet — Domain Knowledge Inventory

Project: `go-stablenet`
Schema version: 1
Code root: resolved at runtime from `$CKS_CODE_ROOT` (override) or `$GO_STABLENET_ROOT` — see `project.yaml`; no machine path is committed.
Verification baseline: go-stablenet `@9978930ba` (dev — WBFT justification fix #84).

## Status summary

| Status | Count |
|---|---|
| verified | 36 |
| needs_verification | 0 |
| draft | 0 |
| needs_author | 0 |

## Subsystem coverage

| Subsystem | Name | Entries (verified / total) |
|---|---|---|
| A1 | WBFT Consensus Core | 5 / 5 |
| A2 | WBFT Block Encoding & Seal | 2 / 2 |
| A3 | WBFT Validator Set & Epoch | 2 / 2 |
| A4 | System Contracts (Governance × 5) | 2 / 2 |
| A5 | Native Coin & Account Extra | 2 / 2 |
| A6 | Fee Delegation & Anzeon Gas | 2 / 2 |
| A7 | Hardfork & Chain Config | 1 / 1 |
| A8 | Genesis & Bootstrap | 3 / 3 |
| A9 | P2P, Istanbul Protocol & Custom RPC | 2 / 2 |
| A10 | Build, Tooling & Codegen | 2 / 2 |
| A11 | Transaction Types, Mempool & State Transition | 1 / 1 |
| A12 | Cryptographic Primitives & Validator Seals | 2 / 2 |
| A13 | Block Production & Sealing | 1 / 1 |
| A14 | Protocol Foundations & Design Philosophy | 9 / 9 |

## Knowledge-type coverage

| Type | Description | Count |
|---|---|---|
| B1 | Architecture | 5 |
| B2 | Data Structure | 2 |
| B3 | Algorithm / Flow | 10 |
| B4 | Invariant / Constraint | 9 |
| B5 | Pitfall / Anti-pattern | 3 |
| B6 | Procedure / Checklist | 4 |
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
| A1.concurrency.core_lock_discipline | A1 | B4 | WBFT Core lock discipline — never read Core.current off currentMutex | verified | P0 |
| A1.wbft_core.consensus_flow_architecture | A1 | B1 | WBFT consensus flow — actors and message sequence per height | verified | P0 |
| A1.wbft_core.message_handlers | A1 | B3 | WBFT consensus message handlers: preprepare, prepare, commit | verified | P0 |
| A1.wbft_core.quorum_calc | A1 | B3 | WBFT quorum size calculation | verified | P0 |
| A1.wbft_core.round_change_protocol | A1 | B3 | WBFT round change: timeout-triggered round bump with prepared-block justification | verified | P0 |
| A10.codegen.contract_regen_procedure | A10 | B6 | System contract artifact regeneration: solc 0.8.14 to artifacts/v1 and v2 | verified | P2 |
| A10.codegen.no_edit_zones | A10 | B4 | Codegen no-edit zones: which files are generated and how to regenerate | verified | P1 |
| A11.state_transition.blacklist_check_points | A11 | B3 | Blacklist check points along the state transition path | verified | P0 |
| A12.seals.bls_seal_scheme | A12 | B2 | WBFT BLS seal scheme — 96-byte aggregate + SealerSet bitpack | verified | P0 |
| A12.seals.feepayer_sighash | A12 | B3 | Fee-delegation sigHash — feepayer signs the wrapped, not the inner, tx | verified | P0 |
| A13.sealing.reorg_serialization | A13 | B4 | Txpool mutation must serialize through the reorg loop | verified | P0 |
| A14.foundations.base_fee_redistribution | A14 | B1 | Base fee is redistributed to validators (not burned) | verified | P0 |
| A14.foundations.cherry_pick_principle | A14 | B6 | Isolate StableNet code so upstream geth fixes stay cherry-pickable | verified | P0 |
| A14.foundations.equal_power | A14 | B4 | Equal-power validators — every validator has voting power = 1 | verified | P0 |
| A14.foundations.instant_finality_inert_reorg | A14 | B5 | Instant BFT finality — geth reorg / Td code is inert (a trap) | verified | P0 |
| A14.foundations.wkrc_not_eth | A14 | B4 | Native asset is WKRC (KRW stablecoin), not ETH | verified | P0 |
| A14.theory.3f1_intersection | A14 | B4 | BFT 3f+1 quorum intersection — why ceil(N − F) is the safety floor | verified | P0 |
| A14.theory.equivocation | A14 | B5 | Equivocation — byzantine validators may send conflicting messages | verified | P0 |
| A14.theory.flp_partial_synchrony | A14 | B4 | FLP impossibility — WBFT trades termination for partial synchrony | verified | P0 |
| A14.theory.justification_locking | A14 | B4 | Justification + locking — why view changes preserve safety | verified | P0 |
| A2.block_encoding.filtered_header_hash | A2 | B3 | Block hash computation requires WBFTFilteredHeader first | verified | P0 |
| A2.block_encoding.wbft_extra_struct | A2 | B2 | WBFTExtra struct layout and contained types | verified | P0 |
| A3.timing.wbft_config_defaults | A3 | B7 | WBFT timing and config defaults: RequestTimeout, BlockPeriod, EpochLength | verified | P1 |
| A3.validator_set.epoch_transition | A3 | B3 | Epoch transition: how the next epoch's validator set is selected and pinned | verified | P0 |
| A4.system_contracts.addresses | A4 | B7 | System contract canonical addresses (Anzeon) | verified | P0 |
| A4.system_contracts.gov_minter | A4 | B1 | GovMinter (0x1003): native coin minting authority with v1/v2 versions | verified | P1 |
| A5.account_extra.bit_layout | A5 | B4 | StateAccount.Extra bit layout and immutability invariants | verified | P0 |
| A5.native_coin.issuance_burn_flow | A5 | B3 | Native coin issuance and burn via NativeCoinManager precompile | verified | P1 |
| A6.anzeon_gas.tip_override | A6 | B3 | Anzeon authorized-account gas tip override | verified | P1 |
| A6.fee_delegation.signing_model | A6 | B5 | Fee delegation signing model: setSignatureValues writes the FeePayer, not the Sender | verified | P0 |
| A7.hardfork.add_new_fork_procedure | A7 | B6 | Procedure: adding a new hardfork to go-stablenet | verified | P0 |
| A8.genesis.bootstrap_architecture | A8 | B1 | Genesis bootstrap architecture: from JSON to first block on disk | verified | P0 |
| A8.genesis.inject_contracts_two_phase | A8 | B3 | InjectContracts two-phase deploy: baseline then block-0 hardfork overlay | verified | P0 |
| A8.genesis.json_authoring_checklist | A8 | B6 | Genesis JSON authoring checklist for stablenet chains | verified | P1 |
| A9.istanbul_p2p.protocol_architecture | A9 | B1 | istanbul/100 subprotocol architecture: piggy-backed on eth peers | verified | P1 |
| A9.istanbul_rpc.api_reference | A9 | B7 | WBFT custom RPC API (istanbul namespace) — method reference | verified | P1 |
