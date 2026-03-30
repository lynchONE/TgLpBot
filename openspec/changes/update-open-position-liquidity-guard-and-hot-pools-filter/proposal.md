# Change: 开仓流动性分级校验与热门池子低流动性过滤

## Why
- 当前 `USDC` 入场路径会先按钱包 `USDT` 余额截断 `AmountUSDT`，再判断是否可直接使用现有 `USDC`，导致用户钱包里明明已有足够 `USDC`，实际却只按较低的 `USDT` 余额开仓。
- 开仓前虽然已经补上“当前池子 raw liquidity 不能为 0”的硬校验，但还缺少按美元流动性分级的风控规则，也缺少把风险提示前置到开仓弹窗的交互。
- 热门池子接口当前会把超低流动性的池子也返回给前端，导致用户在列表里仍能看到并尝试打开明显不安全的池子。

## What Changes
- 修正开仓金额解析逻辑：
  - 用户输入的 `amount` 视为目标上限/预算，而不是必须精确凑满的成交额。
  - 当钱包里已经有入场币（例如 `USDC`、`WBNB`）时，现有入场币余额必须先参与可执行金额计算，而不是先被 `USDT` 余额截断。
  - 当需要通过 `USDT` 兑换入场币时，系统最多执行一次正常入场兑换；兑换后实际到账多少入场币，就按多少开仓。
  - 系统不得为了把实际开仓金额补到用户输入的目标值，再额外执行一次“补缺口 swap”。
- 新增开仓流动性分级校验：
  - 当前池子链上 raw liquidity 为 `0` 时，必须直接禁止开仓。
  - 当前池子流动性美元值 `< 200` 时，必须直接禁止开仓并返回明确提示。
  - 当前池子流动性美元值 `>= 200` 且 `< 1000` 时，必须提示“开仓金额不能高于 200U”，并要求用户在同一开仓页面内修改金额、确认已知风险后才能继续。
- 旧的开仓安全阈值必须并入统一开仓前校验：
  - “池子当前价格 vs OKX 价格偏差”不再只在部分 Zap swap 路径中生效，而是对 `swap / no-swap`、`V3 / V4` 统一生效。
  - “最小池子流动性 USD”继续作为硬门槛，且优先级高于新的 `200U` 软硬分级规则。
  - 当数据库系统配置未设置时，后端必须回退读取部署环境中的安全阈值，而不是固定写死默认值。
- 开仓接口返回结构化的低流动性错误/警告信息，供 MiniApp 和 WebApp 同页消费，不再只能回一段普通错误文案。
- 热门池子接口必须过滤掉流动性美元值 `< 100` 的池子，响应里不再返回这些池子，前端也不再显示。

## Impact
- Affected specs:
  - `open-position-safety`
  - `pool-catalog`
- Affected code:
  - Backend:
    - `backend/service/liquidity/liquidity_enter.go`
    - `backend/service/web_server/open_position.go`
    - `backend/service/web_server/pools_catalog.go`
    - `backend/service/web_server/search_pools.go`（仅在需要补充流动性解析复用时调整）
    - `backend/service/web_server/pool_search_types.go`
  - MiniApp:
    - `miniapp/src/App.jsx`
    - `miniapp/src/lib/api.js`
    - `miniapp/src/components/StepProgressModal.jsx`
  - WebApp:
    - `webapp/src/components/OpenPositionModal.jsx`
    - `webapp/src/api.js`
- Backwards compatibility:
  - `POST /api/open_position` 保持原成功响应结构不变，仅新增可选的风险字段与请求确认字段。
  - 旧客户端若未消费结构化风险信息，后端仍会拒绝不满足阈值的请求，不会放宽安全约束。
