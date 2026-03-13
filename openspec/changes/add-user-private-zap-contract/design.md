# Design Notes: Per-Wallet Private Zap Contracts

## Goal
Create isolation between users by ensuring each wallet interacts with its own Zap contract address. The backend must bind this address and reuse it for all later Zap interactions (entries). When Zap code is upgraded, bindings must be invalidated so a fresh contract is deployed/bound on next open.

## Scope / Non-Goals
- In-scope: bot-level invalidation (stop using old bound addresses when version changes).
- Out-of-scope (future hardening): on-chain invalidation (old contracts forcibly revert), factory/clones to reduce deploy gas, auto-revoke old approvals.

## Data Model
- New table (name TBD, e.g. `wallet_chain_contracts`):
  - `wallet_id` (uint, indexed)
  - `chain` (string, indexed)
  - `contract_kind` (string,  default `zap_simple`)
  - `contract_address` (string, 42 chars)
  - `version` (int)
  - `deploy_tx_hash` (string, 66 chars, optional)
  - `config_tx_hash` (string, 66 chars, optional)
  - timestamps
- Unique key: `(wallet_id, chain, contract_kind)`.

## Resolution Flow (Backend)
`EnsurePrivateZapContract(wallet, chain)`:
1. Read binding for `(wallet_id, chain, zap_simple)`.
2. If missing OR `binding.version != PRIVATE_ZAP_VERSION`:
   - Deploy ZapSimple contract using the wallet's private key (tx sender is the wallet).
   - Call `setTrustedAddresses` with chain-scoped values:
     - `okxSwapRouter`, `okxTokenApprove`
     - `DefaultV3PositionManagerAddress`
     - `UniswapV4PositionManagerAddress` (or zero if not used on that chain)
   - Call `setTrustedV3PositionManagers` to allowlist other configured V3 managers for that chain.
   - Persist binding in DB with version and tx hashes.
3. Return `contract_address`.

Notes:
- Deployment/config must be idempotent under concurrency (two opens for the same wallet should not create two bindings). Prefer DB uniqueness + retry.
- All later Zap approval targets and Zap event parsing MUST use the resolved address.
- Uniswap V3 exits SHOULD call the V3 `NonfungiblePositionManager` directly (`decreaseLiquidity+collect`) and MUST NOT depend on Zap.

## Upgrade Strategy
- Release process:
  - Update contract build (bytecode) shipped with backend.
  - Bump `PRIVATE_ZAP_VERSION`.
- Runtime behavior:
  - On next open, the bot detects version mismatch and deploys/binds a fresh ZapSimple instance.
  - The old address is treated as inactive and is no longer used by the backend.

## Rollback Strategy
- Toggle `PRIVATE_ZAP_ENABLED=false` to revert to legacy shared `ChainConfig.ZapV3Address/ZapV4Address` behavior.
