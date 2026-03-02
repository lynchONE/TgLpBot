# Change: Per-Wallet Private Zap Contracts (BSC/Base)

## Why
- Today the bot uses a single shared Zap contract per chain (BSC/Base). All user wallets approve and interact with the same address, which increases blast radius if the contract logic/config is compromised or a usage bug exists.
- We want isolation: each wallet should interact with its own private Zap contract address so approvals are not shared across users.
- We also need an upgrade path: when we ship a new Zap contract build, previously bound per-wallet contracts must be treated as invalid so the next open deploys/binds a fresh instance.

## What Changes
- Add a per-wallet, per-chain "private Zap contract" binding stored in MySQL.
- On the first open (entry) for a wallet, the backend deploys a new `ZapSimple` instance using that wallet as the tx sender, configures trusted addresses, then persists the deployed address as the wallet's active Zap contract for that chain.
- All later Zap interactions (token approvals, `zapInV3/zapInV4`) use the wallet's bound private Zap contract address instead of `ChainConfig.ZapV3Address/ZapV4Address`.
- Uniswap V3 exits do not use Zap: the backend exits by calling the V3 `NonfungiblePositionManager` directly (`decreaseLiquidity+collect`), so exit does not require a private Zap deployment/binding.
- Introduce a version number (config-driven). If the configured version is higher than the stored binding version, the binding is considered invalid and the backend redeploys and rebinds on next open.

## Impact
- DB: new table to store `wallet_id + chain -> zap_contract_address + version (+ tx hashes)`.
- Runtime: first usage requires extra transactions (deploy + config). This increases time and gas cost for first usage per wallet per chain.
- Security: approvals are isolated per wallet; compromise impact is limited to a single wallet's approved address.
- Ops: when upgrading Zap code, bump the configured version to force re-deploy/re-bind on the next open per wallet.

## Rollout / Backwards Compatibility
- Keep legacy shared Zap addresses in `ChainConfig` for rollback/emergency mode.
- Add a runtime flag (env or GlobalConfig) to enable/disable private Zap mode:
  - When disabled: behavior remains unchanged (shared Zap address).
  - When enabled: new private bindings are created lazily.

## Open Questions
- Should the binding be per-user or per-wallet? (Recommendation: per-wallet, because approvals and contract deployment are per EOA wallet; this also supports multi-wallet mode cleanly.)
- Do we require "on-chain invalidation" (old contracts revert) or is "bot-level invalidation" (stop using old addresses) sufficient? (This change implements bot-level invalidation by version gating.)
