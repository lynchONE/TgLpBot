# Change: 自动跟单支持选择执行钱包

## Why
当前自动跟单配置只展示被跟随的目标钱包，实际开仓执行钱包隐式使用用户默认钱包。多钱包用户无法确认或指定资金从哪个钱包执行跟单。

## What Changes
- 自动跟单配置新增执行钱包字段，保存用户选择的钱包 ID 与地址。
- 自动跟单开仓任务创建时使用配置的执行钱包写入 `StrategyTask.wallet_id` 和 `StrategyTask.wallet_address`。
- WebApp 与 MiniApp 自动跟单表单增加执行钱包选择，并在配置卡片和最近任务中展示。
- 历史未设置执行钱包的配置继续按默认钱包解析，用于兼容旧数据。

## Impact
- Affected specs: `smart-money-follow`, `miniapp-smart-money`
- Affected code:
  - `backend/base/models/smart_money_follow.go`
  - `backend/base/database/mysql.go`
  - `backend/service/smart_money_follow/service.go`
  - `backend/service/web_server/smart_money_auto_follow.go`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `miniapp/src/components/SmartMoneyPage.jsx`
