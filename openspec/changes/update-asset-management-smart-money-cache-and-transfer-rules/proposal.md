# 变更：优化资产管理聪明钱缓存、分页与转账日盈亏口径

## Why
- 当前资产管理里的“聪明钱资产”页在请求钱包总览时，会为当前页钱包逐个读取实时资产；一旦余额入口没有稳定复用缓存，页面响应会明显变慢，同时放大 RPC 压力。
- 当前排行榜展示的是按快照日计算的结果，天然适合复用 Redis 日缓存，但如果仍在请求时动态组装，会产生重复计算。
- 当前聪明钱日历与排行榜中的盈亏值仍可能被普通转入或转出干扰，容易把资金划转误记为盈利或亏损。
- 现有钱包实时资产缓存时长偏长，不利于页面余额展示；需要把聪明钱包余额相关入口统一收敛到同一份 5 分钟 Redis 缓存，在可接受的新鲜度下减少重复查链。

## What Changes
- 后端：
  - 为聪明钱排行榜增加按“快照日 + 指标”组织的 Redis 日缓存，并在每日快照任务完成后刷新当天缓存、清理前一日缓存。
  - 为聪明钱钱包实时资产结果提供按钱包组织的 Redis 缓存，TTL 固定为 5 分钟；钱包总览、单钱包详情、列表余额 enrich 等所有余额消费入口都统一复用该缓存。
  - 保留 `force_refresh` 绕过缓存的能力，便于手动刷新。
  - 调整聪明钱盈亏口径：当某钱包在某自然日检测到普通转入或转出时，该日排行榜盈亏与钱包盈亏日历都不得再使用资产差额计入盈亏。
  - 钱包盈亏日历优先使用已聚合的 LP 日统计结果表达盈亏，并继续返回转账标识用于前端展示。
- 前端：
  - MiniApp 与 webapp 的聪明钱钱包总览继续使用服务端分页请求，不再一次性加载全量钱包详情。
  - 排行榜条目支持点击后直接进入对应钱包详情视图。
  - 盈亏日历在转账日继续展示“转入 / 转出”标识，并与新的盈亏口径保持一致。

## Impact
- Affected specs:
  - `admin-smart-money-analytics`
  - `analytics-performance`
- Affected code:
  - `backend/service/assets/smart_money.go`
  - `backend/service/web_server/assets.go`
  - `backend/service/web_server/smart_money.go`
  - `backend/base/database/redis.go`
  - `miniapp/src/lib/api.js`
  - `miniapp/src/components/AssetManagementPage.jsx`
  - `webapp/src/api.js`
  - `webapp/src/components/AssetManagementPanel.jsx`
