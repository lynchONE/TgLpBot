# Change: 调整 BSC 建池能力以支持 V4 任意费率和单币建池

## Why
- 当前创建池子流程把 `Uniswap V4` 限制为预设静态费率、`full_range` 和双币输入，和实际使用需求不一致。
- 当前 WebApp 不能在输入一侧金额后按当前价格推导另一侧金额，也不能在建池首仓时自动换币组成目标比例。
- `Uniswap V3` 与 `PancakeSwap V3` 的建池费率需要按协议固定档位收敛，避免前端、后端和链上校验口径不一致。

## What Changes
- 明确各协议的费率策略：`Uniswap V3` 仅支持 `100/500/3000/10000`，`PancakeSwap V3` 仅支持 `100/500/2500/10000`，`Uniswap V4` 支持任意静态 `fee_tier`。
- 为 `Uniswap V4` 新增 `tick_spacing` 录入/预填能力，并继续保持 `zero hooks` 约束；不支持动态费率和自定义 hooks。
- 扩展 `create_pool_preview` / `create_pool_execute`，支持 `full_range` 与 `custom_range`；自定义区间由前端提交价格区间，后端负责归一化为合法 ticks。
- 建池首仓支持两种金额模式：双币精确输入、单币输入后自动换币配比；preview 需要返回镜像金额估算、估算来源和风险提示。
- WebApp 创建池子面板补齐协议费率、V4 任意费率、区间模式、单币建池、来源池预填与结果摘要交互。

## Impact
- Affected specs:
  - `pool-creation`
  - `webapp-pool-creation`
- Affected code:
  - Backend: `backend/service/web_server/create_pool.go`, `backend/service/liquidity/*`, `backend/service/pool/*`, `backend/base/blockchain/*`
  - WebApp: `webapp/src/components/CreatePoolPanel.jsx`, `webapp/src/api.js`, `webapp/src/styles.css`
- 风险与约束:
  - 单币自动换币会引入 OKX 报价、滑点与价格漂移风险，执行前必须重新校验报价与池子存在性。
  - `Uniswap V4` 任意费率不再能仅靠 `fee_tier` 推导 `tick_spacing`，必须由来源池预填或由用户显式输入。
  - 自定义区间与单币建池叠加后，preview 只能返回估算值，最终执行结果仍需以后端实时计算为准。
