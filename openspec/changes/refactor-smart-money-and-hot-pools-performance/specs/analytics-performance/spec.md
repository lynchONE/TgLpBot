## ADDED Requirements

### Requirement: Smart Money overview 使用预聚合数据
后端 MUST 为 Smart Money overview 提供基于预聚合数据的查询路径，而不是每次直接扫描 `smart_lp_events` 明细表。

#### Scenario: 查询 Smart Money overview
- **WHEN** 客户端请求 `GET /api/smart_money_overview`
- **THEN** 后端从 Smart Money 预聚合表读取池子、钱包现金流、钱包快照或事件趋势所需数据
- **AND** 不再依赖对 `smart_lp_events` 明细表做全量窗口现算

### Requirement: Smart Money 池信息 Redis 缓存
后端 MUST 将 Smart Money 池子链上元信息缓存到 Redis 中。

缓存 key MUST 包含 `chain`、`pool_version` 和 `pool_id`。

缓存 TTL MUST 为 24 小时。

#### Scenario: 池信息缓存命中
- **GIVEN** Redis 中已存在某个池子的元信息缓存
- **WHEN** Smart Money overview 需要该池子的展示信息
- **THEN** 后端直接读取 Redis 缓存
- **AND** 不再在当前请求中重复执行链上 RPC

#### Scenario: 池信息缓存未命中
- **GIVEN** Redis 中不存在某个池子的元信息缓存
- **WHEN** Smart Money overview 需要该池子的展示信息
- **THEN** 后端执行链上读取
- **AND** 将结果写入 Redis，TTL 为 24 小时

### Requirement: Hot Pools Redis 响应缓存
后端 MUST 为 Hot Pools 查询结果提供 Redis 短期缓存。

缓存 key MUST 包含会影响结果集的请求参数。

缓存 TTL MUST 为 10 秒。

#### Scenario: Hot Pools 缓存命中
- **GIVEN** Redis 中存在当前请求参数对应的 Hot Pools 响应缓存
- **WHEN** 客户端请求 Hot Pools 数据
- **THEN** 后端直接返回缓存内容
- **AND** 不再访问 ClickHouse

#### Scenario: Hot Pools 缓存未命中
- **GIVEN** Redis 中不存在当前请求参数对应的 Hot Pools 响应缓存
- **WHEN** 客户端请求 Hot Pools 数据
- **THEN** 后端查询 ClickHouse
- **AND** 将最终 JSON 响应写入 Redis，TTL 为 10 秒

### Requirement: SmartLP 明细表仅保留近期数据
ClickHouse 中的 `smart_lp_events` 明细表 MUST 仅保留最近 2 天数据，并使用贴合当前查询路径的排序键。

#### Scenario: ClickHouse 启动迁移
- **WHEN** 服务启动并执行 ClickHouse schema/migration
- **THEN** `smart_lp_events` 使用按 `chain/pool/wallet/ts` 优化的排序键
- **AND** 表的 TTL 为 2 天
