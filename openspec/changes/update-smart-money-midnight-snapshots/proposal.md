# Change: 聪明钱资产改用独立 0 点快照

## Why
- 当前聪明钱资产页把每日聚合快照同时当作历史资产点、排行榜基准和日历盈亏来源，服务重启后昨日聚合可能重跑，并把原本 0 点附近的基准覆盖成当前资产，导致实时榜差值变成 0。
- 聪明钱资产需要一个不会被日内刷新覆盖的 0 点资产快照来源，才能稳定支持“当前最近一次资产快照 vs 当日 0 点资产快照”的排行榜，以及“每日 0 点快照 vs 前一日 0 点快照”的盈亏日历。

## What Changes
- 后端新增独立的聪明钱 0 点资产快照表，按钱包、链、快照日唯一保存资产拆分、总资产、活跃池等数据。
- 每日 0 点后写入当日 0 点资产快照，快照一旦存在不得被普通聚合或服务重启覆盖。
- 0 点资产快照至少保留 31 天数据，用于资产趋势、排行榜基准和钱包盈亏日历。
- 聪明钱实时资产排行榜改为使用最近一次实时资产快照与当日 0 点资产快照计算差值。
- 聪明钱盈亏日历改为使用当日 0 点资产快照与前一日 0 点资产快照计算当日资产变化。

## Impact
- Affected specs: `smart-money-assets`
- Affected code:
  - `backend/base/models/asset_management.go`
  - `backend/base/database/mysql.go`
  - `backend/service/assets/service.go`
  - `backend/service/assets/smart_money.go`
  - `backend/service/assets/smart_money_test.go`
  - `backend/service/web_server/assets.go`
  - `webapp/src/components/SmartMoneyAssetsPanel.jsx`
  - `miniapp/src/components/SmartMoneyAssetsPage.jsx`
