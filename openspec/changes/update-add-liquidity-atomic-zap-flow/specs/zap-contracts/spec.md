## ADDED Requirements

### Requirement: Per-wallet private Zap binding MUST cover atomic add-liquidity interactions
系统 MUST 为原子补仓建立独立的私有合约绑定，并将相关 ERC20 / ERC721 授权与补仓调用统一指向该绑定地址。

#### Scenario: 已绑定原子私有 Zap 的钱包执行 V4 原子补仓
- **GIVEN** 钱包 `W` 在链 `bsc` 上已经绑定了有效的 `atomic_increase_zap` 地址
- **AND** 钱包 `W` 持有一个可补仓的 V4 `tokenId`
- **WHEN** 用户对该仓位执行原子补仓
- **THEN** 后端 MUST 使用钱包 `W` 绑定的 `atomic_increase_zap` 地址处理稳定币授权、NFT 授权与 `zapIncreaseV4` 调用
- **AND** MUST NOT 回退到其他钱包的 Zap 地址或共享 Zap 地址

### Requirement: Atomic add-liquidity rollout MUST be gated by a dedicated feature flag
系统 MUST 通过独立的灰度开关控制是否启用原子补仓路径，并在启用时自动准备所需的原子私有 Zap 绑定。

#### Scenario: 灰度开关关闭时继续使用旧补仓链路
- **GIVEN** `ATOMIC_ADD_LIQUIDITY_ENABLED=false`
- **WHEN** 用户发起补仓
- **THEN** 后端 MUST 继续使用旧的两段式补仓链路
- **AND** MUST NOT 部署或调用 `atomic_increase_zap`

#### Scenario: 灰度开关开启且缺少原子绑定时自动部署
- **GIVEN** `ATOMIC_ADD_LIQUIDITY_ENABLED=true`
- **AND** `PRIVATE_ZAP_ENABLED=true`
- **AND** 钱包 `W` 当前没有可用的 `atomic_increase_zap` 绑定
- **WHEN** 钱包 `W` 首次执行原子补仓
- **THEN** 后端 MUST 先部署并配置新的 `atomic_increase_zap` 绑定
- **AND** MUST 使用新的绑定地址执行后续原子补仓调用
