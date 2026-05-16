## Context
现有聪明钱池子列表由 `/api/sm/pools` 返回，主要基于 `sm_lp_positions` 和 `sm_lp_active_positions` 聚合活跃仓位数量、钱包数量、最新事件时间和仓位金额。

`sm_lp_active_positions` 已持久化每个活跃 LP 仓位的当前手续费估算字段：
- `fee_usd`
- `fee_updated_at`
- `net_total_usd`
- `opened_at`
- `last_add_at`

新增收益火焰图应优先复用这些本地读模型，避免页面请求时逐池扫链。速率口径按活跃仓位从 `opened_at` 到当前时间的平均手续费速度实时计算，不依赖历史快照。

## Goals / Non-Goals
- Goals:
  - 让用户在聪明钱池子视图中快速识别「手续费多」和「手续费增长快」的池子。
  - 固定支持 `30s`、`1m`、`5m`、`1h` 四个时间窗口。
  - 保持现有池子列表能力不变，只移动到 `活跃池子` 子页。
  - 双端 MiniApp / WebApp 展示一致，且都能快捷跟单。
- Non-Goals:
  - 不在本次变更中新增自动下单策略。
  - 不在请求链路中实时计算所有池子的链上手续费。
  - 不把火焰图结果作为收益承诺，只作为机会筛选信号。

## Decisions
- Decision: 火焰图新增接口使用本地聚合读模型。
  - Why: 页面需要高频刷新和快速筛选，逐池逐仓位 RPC 会造成明显延迟和不稳定。
- Decision: `手续费` 口径使用当前活跃仓位 `fee_usd` 总和。
  - Why: 这是用户最容易理解的绝对收益信号。
- Decision: `速率` 口径使用当前手续费除以仓位金额和开仓至今秒数，再折算到用户选择的窗口。
  - Why: 不需要额外快照表，且能按每个仓位的实际持仓时长实时比较资金效率。
- Decision: 当仓位缺少有效金额、手续费或开仓时间时，接口必须返回数据质量计数，前端展示弱化状态，不用默认 0 掩盖数据缺口。
  - Why: 遵守项目 fallback 策略，避免把缺数据误解为没收益。

## Data Shape
建议新增返回字段：
- `window`: `30s | 1m | 5m | 1h`
- `sort`: `fee | rate`
- `updated_at`
- `list[]`:
  - 池子基础字段：`pool_address`、`protocol`、`chain_id`、`token0/1`、`fee_tier`、`trading_pair`、`display_token_*`
  - 活跃字段：`wallet_count`、`open_position_count`、`latest_event_at`
  - 金额字段：`total_position_amount_usd`
  - 手续费字段：`fee_usd`
  - 速率字段：`projected_fee_usd`、`fee_rate_per_1k_usd_window`、`fee_rate_per_1k_usd_per_min`
  - 数据质量字段：`sample_status`、`fee_position_count`、`rate_position_count`、`missing_fee_count`、`missing_amount_count`

## Risks / Trade-offs
- 风险：开仓时间很短的仓位可能因短期手续费估算抖动导致速率偏高。
  - Mitigation: 后端返回 `average_age_seconds`，前端展示持仓时长并可弱化样本不足池子。
- 风险：火焰图过度强化短期信号，用户可能忽略流动性和区间风险。
  - Mitigation: 卡片保留仓位金额、钱包数、仓位数、最近活跃时间，并复用开仓风控。

## Migration Plan
1. 新增后端聚合查询与接口，保持 `/api/sm/pools` 现有行为不变。
2. 扩展 MiniApp/WebApp 代理白名单和 API helper。
3. 在双端聪明钱池子视图增加二级 Tab，把旧列表放到 `活跃池子`。
4. 新增 `收益火焰图` UI，接入排序、窗口切换和快捷跟单。
5. 补充后端聚合排序测试和双端 build 验证。

## Open Questions
- 火焰图默认排序是 `速率` 还是 `手续费`？建议默认 `速率`，因为更符合「第一时间锁定适合开单池子」。
