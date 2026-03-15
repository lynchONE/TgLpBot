# Change: WebApp 支持 BSC 一键建池模块

## Why
- 当前系统的主交易入口是针对已有池子的 `open_position`/任务开仓流程，缺少对新池 permissionless 创建与首仓注入的独立支持。
- WebApp 已经有「热门池子」「聪明钱」两类池子视图，但用户如果想基于这些池子的币对去创建新池，还需要重复手填 token 信息、价格和协议参数，操作成本高且容易出错。
- BSC 场景下，用户明确需要优先支持 `Uniswap V3`、`Uniswap V4`、`PancakeSwap V3` 三类建池能力，因此需要先把协议范围、输入参数和交互边界收敛清楚，再进入实现。

## What Changes
- WebApp 新增独立的「创建池子」模块，面向 BSC 提供 `Uniswap V3`、`Uniswap V4`、`PancakeSwap V3` 建池入口。
- WebApp 的创建池子模块内支持把「热门池子」或「聪明钱」池子选作基础数据来源，用于预填 token 基础信息、参考价格和参考费率，减少手工输入。
- 后端新增独立的建池预校验与执行接口，交易流不复用 `open_position` / `StrategyTask`。
- 后端补齐 V3/V4 所需的链上绑定与参数推导逻辑，支持两种模式：
  - `create_only`：仅创建并初始化池子
  - `create_and_seed`：创建并初始化池子后立刻打入首笔流动性
- 一期对 `Uniswap V4` 限定为 `zero hooks + static fee`，不开放自定义 hooks、动态费率和 Infinity 类扩展。

## Impact
- Affected specs:
  - `pool-creation`
  - `webapp-pool-creation`
- Affected code:
  - Backend: `backend/base/blockchain/*`、`backend/service/liquidity/*`、`backend/service/web_server/*`、`backend/service/txexec/*`
  - WebApp: `webapp/src/App.jsx`、`webapp/src/api.js`、`webapp/src/components/*`、`webapp/src/styles.css`
- 风险与约束:
  - 涉及真实链上交易与授权，必须先做预校验再允许发交易
  - V4 建池参数面比 V3 更复杂，一期必须强约束输入面，避免把 hooks/tickSpacing 直接暴露给普通用户
  - 首仓默认不做自动换币，避免把「建池」和「路由换币」耦合成高风险流程
