## MODIFIED Requirements

### Requirement: Per-wallet private Zap contract binding
The system MUST support a per-wallet, per-chain private Zap contract address binding.

The binding MUST be used for all Zap interactions for that wallet and chain, including:
- token approvals to Zap
- `zapInV3` / `zapInV4` calls
- receipt/event parsing that relies on the Zap contract address

私有 Zap 合约的资金入口 MUST 只允许部署该合约的钱包调用。外部聚合器 swap target 和 approve target MUST NOT 作为 Zap 执行外部 swap 的链上白名单前置条件。Zap 配置流程 MUST NOT 要求 OKX/Binance swap target 或 approve target 地址；它只需要配置 V3/V4 Position Manager 和 wrapped native。

#### Scenario: First open deploys and binds a private Zap contract
- **GIVEN** wallet `W` has no private Zap binding on chain `bsc`
- **WHEN** the user opens a new position on `bsc` using wallet `W`
- **THEN** the backend deploys a new Zap contract from wallet `W`, configures position managers, persists the binding, and uses that address for the entry transaction.

#### Scenario: Two wallets are isolated
- **GIVEN** a user has two wallets `A` and `B`
- **AND** both wallets have private Zap bindings on `base`
- **WHEN** wallet `A` approves USDC spending for Zap
- **THEN** the approval target is wallet `A`'s bound Zap address and does not grant any allowance to wallet `B`'s Zap address.

#### Scenario: Non-owner cannot use a private Zap
- **GIVEN** wallet `A` deployed private Zap contract `Z`
- **WHEN** wallet `B` calls a funds-moving Zap entry point on `Z`
- **THEN** the transaction MUST revert.

#### Scenario: Aggregator target allowlist is not required
- **GIVEN** wallet `A` deployed private Zap contract `Z`
- **AND** Binance or OKX returns swap calldata with a swap target and approve target that were not added to Zap target allowlists
- **WHEN** wallet `A` calls the Zap entry point using that swap calldata
- **THEN** Zap MUST NOT revert because of untrusted swap target or untrusted approve target.
