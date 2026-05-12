# Change: 支持按百分比撤出仓位

## Why
当前用户在 WebApp、MiniApp 或 Telegram 文字版点击停止任务时，系统默认撤出该任务的全部流动性并兑换回稳定币。对深度较浅或价格敏感的池子，这会带来较大的币价冲击。

用户需要在保留现有“停止任务=全撤”逻辑不变的前提下，能选择撤出任意百分比仓位，用更小的交易规模分批降仓。

## What Changes
- 新增“按百分比撤出仓位”能力：用户可输入 1-100 的撤出百分比。
- 保持现有停止任务、自动止损、出区间停止、再平衡、换仓的全撤行为不变；未显式传入百分比时仍按 100% 执行。
- WebApp、MiniApp 和 Telegram 文字版都提供部分撤仓入口；100% 等价于现有停止任务。
- 部分撤仓成功后，撤出的资产会兑换为稳定币，任务继续保留剩余流动性并保持可管理状态；仅当撤出 100% 时沿用现有停止任务状态流转。
- V3 `decreaseLiquidity` 和 V4 `DECREASE_LIQUIDITY` 按链上当前 liquidity 与用户百分比计算本次撤出 liquidity，交易完成后刷新任务剩余 `current_liquidity`。

## Impact
- Affected specs: `task-liquidity-exit`
- Affected code:
  - `backend/service/liquidity/liquidity_exit.go`
  - `backend/service/strategy/strategy_exit_retry.go`
  - `backend/service/web_server/task_stop.go`
  - `backend/service/web_server/task_withdraw_liquidity.go`
  - `backend/service/bot/task_callbacks.go`
  - `backend/service/bot/task_input_handlers.go`
  - `miniapp/src/lib/api.js`
  - `miniapp/src/App.jsx`
  - `miniapp/src/components/PositionCard.jsx`
  - `webapp/src/api.js`
  - `webapp/src/App.jsx`
  - `webapp/src/components/TaskActionMenu.jsx`
