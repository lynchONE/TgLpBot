## ADDED Requirements

### Requirement: Pools 同步必须保留最新 PoolM 返回字段
系统 MUST 将 PoolM `top-fees/5` 返回中的最新池子字段持久化到 MySQL `pools` 表中，并且同步链路 MUST 不再依赖 DexScreener 为这些池子补充目录字段。

#### Scenario: 同步单个池子的最新载荷
- **WHEN** 定时同步从 PoolM 拉到某个池子的最新返回
- **THEN** `pools` 表保存该池子的共享当前态字段、token 基础信息、价格/流动性状态以及 `metricTrends`、`liquidityTicks`、`badges` 等复杂字段
- **AND** 这些字段直接来源于 PoolM 返回，而不是 DexScreener 补数

### Requirement: Pools 同步必须只抓取 top-fees/5
系统 MUST 在池子目录同步时仅抓取 PoolM `top-fees/5`，并将这份 5 分钟窗口结果作为 `pools` 表中的热门池子快照。

#### Scenario: 执行一次目录同步
- **WHEN** `pool_sync` 执行一次正常同步
- **THEN** 同步过程只请求 `top-fees/5`
- **AND** `pools` 表中保存该 5 分钟窗口对应的 `transaction_count`、`total_fees`、`total_volume` 及相关派生展示数据

### Requirement: Pools 读取接口必须返回已保存的 top-fees/5 快照
后端 `GET /api/pools` MUST 直接从 MySQL `pools` 表中读取已保存的 `top-fees/5` 快照并返回，而不是重新依赖 DexScreener 或其它时间窗口数据。

#### Scenario: 客户端请求池子榜单
- **WHEN** 客户端调用 `GET /api/pools`
- **THEN** 后端使用 MySQL 中保存的 `top-fees/5` 快照完成排序和返回
- **AND** 响应中的池子高级字段与当前态字段来自 `pools` 表，而不是请求时临时补数
