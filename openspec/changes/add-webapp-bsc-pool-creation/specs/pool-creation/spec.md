## ADDED Requirements
### Requirement: BSC 支持受限协议的一键建池
系统 MUST 在 `bsc` 链上提供面向 `Uniswap V3`、`Uniswap V4`、`PancakeSwap V3` 的建池能力，并对不在一期范围内的协议或参数组合进行拒绝。

#### Scenario: 选择受支持的协议
- **WHEN** 用户在 BSC 的创建池子模块中选择 `Uniswap V3`、`Uniswap V4` 或 `PancakeSwap V3`
- **THEN** 系统 SHALL 允许进入预校验与执行流程

#### Scenario: 选择不受支持的链或协议
- **WHEN** 用户尝试在非 BSC 链、或使用一期未开放的协议能力发起建池
- **THEN** 系统 MUST 阻止执行，并返回明确的不可用原因

### Requirement: 建池请求 MUST 先经过 preview 归一化和校验
系统 MUST 提供建池预校验能力，在实际发起链上交易前统一完成 token 元数据读取、token 排序、现有池检查、价格换算和参数派生。

#### Scenario: 预校验返回归一化摘要
- **WHEN** 用户提交一组待建池参数进行 preview
- **THEN** 系统 SHALL 返回归一化后的 token0/token1、目标协议、目标费率或模板、初始价格摘要、默认区间以及风险提示

#### Scenario: 目标池已存在
- **WHEN** preview 检测到相同协议下同一 pair 与相同 key 的池子已存在
- **THEN** 系统 MUST 在 preview 结果中标记该池已存在，并阻止后续 execute 成功发起新建池交易

### Requirement: 建池执行流 MUST 独立于现有开仓任务
系统 MUST 使用独立的建池执行流处理新池创建与首仓，不得依赖 `open_position`、`StrategyTask` 或稳定币入场任务模型。

#### Scenario: 执行建池并返回结果对象
- **WHEN** 用户确认 preview 结果并提交 execute
- **THEN** 系统 SHALL 使用所选钱包发起独立交易，并返回包含协议、交易哈希、pool address 或 poolId、以及可用 tokenId 的结果对象

### Requirement: V3 建池与首仓 MUST 使用 PositionManager 标准流程
对于 `Uniswap V3` 与 `PancakeSwap V3`，系统 MUST 先检查目标池是否存在，再使用 PositionManager 完成池子初始化，并在需要时完成首仓 mint。

#### Scenario: 仅创建 V3 池子
- **WHEN** 用户对 V3 协议选择 `create_only`
- **THEN** 系统 MUST 检查 factory 中的池子是否已存在，并调用 `createAndInitializePoolIfNecessary` 创建并初始化池子

#### Scenario: 创建并首注 V3 池子
- **WHEN** 用户对 V3 协议选择 `create_and_seed`
- **THEN** 系统 MUST 在创建并初始化池子后完成所需授权，并调用 `mint` 打入首笔流动性并返回 NFT tokenId

### Requirement: V4 建池一期 MUST 限制为 zero hooks 和 static fee
对于 `Uniswap V4`，系统 MUST 在一期仅支持零 hooks、静态费率与受控模板，不得接受自定义 hooks 或动态费率输入。

#### Scenario: 使用允许的 V4 模板
- **WHEN** 用户选择符合一期限制的 V4 费率模板和零 hooks 配置
- **THEN** 系统 SHALL 允许继续 preview 与 execute，并统一计算和校验 PoolKey 与 poolId

#### Scenario: 提交自定义 hooks 或一期外费率模式
- **WHEN** 用户尝试提交非零 hooks、动态费率或一期未开放的 V4 参数组合
- **THEN** 系统 MUST 拒绝请求，并返回该参数组合暂不支持的说明

### Requirement: 首仓默认使用 full range，且不得自动换币
系统 MUST 将 `create_and_seed` 的默认首仓模式设为 `full_range`，并要求用户提供两侧 token 数量；一期不得把自动换币纳入建池首仓流程。

#### Scenario: 仅创建模式无需填写首仓数量
- **WHEN** 用户选择 `create_only`
- **THEN** 系统 SHALL 不要求用户提供首仓 token 数量

#### Scenario: 创建并首注需要双币数量
- **WHEN** 用户选择 `create_and_seed`
- **THEN** 系统 MUST 要求输入两侧 token 数量，并在余额不足或授权失败时终止执行

