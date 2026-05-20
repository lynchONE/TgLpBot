# Change: 钱包一键兑换支持限价单自动兑换

## Why
- 当前一键兑换只能由用户手动确认即时兑换，无法在目标到账金额或目标价格达到后自动执行。
- 用户希望提前设置卖出代币、买入代币、数量与触发价格，由系统在报价达到条件后自动兑换。

## What Changes
- 新增钱包限价单模型，保存用户、链、钱包、卖出代币、买入代币、卖出数量、目标价格/目标到账金额、滑点、provider 偏好与订单状态。
- 新增限价单 API，支持创建、列表、详情、取消，并复用一键兑换模块权限校验。
- 新增后台限价单 worker，定时获取报价，达到触发条件后锁定订单并自动执行兑换。
- 自动执行时复用现有 `wallet_swap_single` provider 执行能力，记录交易结果、失败原因和最终到账金额。
- `webapp` 一键兑换页面新增限价单模式，允许用户设置目标价格或目标到账金额，查看订单状态并取消未成交订单。

## Impact
- Affected specs: `wallet-swap`
- Affected code:
  - `backend/base/models/`
  - `backend/base/database/mysql.go`
  - `backend/main.go`
  - `backend/service/liquidity/`
  - `backend/service/web_server/`
  - `webapp/src/api.js`
  - `webapp/src/components/SwapPanel.jsx`
