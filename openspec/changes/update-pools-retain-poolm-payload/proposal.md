# Change: 调整 Pools 同步并保留 top-fees/5 最新返回

## Why
- 当前 `pools` 表结构沿用了旧的分析字段口径，无法完整保留 PoolM 最新 `top-fees/5` 返回中的高级字段，例如 `metricTrends`、`liquidityTicks`、`badges`、`priced_token_address` 等。
- 当前 `pool_sync` 会把 PoolM 和 DexScreener 两套数据混在一起补数，导致 MySQL 中部分字段来源不一致、语义不清，而且会留下大量 `0` 或空值。
- 用户已明确要求 `pools` 只保留 `top-fees/5` 的最新完整返回，并去掉 DexScreener 补数据链路。

## What Changes
- 调整 `pools` 表结构，围绕 PoolM `top-fees/5` 最新返回重建字段集合：
  - 保留共享的池子当前态字段
  - 保留响应级索引字段
  - 保留高级数组/徽章等复杂字段
  - 保留当前 5 分钟窗口的统计结果
- 重构 `pool_sync`，仅同步 PoolM `top-fees/5`，并将这份最新完整快照写入 MySQL。
- 删除 `pool_sync` 中对 DexScreener 的补数依赖，`/api/pools` 及相关读取链路只使用 MySQL 中已保存的 PoolM 数据。
- 本次不改 `search_pools` 当前基于 DexScreener 的搜索链路；该接口不属于“热门池子/`pools` 表同步”范围。

## Impact
- Affected specs: `pool-catalog`
- Affected code:
  - `backend/base/models/pool.go`
  - `backend/base/database/mysql.go`
  - `backend/service/pool_sync/client.go`
  - `backend/service/pool_sync/service.go`
  - `backend/service/web_server/pools_catalog.go`
  - `backend/service/web_server/pool_search_types.go`
  - `backend/service/web_server/search_pools.go`
  - `backend/service/web_server/server.go`
