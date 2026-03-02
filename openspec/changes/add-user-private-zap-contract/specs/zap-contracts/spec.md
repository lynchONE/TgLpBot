## ADDED Requirements

### Requirement: Per-wallet private Zap contract binding
The system MUST support a per-wallet, per-chain private Zap contract address binding.

The binding MUST be used for all Zap interactions for that wallet and chain, including:
- token approvals to Zap
- `zapInV3` / `zapInV4` calls
- receipt/event parsing that relies on the Zap contract address

#### Scenario: First open deploys and binds a private Zap contract
- **GIVEN** wallet `W` has no private Zap binding on chain `bsc`
- **WHEN** the user opens a new position on `bsc` using wallet `W`
- **THEN** the backend deploys a new Zap contract from wallet `W`, configures trusted addresses, persists the binding, and uses that address for the entry transaction.

#### Scenario: Two wallets are isolated
- **GIVEN** a user has two wallets `A` and `B`
- **AND** both wallets have private Zap bindings on `base`
- **WHEN** wallet `A` approves USDC spending for Zap
- **THEN** the approval target is wallet `A`'s bound Zap address and does not grant any allowance to wallet `B`'s Zap address.

### Requirement: V3 exits do not use Zap
The system MUST exit Uniswap V3 positions by calling the V3 `NonfungiblePositionManager` directly (`decreaseLiquidity` + `collect`).

V3 exits MUST NOT require deploying, binding, or approving any Zap contract address.

#### Scenario: Exit uses NPM directly
- **GIVEN** wallet `W` has a V3 position tokenId `T`
- **WHEN** the user exits the position
- **THEN** the backend sends transactions to the V3 position manager (via `multicall` or `decreaseLiquidity+collect`) and does not reference any Zap address.

### Requirement: Private Zap versioning and invalidation
Each private Zap binding MUST store a `version`.

The runtime config MUST define the current required `PRIVATE_ZAP_VERSION`.

If a wallet's stored binding version does not match `PRIVATE_ZAP_VERSION`, the binding MUST be treated as invalid and MUST NOT be used for new Zap interactions.

#### Scenario: Upgrade forces re-deploy and re-bind
- **GIVEN** `PRIVATE_ZAP_VERSION=2`
- **AND** wallet `W` has a stored private Zap binding with `version=1`
- **WHEN** the user opens a new position using wallet `W`
- **THEN** the backend deploys and binds a new private Zap contract at `version=2` before sending the entry transaction.

### Requirement: Private Zap binding Redis cache
The system MUST support caching per-wallet private Zap binding resolution in Redis.

The cache key MUST include chain, wallet id, contract kind, and required private zap version.

The cache TTL MUST be 1 hour.

On cache miss, the backend MUST read from DB and repopulate Redis cache.

#### Scenario: Cache miss falls back to DB and repopulates cache
- **GIVEN** wallet `W` already has a valid private Zap binding in DB
- **AND** Redis has no corresponding cache key
- **WHEN** the backend resolves private Zap address for wallet `W`
- **THEN** the backend reads DB binding, returns the address, and writes the cache key with 1 hour TTL.

### Requirement: Rollback to shared Zap (emergency)
The system MUST support disabling private Zap mode and reverting to shared Zap addresses from `ChainConfig`.

#### Scenario: Private Zap is disabled
- **GIVEN** `PRIVATE_ZAP_ENABLED=false`
- **WHEN** the user opens a new position
- **THEN** the backend uses `ChainConfig.ZapV3Address/ZapV4Address` as before and does not deploy or bind a private Zap contract.
