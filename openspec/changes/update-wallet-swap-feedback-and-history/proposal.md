# Change: 优化钱包一键兑换完成反馈与兑换历史

## Why
- 当前 `webapp` 一键兑换提交后，页面缺少明确的“已完成”反馈，用户难以判断交易是否真的完成。
- 兑换成功后，前端执行态和按钮态清理不彻底，容易把上一次状态残留到下一次操作。
- 当前没有“当前钱包最近兑换记录”入口，用户无法在面板内快速回看刚完成的兑换。

## What Changes
- 后端钱包单币兑换执行接口在确认成功后，返回明确的完成态消息、完成时间、交易链接和实际到账数量。
- 后端复用现有 `transactions` 表记录钱包一键兑换结果，并新增当前钱包维度的兑换历史查询接口。
- 前端在兑换成功后清理执行态、刷新钱包余额与兑换历史，并展示成功卡片。
- 前端在一键兑换面板内新增“最近兑换”列表，展示当前钱包最近的兑换方向、数量、时间、状态和 Tx 链接。

## Impact
- Affected specs: `wallet-swap`
- Affected code:
  - `backend/service/web_server/wallet_swap_single_api.go`
  - `backend/service/web_server/wallet_swap_history_api.go`
  - `backend/service/web_server/compat_routes.go`
  - `backend/service/web_server/server.go`
  - `backend/service/liquidity/liquidity_wallet_swap.go`
  - `backend/service/liquidity/okx_swap.go`
  - `webapp/src/api.js`
  - `webapp/src/components/SwapPanel.jsx`
  - `webapp/src/styles.css`
