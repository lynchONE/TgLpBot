## Context
- 当前 `create_pool` 已经具备独立的 `preview / execute` 接口，但 `range_mode` 只有 `full_range`，`create_and_seed` 只接受双币输入。
- `Uniswap V3` / `PancakeSwap V3` 当前通过 factory 的 `feeAmountTickSpacing` 校验 fee，天然适合固定档位能力。
- `Uniswap V4` 当前仅支持 `100/500/3000/10000` 四档费率，并通过 `StandardTickSpacingFromFee` 推导 `tickSpacing`；这无法覆盖任意静态费率。
- 代码里已经具备 `ZapInV3` / `ZapInV4` ABI 与 OKX swap calldata 组装能力，可以复用到“单币自动换币后建池”的执行链路。

## Goals / Non-Goals
- Goals:
  - 统一 `Uniswap V3`、`PancakeSwap V3` 固定费率与 `Uniswap V4` 任意静态费率的校验口径
  - 支持 `full_range` 与 `custom_range`
  - 支持输入单侧金额后返回另一侧镜像金额估算
  - 支持 `create_and_seed` 的单币自动换币配比建池
  - 保留来源池预填与 `preview + execute` 两段式交互
- Non-Goals:
  - 不支持 `Uniswap V4` 动态费率和非零 hooks
  - 不支持把建池后的仓位自动导入监控或策略任务
  - 不支持原生 BNB 直充；仍以 ERC20 输入为前提

## Decisions
### Decision: 保留 preview + execute，两段都必须理解“区间 + 金额模式”
- `preview` 统一负责 token 顺序归一化、fee / tickSpacing 校验、价格推导、区间归一化、金额估算和池子存在性校验。
- `execute` 在执行前重新做关键校验，避免 preview 之后价格漂移、报价失效或池子被他人抢先初始化。
- Alternatives considered:
  - 只在前端做镜像金额推导：会造成前端展示、后端真实执行和 OKX 报价口径分裂，因此不采用。

### Decision: V3 / Pancake V3 固定费率，V4 任意静态费率必须显式处理 tickSpacing
- `Uniswap V3` 仅接受 `100/500/3000/10000`。
- `PancakeSwap V3` 仅接受 `100/500/2500/10000`。
- `Uniswap V4` 接受任意静态 `fee_tier`，但由于 fee 与 `tickSpacing` 不再是一一映射，接口需要新增 `tick_spacing`。
- `tick_spacing` 的来源规则：
  - 来源池预填时，优先直接带入来源池的 `tickSpacing`
  - 手动创建 `Uniswap V4` 池子时，前端展示推荐值并允许改写，后端只接受合法的正整数 `tick_spacing`
- Alternatives considered:
  - 继续通过固定 `fee -> tickSpacing` 映射支持 V4 任意费率：会生成错误的 `PoolKey`，因此不采用。

### Decision: 自定义区间以“价格区间输入，后端归一化为 ticks”为主
- API 新增 `range_mode=custom_range` 以及 `min_price` / `max_price`，价格语义统一为 `1 TokenA = X TokenB`。
- 后端根据归一化后的 `token0/token1`、`tick_spacing` 和价格方向，把区间转换为按 `tick_spacing` 对齐的 `tickLower` / `tickUpper`。
- preview 返回最终 `tickLower` / `tickUpper` 和归一化后的价格摘要，前端只展示，不自行拼装 ticks。
- Alternatives considered:
  - 直接让前端提交 ticks：普通用户理解成本高，也容易产生未按 `tickSpacing` 对齐的无效输入，因此不采用。

### Decision: 金额模式拆成“双币精确输入”和“单币自动换币”
- API 新增 `amount_mode`：
  - `dual_exact`: 用户显式提供 `amount_a` 与 `amount_b`
  - `single_auto_swap`: 用户只提供一侧 token 和数量，由后端基于当前价格、目标区间和 OKX 路由生成换币与建池参数
- preview 阶段允许用户只输入一侧金额，此时后端返回镜像金额估算；只有当用户显式选择 `single_auto_swap` 时，execute 才允许自动换币。
- Alternatives considered:
  - 只做单侧金额展示，不支持执行：不能满足“单币直接自己兑换组成比例建池”，因此不采用。

### Decision: 单币自动换币尽量复用现有 zap 与 OKX 路由
- `Uniswap V3` / `PancakeSwap V3` 的单币首仓复用 `ZapInV3` 能力；新池初始化后，通过 `createAndInitializePoolIfNecessary + zapInV3` 完成首仓。
- `Uniswap V4` 的单币首仓复用 `ZapInV4` 能力；新池初始化后，通过 `initializePool + zapInV4` 完成首仓。
- 双币输入时：
  - V3 / Pancake V3 继续使用 `mint` 直打
  - V4 继续使用 `zapInV4`，但 swap 参数为空
- Alternatives considered:
  - 所有协议统一切到 zap：可行，但会放大现有双币链路的改动面；当前优先补单币 path，同时保留双币 path 的稳定性。

## Risks / Trade-offs
- 单币自动换币依赖 OKX 报价，报价过期或链上价格快速变化时，execute 可能失败。
  - Mitigation: preview 返回风险提示；execute 重新拉取 swap calldata，并对 `minAmountOut`、`maxSwapLossBps` 做保护。
- `Uniswap V4` 任意 fee 需要用户理解 `tickSpacing`，否则容易创建出与预期不符的池子。
  - Mitigation: 默认优先从来源池预填；手动模式只在高级设置中暴露 `tick_spacing`，并给出推荐值与说明。
- 自定义区间与单币换币叠加后，preview 估算值与最终落地值可能存在偏差。
  - Mitigation: UI 明确区分“估算展示”和“最终执行”，并在 execute 前强制再次校验。

## Migration Plan
1. 扩展后端 `create_pool` 请求/响应结构，补齐 `tick_spacing`、`amount_mode`、`min_price`、`max_price` 等字段。
2. 完成后端 fee / range / 单币 quote 与执行链路，并补齐单元测试。
3. 更新 WebApp 创建池子面板与 API 调用，加入 V4 任意 fee、自定义区间、单币镜像金额与自动换币选项。
4. 完成 `webapp` 构建验证，并在 BSC 环境手工验证三类协议的双币 / 单币建池流程。

## Open Questions
- 暂无。当前变更默认把 `single_auto_swap` 作为 `create_and_seed` 的可选模式，而不是默认模式。
