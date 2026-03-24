# 变更：优化资产管理聪明钱缓存、分页与转账日盈亏口径

## Why
- 当前资产管理里的“聪明钱资产”页在请求钱包总览时，会为所有监控钱包逐个实时查链读取余额与代币持仓；前端虽然只展示 10 条分页数据，但后端仍然把全量钱包都查完，导致接口响应明显偏慢。
- 当前排行榜展示的是“昨天相对前天”的结果，数据天然按日稳定；但接口仍在请求时动态组装排行榜，缺少按快照日复用的 Redis 缓存。
- 当前排行榜条目不能直接进入钱包详情，管理员需要先切回钱包总览再找目标钱包，交互链路偏长。
- 当前聪明钱日历与排行榜中的盈亏值仍会直接使用“总资产差额”推算；当钱包当天发生普通转入或转出时，会把资金划转误记为盈利或亏损，虽然已有转账标识，但口径仍不正确。

## What Changes
- 后端：
  - 为聪明钱排行榜增加按“快照日 + 指标”组织的 Redis 日缓存，并在每日快照任务完成后刷新当天缓存、清理前一日缓存。
  - 为聪明钱钱包总览增加服务端分页能力，并把实时钱包资产结果按钱包缓存 30 分钟；总览接口只加载当前页所需的钱包详情，翻页时再请求下一页。
  - 调整聪明钱盈亏口径：当某钱包在某自然日检测到普通转入或转出时，该日排行榜盈亏与钱包盈亏日历都不得再使用资产差额计入盈亏。
  - 钱包盈亏日历改为优先使用已聚合的 LP 日统计表达盈亏，并继续返回转账标识用于前端展示。
- 前端：
  - MiniApp 与 webapp 的聪明钱钱包总览改为服务端分页请求，不再一次性加载全量钱包详情。
  - 排行榜条目支持点击后直接进入对应钱包详情视图。
  - 盈亏日历在转账日继续展示“转入/转出”标识，并与新的盈亏口径保持一致。

## Impact
- Affected specs:
  - `admin-smart-money-analytics`
  - `analytics-performance`
- Affected code:
  - `backend/service/assets/smart_money.go`
  - `backend/service/web_server/assets.go`
  - `backend/base/database/redis.go`
  - `miniapp/src/lib/api.js`
  - `miniapp/src/components/AssetManagementPage.jsx`
  - `webapp/src/api.js`
  - `webapp/src/components/AssetManagementPanel.jsx`
