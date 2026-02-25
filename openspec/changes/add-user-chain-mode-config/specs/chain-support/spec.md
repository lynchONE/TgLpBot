## ADDED Requirements

### Requirement: 用户可配置的链模式
系统 MUST 允许用户通过 `GlobalConfig` 配置“链选择行为”，包含：
- `multi_chain_enabled`（bool）：是否启用多链选择交互。
- `default_chain`（string）：默认链（例如 `bsc`、`base`）。

当 `multi_chain_enabled=false` 时，系统 MUST 使用用户的 `default_chain` 作为“有效链”（effective chain）来执行所有需要链上下文的用户操作（包含：开仓、钱包 dust swap），并且 MUST 不要求用户在 Bot 或 Mini App 中手动选择链。

当 `multi_chain_enabled=true` 时，系统 SHALL 保持现有多链行为（当服务器配置启用了多条链时，允许/提示用户选择链）。

#### Scenario: 关闭 multi-chain 时强制使用默认链
- **GIVEN** server enabled chains are `bsc,base`
- **AND** user `multi_chain_enabled=false` and `default_chain=base`
- **WHEN** the user opens a position from the Mini App (even if the request body contains `chain=bsc`)
- **THEN** the backend uses `base` as the effective chain for validation and execution.

#### Scenario: 关闭 multi-chain 时 Bot 不再提示选链
- **GIVEN** server enabled chains are `bsc,base`
- **AND** user `multi_chain_enabled=false` and `default_chain=bsc`
- **WHEN** the user starts `/newposition` or pastes a pool address directly
- **THEN** the Bot does not show a chain selection keyboard and continues using `bsc`.

#### Scenario: 开启 multi-chain 时保持现有行为
- **GIVEN** server enabled chains are `bsc,base`
- **AND** user `multi_chain_enabled=true`
- **WHEN** the user starts `/newposition`
- **THEN** the Bot prompts the user to select the chain before requesting a pool address.

### Requirement: 默认链必须有效
系统 MUST 校验 `default_chain` 是否在服务器启用链列表内（`CHAINS` / `EnabledChains`）。

如果用户配置的 `default_chain` 未被服务器启用，系统 MUST 回退到安全的默认链：
- 若 `bsc` 已启用，则优先用 `bsc`
- 否则使用服务器启用链列表中的第一条

#### Scenario: 用户配置了 default_chain 但服务器未启用
- **GIVEN** server enabled chains are `bsc` only
- **AND** user `default_chain=base`
- **WHEN** the user opens a position
- **THEN** the system falls back to `bsc` as the effective chain.
