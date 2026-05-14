# Change: 自动跟单支持多钱包组合触发

## Why
当前自动跟单配置只能监听单个目标钱包，用户需要分别创建多条配置，无法表达“监控一组钱包，任意一个开仓就跟”或“同一池子/同一仓位信号必须被多个钱包同时确认后才跟”的策略。对真实资金执行来说，多钱包确认可以减少单钱包误触发，任意钱包触发则适合强信号钱包组的快速跟随。

## What Changes
- 自动跟单配置从单目标钱包扩展为目标钱包组，兼容已有单钱包配置。
- 新增触发规则：
  - `any`：目标钱包组中任意一个钱包发生开仓事件即触发跟单。
  - `threshold`：同一池子、协议和价格区间在指定时间窗口内，至少 `N` 个不同目标钱包发生开仓事件才触发跟单。
- 自动跟单任务必须记录触发规则、触发钱包列表和主触发事件，便于前端追踪原因。
- 前端自动跟单表单支持维护多个钱包地址，并配置触发模式、阈值数量和统计窗口。

## Impact
- Affected specs:
  - `miniapp-smart-money`
  - `smart-money-follow`
- Affected code:
  - `backend/base/models/smart_money_follow.go`
  - `backend/base/database/mysql.go`
  - `backend/service/smart_money_follow/*`
  - `backend/service/web_server/smart_money_auto_follow.go`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `webapp/src/smartMoneyApi.js`
  - `miniapp/src/lib/smartMoneyApi.js`

## Compatibility
- 已有单钱包配置迁移为包含一个钱包的目标钱包组，默认触发规则为 `any`。
- 旧字段 `target_wallet_address` 在兼容期继续返回，用于展示主钱包或第一个目标钱包；新前端应优先使用钱包组字段。
- 新规则默认关闭历史回放：启用或修改配置后，只处理游标之后的新事件。
