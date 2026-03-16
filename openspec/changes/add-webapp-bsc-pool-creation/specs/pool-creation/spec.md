## ADDED Requirements
### Requirement: BSC 建池 MUST 按协议执行费率与 PoolKey 校验
系统 MUST 在 `bsc` 链上提供面向 `Uniswap V3`、`Uniswap V4`、`PancakeSwap V3` 的建池能力，并按协议分别校验允许的 fee 参数与 `PoolKey` 组成。

#### Scenario: Uniswap V3 接受固定费率
- **WHEN** 用户在 BSC 创建 `Uniswap V3` 池子并提交 `100`、`500`、`3000` 或 `10000` 之一
- **THEN** 系统 SHALL 允许进入 preview / execute 流程

#### Scenario: PancakeSwap V3 拒绝非固定费率
- **WHEN** 用户在 BSC 创建 `PancakeSwap V3` 池子并提交不在 `100`、`500`、`2500`、`10000` 内的 fee
- **THEN** 系统 MUST 拒绝请求并返回明确的 fee 不支持原因

#### Scenario: Uniswap V4 接受任意静态费率与 tick spacing
- **WHEN** 用户在 BSC 创建 `Uniswap V4` 池子并提交合法的静态 `fee_tier` 与正整数 `tick_spacing`
- **THEN** 系统 SHALL 用这组参数构造 `PoolKey`，并继续 preview / execute

#### Scenario: Uniswap V4 拒绝动态费率或非零 hooks
- **WHEN** 用户尝试对 `Uniswap V4` 提交动态费率标记、非法 `tick_spacing` 或非零 hooks
- **THEN** 系统 MUST 拒绝请求并说明该参数组合暂不支持

### Requirement: 建池 preview MUST 支持 full range 与 custom range 的归一化
系统 MUST 在 preview 阶段统一完成 token 排序、初始价格推导、区间归一化与池子存在性校验，并把最终可执行的 ticks 返回给调用方。

#### Scenario: Full range 归一化
- **WHEN** 用户提交 `range_mode=full_range`
- **THEN** 系统 SHALL 根据目标协议的 `tick_spacing` 返回合法的 `tickLower` 与 `tickUpper`

#### Scenario: Custom range 归一化
- **WHEN** 用户提交 `range_mode=custom_range` 和价格区间
- **THEN** 系统 SHALL 将价格区间转换为按 `tick_spacing` 对齐的 `tickLower` 与 `tickUpper`，并返回归一化后的价格摘要

#### Scenario: 目标池已存在
- **WHEN** preview 检测到同协议、同 pair、同 fee 与同 `PoolKey` 的目标池已经存在
- **THEN** 系统 MUST 在 preview 结果中标记该池已存在，并阻止 execute 成功发起新建池交易

### Requirement: 建池首仓 MUST 支持双币输入与单币自动换币
系统 MUST 让 `create_and_seed` 同时支持双币精确输入和单币输入后自动换币建池，并在 preview 返回对应的金额摘要与风险提示。

#### Scenario: 双币精确输入
- **WHEN** 用户选择 `create_and_seed` 且提交 `amount_mode=dual_exact` 与两侧 token 数量
- **THEN** 系统 SHALL 直接按两侧数量估算 liquidity 并准备首仓参数

#### Scenario: 单币输入镜像估算
- **WHEN** 用户在 preview 中只输入一侧 token 数量
- **THEN** 系统 SHALL 按当前价格与目标区间返回另一侧 token 的镜像金额估算与估算来源说明

#### Scenario: 单币自动换币执行
- **WHEN** 用户选择 `create_and_seed` 且提交 `amount_mode=single_auto_swap` 与单侧 token 数量
- **THEN** 系统 MUST 在 execute 时生成换币参数、完成自动配比并创建首仓

#### Scenario: 余额不足或换币报价不可用
- **WHEN** 单币自动换币所需余额不足，或路由报价无法生成
- **THEN** 系统 MUST 终止 execute，并返回明确的失败原因

### Requirement: V3 / Pancake V3 的新池首仓 MUST 复用标准 create + mint / zap 流程
对于 `Uniswap V3` 与 `PancakeSwap V3`，系统 MUST 先完成池子初始化，再按金额模式选择 `mint` 或 `zapInV3` 完成首仓。

#### Scenario: 双币首仓使用 mint
- **WHEN** 用户对 V3 类协议提交 `dual_exact`
- **THEN** 系统 SHALL 在 `createAndInitializePoolIfNecessary` 成功后继续执行 `mint`，并返回 `pool address`、`tokenId` 和 `tx hash`

#### Scenario: 单币首仓使用 zapInV3
- **WHEN** 用户对 V3 类协议提交 `single_auto_swap`
- **THEN** 系统 SHALL 在池子初始化后使用 `zapInV3` 与换币参数完成首仓，并返回 `pool address`、`tokenId` 和 `tx hash`

### Requirement: V4 新池 MUST 支持任意静态费率、任意合法区间和单币首仓
对于 `Uniswap V4`，系统 MUST 在保持 `zero hooks` 的前提下，支持任意静态 fee、任意合法区间和单币自动换币首仓。

#### Scenario: V4 双币首仓
- **WHEN** 用户对 `Uniswap V4` 提交合法的 `fee_tier`、`tick_spacing`、区间和双币数量
- **THEN** 系统 SHALL 完成 `initializePool` 并继续执行无 swap 的 `zapInV4`

#### Scenario: V4 单币首仓
- **WHEN** 用户对 `Uniswap V4` 提交合法的 `fee_tier`、`tick_spacing`、区间并选择 `single_auto_swap`
- **THEN** 系统 SHALL 完成 `initializePool`，再使用带 swap 参数的 `zapInV4` 创建首仓并返回 `poolId`、`tokenId` 和 `tx hash`
