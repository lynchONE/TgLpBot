## Context
当前聪明钱模块的“当前活跃仓位”来自两种临时拼装：
- 列表视图：按 `smart_lp_events` 聚合最近 add/remove 事件，使用净 liquidity 推断是否仍然活跃
- 详情视图：在聚合结果基础上，临时调用链上 position manager / pool 状态做 live resolve

这导致两个结构性问题：
- 事件表 TTL 仅 2 天，且漏采 remove 时无法在查询层稳定纠正
- 查询路径引入链上 RPC，接口耗时和稳定性直接受外部节点影响

## Goals / Non-Goals
- Goals:
  - 让聪明钱“当前是否活跃”在存储层有单独真源
  - 在用户请求链路中移除“为校验活跃状态而发起的链上 RPC”
  - 保持 `smart_lp_events` 继续承担明细、走势、Pnl 回放等用途
  - 为后续彻底消除空壳钱包行提供稳定基础
- Non-Goals:
  - 本变更不要求消除所有链上读取；手续费、实时金额等仍可按现有策略逐步收敛
  - 本变更不要求把 2 天前的全量历史都永久回填到 active state

## Decisions

- Decision: 新增 `smart_lp_active_positions` 作为当前活跃仓位真源
  - 主键维度使用仓位唯一键：
    - `chain`
    - `pool_version`
    - `pool_id`
    - `wallet_address`
    - `contract_address`
    - `token_id`
    - `tick_lower`
    - `tick_upper`
  - 额外持久化：
    - `current_liquidity`
    - `is_active`
    - `first_add_at`
    - `last_add_at`
    - `last_remove_at`
    - `last_event_seq`
    - `last_event_block`
    - `updated_at`

- Decision: 采集 add/remove 时增量更新 active state
  - add:
    - `current_liquidity += liquidity_delta`
    - 若此前为 0，则刷新 `first_add_at` / 当前 run 起点
    - `is_active = current_liquidity > 0`
  - remove:
    - V3 使用负向扣减
    - V4 使用事件自带 signed `liquidity_delta`
    - 扣减后 `current_liquidity <= 0` 时，将 `is_active=false` 并把 `current_liquidity` 归零

- Decision: 请求接口只查 active state，不再用 live RPC 过滤空壳行
  - `smart_money_pool_adds` 只从 `smart_lp_active_positions WHERE is_active=1` 读取当前钱包行
  - `smart_money_wallet_positions` 优先从 `smart_lp_active_positions` 拿当前位置引用，再做仓位详情构建

- Decision: 保留后台低频校准，不进入请求主链路
  - 通过定时任务按 watchlist / 活跃池窗口抽样回放或链上校验，修复漏掉的 remove/add
  - 若校准发现 active state 与链上差异，以校准结果修正 active state

## Alternatives Considered

- 方案 A：继续在查询时打链上 RPC 过滤
  - 优点：实现快
  - 缺点：RPC 压力不可控，且不能作为真源
  - 结论：拒绝

- 方案 B：仅依赖 `smart_lp_events` 净 liquidity 聚合
  - 优点：不打链上
  - 缺点：漏采 remove 时无法自动恢复；TTL 2 天后还会失真
  - 结论：拒绝

- 方案 C：将 active state 前移到采集层，并配合后台校准
  - 优点：查询快、稳定，不依赖请求时 RPC
  - 缺点：实现复杂度更高，需要新增表与回填逻辑
  - 结论：采用

## Risks / Trade-offs
- 采集链路一旦漏事件，active state 会漂移
  - Mitigation: 增加低频 reconciliation，并让 active state 保存最近事件游标与更新时间
- 新增状态表后，需要处理与旧明细表的初始化一致性
  - Mitigation: 启动时从最近窗口 `smart_lp_events` 回放初始状态，并标记回放窗口
- 同一仓位键在历史上多次开平仓，运行时间与成本口径要和 active state 配合
  - Mitigation: active state 保存当前 run 的起点，历史 PnL 仍由事件回放辅助计算

## Migration Plan
1. 新增 `smart_lp_active_positions` 表与相关写入 helper。
2. 启动迁移时从最近窗口 `smart_lp_events` 回放 active state，生成初始快照。
3. SmartLP monitor 在插入 `smart_lp_events` 后同步 upsert active state。
4. `smart_money_pool_adds` 切换到 active state 读路径。
5. `smart_money_wallet_positions` 切换到 active state 读路径。
6. 增加后台校准任务与告警。
7. 稳定后逐步移除请求路径中的活跃校验 RPC。

## Open Questions
- active state 的历史保留周期是否长期保存，还是与 watchlist 生命周期联动
- 校准任务是按钱包、按池子还是按最近变更仓位分批执行更合适
