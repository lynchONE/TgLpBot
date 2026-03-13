# Change: 优化 Smart Money 与 Hot Pools 的查询性能

## Why
- 当前 `smart_lp_events` 明细表同时承担写入、排行榜聚合、钱包 PnL 统计和事件趋势查询，导致 ClickHouse 读写竞争明显，接口容易出现 `context deadline exceeded`。
- `smart_money` 在请求内串行读取链上池子信息，`hot_pools` 每次都直接打 ClickHouse，缺少短期缓存，放大了接口延迟与上游依赖波动。

## What Changes
- 调整 `smart_lp_events` 的保留期为 2 天，并将排序键改为更贴合当前查询字段的 `(chain, pool_version, pool_id, wallet_address, ts, ...)`。
- 新增 Smart Money 小时级聚合表与物化视图，将 overview 相关查询切到聚合表，避免每次扫描明细事件。
- 为 Smart Money 池信息增加 Redis 缓存；池子元信息命中后直接复用，TTL 为 24 小时。
- 为 Hot Pools 接口增加 Redis 响应缓存；优先读 Redis，miss 时再查 ClickHouse 并回填，TTL 为 10 秒。

## Impact
- Affected specs: `analytics-performance`
- Affected code:
  - `backend/base/clickhouse/clickhouse.go`
  - `backend/service/smart_lp/smart_lp_service.go`
  - `backend/service/web_server/smart_money_overview.go`
  - `backend/service/web_server/hot_pools.go`
  - `backend/service/pool/*`
