## 1. Implementation
- [x] 1.1 创建 OpenSpec 变更说明，明确保留 PoolM 最新 payload、仅同步 `top-fees/5`，并移除热门池子同步链路中的 DexScreener 补数。
- [x] 1.2 调整 `models.Pool` 与 MySQL 自动迁移，新增 PoolM 最新返回中的基础字段、源元数据字段，以及 `metricTrends`、`liquidityTicks`、`badges` 等 JSON 字段。
- [x] 1.3 重构 `backend/service/pool_sync`，仅调用 PoolM `top-fees/5` 并将返回数据直接写入 MySQL。
- [x] 1.4 适配 `/api/pools` 的读取与排序逻辑，改为直接读取 MySQL 中保存的 `top-fees/5` 快照，并返回最新的顶层索引字段。
- [x] 1.5 回归受影响的 pools 读取路径，保证 `search_pools` 等不在本次范围内的链路仅做兼容性调整，不混入热门池子同步逻辑。
- [x] 1.6 运行后端编译与测试，确认改动通过 `go test ./...`。
