## ADDED Requirements

### Requirement: 单实例多链（链作为一等公民）
系统 MUST 支持在同一个后端进程中同时启用多条链，并且在核心业务对象中携带 `chain` 维度（例如 task/tx/trade 等）。

系统 MUST 至少支持：
- `bsc`
- `base`

系统 MUST 在启动时加载多链 `ChainConfig`（RPC/chainId/稳定币/Native/合约地址等），并将其用于交易闭环（开仓/撤仓/报价/展示）。

#### Scenario: 未传 chain 时默认使用 bsc
- **WHEN** 调用方未提供 `chain`（例如旧版 API 或旧数据）
- **THEN** 系统默认按 `bsc` 执行并保持现有行为（向后兼容）

#### Scenario: 同时存在 bsc/base 任务时分别走各自链 client
- **GIVEN** 系统同时启用了 `bsc` 与 `base`
- **WHEN** 两条链各自存在正在执行的开仓/撤仓任务
- **THEN** 系统对每个任务使用其 `task.chain` 对应的 RPC client/chainId/合约地址完成交易闭环

### Requirement: 稳定币 decimals 不能硬编码
系统 MUST 不再假设稳定币精度恒为 18（BSC USDT/USDC 常为 18，但 Base USDC/USDT 多为 6）。

系统 MUST 基于 `ChainConfig.StableTokenDecimals`（或链上读取结果）进行：
- “用户输入的 USD 金额” -> “稳定币最小单位 amountIn”
- OKX `/swap` 与 `/quote` 返回的 `toTokenAmount` 解析为 USD 值

#### Scenario: Base(6 decimals) 报价解析不出现 1e12 误差
- **GIVEN** `CHAIN=base` 且稳定币 decimals=6
- **WHEN** 系统将 OKX 返回的 `toTokenAmount` 转为 USD 数值用于阈值判断/展示
- **THEN** 解析结果与实际链上单位一致，不出现数量级偏差（例如 10^12）

### Requirement: OKX swap 请求必须使用正确 chainId
系统 MUST 在 OKX DEX aggregator 请求中使用当前链的 chainId（来自 `ChainConfig.ChainID`），并且 Router/TokenApprove 的 allowlist 校验 MUST 按当前链配置执行。

#### Scenario: Base swap 使用 Base chainId
- **GIVEN** `CHAIN=base`
- **WHEN** 系统调用 OKX `/swap` 构建 calldata
- **THEN** 请求参数中的 chainId 与 allowlist 校验均使用 Base 的配置

### Requirement: V3 工厂识别与 PositionManager 兜底
PoolService MUST 通过 V3 pool 的 `factory()` 地址识别 exchange（至少覆盖 BSC 的 Pancake/Uniswap + Base 的 Uniswap）。

当 exchange 无法识别时，开仓流程 MUST 仍可进行：系统 SHALL 使用当前链配置的默认 V3 PositionManager（而不是因为 `exchange` 字段不匹配而直接失败）。

#### Scenario: 未识别 exchange 仍可开仓
- **GIVEN** V3 pool 的 factory 地址不在已知列表
- **WHEN** 用户创建并开仓该 V3 pool
- **THEN** 系统使用默认 V3 PositionManager 完成开仓（或返回明确的“未配置 PositionManager”错误）

### Requirement: Explorer 链接按链渲染
系统 MUST 使用 `ChainConfig.ExplorerTxURLTemplate`（或等效配置）渲染交易链接，避免硬编码 `https://bscscan.com/tx/`。

#### Scenario: Base 展示 basescan 链接
- **GIVEN** `CHAIN=base`
- **WHEN** 系统在 Bot/页面中展示 txHash 链接
- **THEN** 链接指向 Base 的区块浏览器域名（例如 basescan），而不是 bscscan

### Requirement: 预留非 EVM 扩展点（Solana）
系统 MUST 在业务层引入可插拔的链执行入口（例如 `ChainExecutor` 接口或等效抽象），以便后续新增非 EVM 链（如 Solana）时，不需要在业务层到处散落 `if chain == ...`。

#### Scenario: 新增链只需实现 executor 并注册
- **GIVEN** 未来新增 `solana`（非 EVM）链
- **WHEN** 开发者为该链实现并注册 `ChainExecutor`
- **THEN** 现有业务入口可以通过 `chain` 分发到对应 executor，而不需要修改核心交易闭环的主流程结构
