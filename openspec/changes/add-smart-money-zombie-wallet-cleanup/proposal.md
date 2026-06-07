# Change: 增加聪明钱僵尸钱包筛选与选择删除

## Why
- 聪明钱钱包长期无 LP 开仓或撤仓活动时，继续保留会稀释钱包视图、活动统计和排行榜的有效性。
- 用户希望在钱包视图中手动找出这些僵尸钱包，并能选择保留其中一部分。
- 删除钱包时需要同步删除该钱包的聪明钱历史数据，避免后续统计、特别关注、跟单配置或详情页残留旧数据。

## What Changes
- 在聪明钱钱包视图增加“查找僵尸钱包”按钮。
- 后端提供候选查询能力：默认找出最近 30 天没有 LP add/remove 活动的 active 聪明钱钱包，并返回最后活动时间、来源、标签、历史计数等确认信息。
- 前端展示可勾选候选列表，默认选中全部候选，用户可以取消选择不想删除的钱包。
- 后端提供批量删除能力：只删除用户确认选择的钱包，并同步删除该钱包相关的聪明钱历史数据。
- 现有单个钱包删除也统一为硬删除语义：删除 `monitored_wallets` 记录以及该钱包的 LP 事件、仓位、active state、转账事件、每日快照、live state、LP 日统计、特别关注和跟单引用数据。
- 删除完成后刷新钱包视图、排行榜相关缓存或列表数据，使被删除钱包不再参与 active 钱包展示和统计。

## Impact
- Affected specs: `smart-money-wallet-view`
- Affected code:
  - `backend/service/web_server/smart_money.go`
  - `backend/service/smart_money/repository.go`
  - `backend/service/assets/smart_money.go`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `webapp/src/smartMoneyApi.js`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `miniapp/src/lib/smartMoneyApi.js`
- Data impact: 删除目标钱包的聪明钱监控记录与历史数据；不操作用户自有钱包、私钥、策略任务资金或链上资金。
