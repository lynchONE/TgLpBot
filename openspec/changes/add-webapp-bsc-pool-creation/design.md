## Context
- 当前代码里的开仓主路径围绕 `open_position` 与 `StrategyTask` 展开，默认假设目标池已经存在，且主要以稳定币入场、后端代做换币与注流动性为核心。
- WebApp 已经具备「热门池子」「聪明钱」两类池子数据，并在前端状态里持有 token 地址、symbol、pool version、pool id/address 等基础信息，具备天然的预填来源。
- 现有链上绑定仍偏向读操作和已有仓位管理：
  - V3 PositionManager 还没有 `createAndInitializePoolIfNecessary`、`mint`
  - V4 PositionManager 目前只有 `modifyLiquidities`/读仓位等最小面，缺少建池初始化和多调用封装
- 该功能直接影响真实资金和授权，因此需要把参数推导、池子是否已存在、协议限制检查放在后端统一收口，而不是让前端自由拼装链上参数。

## Goals / Non-Goals
- Goals:
  - 在 BSC 上支持 `Uniswap V3`、`Uniswap V4`、`PancakeSwap V3` 的新池创建
  - 提供独立于现有开仓任务的建池与首注交易流
- 让用户可以在建池模块中选择「热门池子」「聪明钱」池子作为基础数据来源并预填 token 基础信息
  - 默认最小化输入，只暴露协议、价格、费率预设、模式、首注数量等必要参数
  - 在成功态返回 `pool address/poolId`、`tokenId`、`tx hash`
- Non-Goals:
  - 一期不支持 V4 自定义 hooks、动态费率、Pancake Infinity
  - 一期不自动把新池或首仓仓位导入监控、策略任务或聪明钱追踪
  - 一期不支持单币输入后自动换币打首仓
  - 一期不扩展到 Base 等其他链

## Decisions
### Decision: 建池走独立交易流，不复用 open_position
- 建池接口与执行逻辑独立于 `open_position`、`StrategyTask`、`EnterTaskFromUSDTWithOptions`。
- 执行时仍复用现有每钱包串行交易执行器与授权能力，避免并发交易冲突。
- 返回值以「建池结果」为中心，而不是任务对象。
- Alternatives considered:
  - 复用 `open_position`：会把“已有池开仓”和“新池创建+首仓”混在一套任务模型里，导致参数语义、成功态、后续持仓归属都变得不清晰，因此不采用。

### Decision: 使用 preview + execute 两段式接口
- 新增 `preview` 接口，用于统一完成以下派生与校验：
  - token metadata 读取与 token0/token1 自动排序
  - 池子是否已存在检查
  - 初始价格到 `sqrtPriceX96` 的换算
  - V3 tickSpacing / V4 fee+ticks 预设解析
  - 默认首仓范围推导
  - 授权与余额风险提示
- 新增 `execute` 接口，基于确认后的参数执行链上交易，并在执行前重新做关键校验。
- Alternatives considered:
  - 只保留单一 execute 接口：输入错误会直接在链上阶段暴露，用户难以理解失败原因，也不利于做“池子已存在”与参数摘要展示，因此不采用。

### Decision: WebApp 默认最小表单，进阶参数折叠
- 默认可见参数：
  - 钱包
  - 目标协议
  - Token A / Token B
  - 初始价格
  - 费率或预设模板
  - 模式（仅建池 / 建池并首注）
  - 首注数量（仅 `create_and_seed`）
- 默认隐藏在“高级选项”中的参数：
  - 自定义价格区间
  - deadline
  - slippage
  - V4 的 tickSpacing 明细（通过模板驱动，而不是直接裸露输入）
- 首仓默认使用 `full_range`，降低首次输入负担；高级选项可切换到自定义区间。

### Decision: 预填来源在建池模块内选择
- 用户进入建池模块后，可以从「热门池子」或「聪明钱」池子数据集中选择一个来源，前端把以下信息带入表单：
  - token 地址、symbol、decimals（若已有）
  - 参考价格
  - 来源池协议与费率提示
  - 来源池标识，用于在 UI 上说明“本次建池基础信息来自哪个池子”
- 用户仍可在建池模块内切换目标协议，不把来源池协议锁死。
- 手工入口依然保留，允许用户跳过来源选择并直接输入 token 地址进行建池。

### Decision: 协议执行采用分协议封装
- `Uniswap V3` / `PancakeSwap V3`
  - 使用各自 factory 检查同 pair + fee 的池子是否已存在
  - 通过 PositionManager `createAndInitializePoolIfNecessary` 创建并初始化池子
  - 在 `create_and_seed` 模式下先完成 token 授权，再调用 `mint` 打首仓
  - 返回 pool address、NFT tokenId、tx hash
- `Uniswap V4`
  - 一期仅允许 `zero hooks + static fee`
  - `create_only` 负责构造 `PoolKey` 并初始化池子，返回 poolId
  - `create_and_seed` 需要在初始化后继续执行首仓动作，并返回 tokenId
  - poolId 使用后端统一计算与校验，不让前端自己拼接

### Decision: 首仓不做自动换币，要求双币余额充足
- `create_and_seed` 由用户直接输入两侧 token 数量。
- 后端只负责余额校验、授权与 mint/modify liquidity，不引入路由换币。
- Alternatives considered:
  - 允许单边输入并自动 swap：会把路由、滑点和 MEV 风险引入建池首仓场景，一期复杂度与资金风险都过高，因此不采用。

## Risks / Trade-offs
- V4 参数面天然大于 V3，若不限制 hooks 和 fee/tickSpacing 组合，会显著抬高实现与测试成本。
  - Mitigation: 一期只开放零 hooks 和有限模板。
- 用户输入的初始价格如果方向理解错误，会直接导致错误初始化价格。
  - Mitigation: preview 返回归一化后的 token 顺序、价格摘要和结果卡片，要求用户确认后再执行。
- 建池与首仓一旦成功，后续是否纳入监控仍需要额外动作。
  - Mitigation: 成功态先明确返回识别信息，后续再单独规划“导入监控/导入策略”。

## Migration Plan
1. 后端先补齐协议绑定、参数推导与 preview/execute API。
2. WebApp 增加独立的「创建池子」模块与成功结果卡。
3. 在建池模块中接入「热门池子」「聪明钱」来源选择器。
4. 仅当链为 `bsc` 且目标协议部署地址配置完整时，前端才展示对应协议选项。

## Open Questions
- 无。该提案默认以 `create_and_seed` 为推荐模式，但保留 `create_only` 以兼容只想抢先初始化池子的用户。
