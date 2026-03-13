## Context
- `smart_money_overview` 当前直接读取 `smart_lp_events` 明细表，窗口涉及 2h/24h，并在请求内额外执行池子链上读取与价格查询。
- `smart_lp_events` 现有排序键偏向 `(tx_hash, log_index)`，不利于按 `chain/pool/wallet/ts` 过滤的读路径。
- `hot_pools` 当前只依赖 ClickHouse，且响应内容对短时间内的大量重复请求高度相同，适合做秒级缓存。

## Goals / Non-Goals
- Goals:
  - 降低 Smart Money overview 的 ClickHouse 扫描量。
  - 降低 Smart Money 请求内链上 RPC 次数。
  - 降低 Hot Pools 重复请求对 ClickHouse 的压力。
  - 将 `smart_lp_events` 的保留期收缩到当前业务认可的 2 天。
- Non-Goals:
  - 不修改前端接口协议。
  - 不引入新的外部存储组件，继续复用现有 Redis 与 ClickHouse。

## Decisions
- Decision: 为 `smart_lp_events` 增加一次性重建迁移逻辑
  - 使用新表 + 回填最近 2 天数据 + rename swap 的方式调整 TTL 与排序键。
  - 原因：直接修改 ClickHouse 主键/排序键不可依赖，重建更稳定。

- Decision: 引入小时级 Smart Money 聚合表
  - 聚合维度为 `bucket/chain/pool_version/pool_id/wallet_address/action`。
  - 物化视图从 `smart_lp_events` 实时写入聚合表。
  - overview 中的池榜、钱包榜、现金流、快照、事件趋势统一改读聚合表。

- Decision: 池信息缓存下沉到后端服务层
  - Redis key 按 `chain + pool_version + pool_id` 组织。
  - TTL 固定 24 小时；Redis 不可用时回退到直接链上读取。

- Decision: Hot Pools 缓存使用最终响应 JSON
  - Redis 缓存 key 按请求参数维度组织。
  - 直接缓存最终 JSON 字节，减少二次序列化和结构体兼容问题。

## Risks / Trade-offs
- ClickHouse 启动迁移阶段会执行一次最近 2 天数据回填；若历史数据量异常偏大，会拉长启动时间。
  - Mitigation: 只回填最近 2 天，并在迁移日志中明确标记。
- 聚合表会增加写放大。
  - Mitigation: 只保留 2 天，与明细表一致；查询收益明显大于额外写入成本。
- Redis 缓存可能返回短暂陈旧数据。
  - Mitigation: Hot Pools TTL 仅 10 秒，池信息 TTL 24 小时且本身属于低频变更元数据。

## Migration Plan
1. 启动时检查 `smart_lp_events` 定义；若不符合新 TTL / 排序键，则创建新表并回填最近 2 天数据。
2. 创建 Smart Money 聚合表与物化视图；若聚合表为空，则从最近 2 天明细表回灌一次。
3. 将 Smart Money overview 查询切换到聚合表。
4. 为池信息与 Hot Pools 响应增加 Redis 缓存。

## Open Questions
- 无；本次按用户给定的 TTL 与缓存时间直接落地。
