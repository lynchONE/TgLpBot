## Context
- 当前 `pool_sync` 会从 PoolM 拉热门池子数据，再用 DexScreener 补全 TVL、24h 成交量、创建时间、FDV 等字段。
- 现网 PoolM `top-fees/5` 已经返回完整结构，包含共享当前态字段、复杂数组字段以及当前窗口统计字段。
- `pools` 现在已经是 `/api/pools` 的 MySQL 数据底座，因此 schema 改动必须同时考虑“完整保留上游字段”和“读路径直接使用 5 分钟快照”两个目标。

## Goals / Non-Goals
- Goals:
  - 让 `pools` 表保留 PoolM `top-fees/5` 最新返回中的关键字段，不再因为旧 schema 丢失信息
  - 让 `pool_sync` 只依赖 PoolM，同步来源单一、可解释
  - 让 `/api/pools` 能直接从 MySQL 读取 `top-fees/5` 的持久化结果
- Non-Goals:
  - 不在本次重写 `search_pools` 的 DexScreener 搜索逻辑
  - 不在本次引入新的 `pools` 辅助表；优先在 `pools` 表内完成持久化
  - 不改变现有 Telegram WebApp 鉴权、权限校验与 Redis 缓存策略

## Decisions
- Decision: `pool_sync` 的池子目录同步只使用 PoolM `top-fees` 接口。
  - Why: 用户明确要求删除 DexScreener 补数；现网 PoolM 已返回足够完整的池子数据。

- Decision: 默认只同步 `top-fees/5`，并将这份 5 分钟窗口结果视为当前热门池子快照。
  - Why: 用户已经明确只要 `top-fees/5`。

- Decision: `pools` 表按三类数据组织字段。
  - 共享当前态字段：例如 `factory_*`、token 基础信息、`current_pool_value`、`current_token_price`、tick/liquidity 状态等。
  - 当前窗口统计字段：例如 `transaction_count`、`total_fees`、`total_volume`、`unique_wallets`、`top_wallet_vol_pct`，直接保存 `top-fees/5` 返回的值。
  - 复杂返回字段：例如 `metricTrends`、`liquidityTicks`、`badges` 以及响应级索引数组，使用 JSON 列保存，避免再次丢字段。
  - Why: 排序和筛选需要标量列；“保留最新返回数据”又要求复杂字段可原样保存，单纯用旧标量 schema 无法满足。

- Decision: 对于响应级元数据（如 `metricTrendsIndex`、`liquidityTicksIndex`、请求窗口上下文），直接保存在 `pools` 行内的 `source_*` / `*_index_json` 字段中。
  - Why: 本次约束是不新增辅助表，同时用户要求 `pools` 表尽量保留最新返回信息。

- Decision: `/api/pools` 读取时统一基于 `top-fees/5` 的持久化结果返回，也不再从 DexScreener 或其它来源派生字段。
  - Why: 读路径必须和用户指定的唯一上游数据源保持一致，避免再次出现“字段存在但来源不清”的情况。

## Risks / Trade-offs
- `pools` 单行会因为 JSON 字段变宽。
  - Mitigation: 只把需要原样保留的复杂字段放到 JSON 列，可排序指标继续用标量列。

- 旧列与新列在迁移阶段可能并存，相关读写代码会有一段过渡期。
  - Mitigation: 先补 schema 和写路径，再切换读路径，最后再清理废弃逻辑。

- 当前工作区在 `pool_sync`、`pools_catalog` 等文件上已有未提交改动。
  - Mitigation: 实施时只做增量修改，避免覆盖现有未提交编辑。

## Migration Plan
1. 为 `pools` 增加新的 PoolM 字段、5 分钟窗口列和 JSON 列，保留现有表名不变。
2. 重构 `pool_sync` 写路径，仅使用 PoolM `top-fees/5` 构造并写入 `pools`。
3. 切换 `/api/pools` 与相关读路径到新列，并保持现有接口鉴权与缓存行为。
4. 删除 `pool_sync` 内 DexScreener client 与补数代码，清理已无意义的旧派生字段逻辑。
5. 通过编译、测试和 MySQL 抽样核对确认新行结构符合上游返回。

## Open Questions
- 暂无；本次按“`search_pools` 不动、`pool_sync` 去 DexScreener、`pools` 保留 `top-fees/5` 最新字段”推进。
