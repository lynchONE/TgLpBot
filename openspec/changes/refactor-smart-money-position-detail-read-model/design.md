## Context
- 当前聪明钱采集链路已经完全基于 MySQL：`watcher.go` 在同一事务里写入 `sm_lp_events` 与 `sm_lp_positions`，并没有 ClickHouse 依赖。
- 当前聪明钱列表只能展示摘要信息；点开后如果要做“像自己的仓位一样”的详情卡片，现有后端没有可直接复用的读模型。
- 项目已有 `pools` 表，里面保存了 `current_tick`、`current_sqrt_price_x96`、token decimals、tickSpacing 等池子快照；这些字段足以支持“无链上 slot0 调用”的持仓金额估算。
- 真正高成本的是：详情请求每次都要先通过 RPC 重新解析池子 / token / manager 元数据，再去读取实时链上状态；这会让一个仓位详情请求拆成多段 RPC。

## Goals / Non-Goals
- Goals:
  - 让聪明钱仓位详情具备独立的 MySQL 持久化读模型。
  - 在事件入库时就保存详情查询所需的核心静态字段与当前活跃状态。
  - 让 WebApp / MiniApp 的聪明钱仓位详情轮询在链上只读取真正需要实时性的字段，而不是重复查询静态元数据。
  - 让详情页返回结构尽量贴近现有仓位卡片所需字段，降低前端适配成本。
- Non-Goals:
  - 不要求把聪明钱详情完全升级为审计级收益系统。
  - 不要求彻底去掉所有链上读取；实时字段仍然以链上为准。
  - 不要求在首版就覆盖钱包自由余额等所有“自己的实时仓位”字段；缺失字段允许明确降级。

## Decisions
- Decision: 使用 MySQL `sm_lp_active_positions` 作为聪明钱当前仓位详情真源
  - 主键引用采用 `position_ref`，逻辑上由以下字段组成：
    - `chain_id`
    - `protocol`
    - `wallet_address`
    - `nft_token_id`
    - 当 `nft_token_id` 缺失时，退化为 `pool_address + tick_lower + tick_upper`
  - 表内至少持久化以下字段：
    - 钱包 / 池子 / tokenId / 协议 / tickLower / tickUpper / feeTier / tickSpacing
    - token0 / token1 地址、symbol、decimals
    - `current_liquidity`
    - `is_active`
    - `opened_at` / `last_add_at` / `last_remove_at` / `updated_at`
    - 开仓金额快照：`entry_amount0` / `entry_amount1` / `entry_total_usd`
    - 净投入快照：`net_amount0` / `net_amount1` / `net_total_usd`
    - 最近手续费快照：`fee_amount0` / `fee_amount1` / `fee_usd` / `fee_status` / `fee_updated_at`

- Decision: add/remove 入库时同步更新 active position
  - add：
    - 新建或更新 active row。
    - 累加 `current_liquidity`。
    - 刷新 `last_add_at`、token 元数据、开仓 / 净投入快照。
    - `is_active = current_liquidity > 0`
  - remove：
    - 扣减 `current_liquidity`。
    - 刷新 `last_remove_at`。
    - 当 `current_liquidity <= 0` 时，将其归零并标记 `is_active=false`。

- Decision: 详情查询采用“持久化元数据 + 最小化链上实时读取”的混合模型
  - `sm_lp_active_positions` 提供以下无需重复 RPC 解析的静态信息：
    - 池子 / token / manager / tick 区间 / decimals / tickSpacing / 当前活跃 liquidity
  - 详情请求时，链上只读取真正需要实时性的字段，例如：
    - current tick / sqrtPrice
    - 必要的 live position 数据
    - claimable fee
  - 请求路径不得为了以下目的重复发起元数据类 RPC：
    - 反查池子地址 / poolId
    - 反查 token 地址、symbol、decimals
    - 反查 tickSpacing、feeTier、position manager 关联关系

- Decision: 手续费支持实时链上读取，但必须有缓存与降级
  - 详情接口可以读取链上 claimable fee。
  - 对高成本字段必须增加短 TTL 缓存、并发去重和限流。
  - 当链上读取失败时，允许返回最近一次成功快照，并用 `fee_status` / `warnings` 显式标记降级状态。

- Decision: 详情接口返回卡片化字段
  - 后端详情接口直接返回与仓位卡片接近的结构：
    - `title`
    - `status_label`
    - `current_tick`
    - `tick_lower`
    - `tick_upper`
    - `in_range`
    - `token_rows`
    - `totals`
    - `current_value_usd`
    - `absolute_pnl_usd`
    - `has_pnl`
    - `poll_interval_sec`
    - `warnings`
  - WebApp / MiniApp 只在显示层做轻量映射，不再各自重复拼装详情数据。

## Risks / Trade-offs
- 将池子 / token / manager 元数据前移到读模型后，入库链路更重
  - Mitigation: 仅保存详情查询必需的静态字段与 active state，不把所有动态链上值都写死。
- 详情仍然依赖链上读取 current tick / fee 时，RPC 仍可能抖动
  - Mitigation: 增加短 TTL 缓存、singleflight、超时和显式降级状态，避免前端轮询放大抖动。
- 手续费快照可能滞后
  - Mitigation: 返回 `fee_status` 与 `fee_updated_at`，让前端显式展示估算状态。

## Migration Plan
1. 新增 `sm_lp_active_positions` 模型、索引与 MySQL 迁移逻辑。
2. 新增基于 `sm_lp_events` 回放的初始化逻辑，为现有仓位生成 active rows。
3. 修改 Smart Money watcher，在写入 `sm_lp_events` / `sm_lp_positions` 的同一事务中 upsert `sm_lp_active_positions`。
4. 为动态链上字段增加短 TTL 缓存 / 并发去重 / 降级快照。
5. 新增统一详情接口，并让列表接口输出 `position_ref`。
6. WebApp / MiniApp 接入详情弹层与轮询刷新。

## Open Questions
- 首版 WebApp 是否直接在现有列表下展开详情，还是使用独立抽屉 / 弹层。
- 钱包自由余额是否纳入聪明钱详情首版；如果不纳入，前端需要如何展示缺省态。
- 手续费降级快照是按“最近打开详情的仓位”懒刷新，还是按全量活跃仓位定时刷新更合适。
