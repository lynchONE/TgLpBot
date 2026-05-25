# Change: 新增特别关注操作记录模块

## Why
当前聪明钱模块已经支持把钱包加入“特别关注”，但用户只能收到开仓提醒或在钱包详情中单独查看记录，无法在 MiniApp 和 WebApp 中集中查看这些特别关注对象最近的 LP 加仓和撤仓动态。

## What Changes
- 在 MiniApp 和 WebApp 聪明钱中新增“特别关注”模块，展示用户已关注钱包列表，并允许点击查看某个钱包最近的 LP 操作记录。
- 新增或扩展后端读取接口，按当前 Telegram 用户的特别关注钱包范围查询 `sm_lp_events`，仅返回 `add/remove` 记录并包含钱包、池子、交易对、金额、区间、时间和交易哈希等展示字段。
- 前端复用现有特别关注钱包状态和聪明钱事件展示能力，支持空状态、加载状态、分页或加载更多。

## Impact
- Affected specs: `miniapp-smart-money`, `webapp-smart-money`
- Affected code:
  - `backend/service/web_server/smart_money.go`
  - `backend/service/smart_money/repository.go`
  - `backend/service/smart_money/watch_activity_test.go`
  - `backend/service/web_server/smart_money_watch_open_alert.go`
  - `backend/base/database/mysql.go`
  - `miniapp/src/lib/smartMoneyApi.js`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `webapp/src/smartMoneyApi.js`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `webapp/src/styles.css`
