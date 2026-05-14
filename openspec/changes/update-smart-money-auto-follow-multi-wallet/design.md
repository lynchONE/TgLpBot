## Context
自动跟单当前以 `smart_money_follow_configs.target_wallet_address` 表示单个目标钱包，并通过 `cursor_event_id` 消费该钱包的新 `sm_lp_events`。任务去重依赖 `config_id + event_id + action`，撤仓映射依赖开仓时保存的 `target_position_ref`。

多钱包组合触发会改变配置结构和作业生成条件，但不应改变资金执行链路：一旦触发，开仓仍复用现有跟单开仓任务构建与执行；撤仓仍只影响该配置创建的跟单仓位。

## Goals / Non-Goals
- Goals:
  - 单条自动跟单配置支持多个目标钱包。
  - 支持任意钱包触发与阈值确认触发两种模式。
  - 阈值模式按同一池子、协议、tick 区间和时间窗口聚合不同钱包开仓信号。
  - 任务记录触发钱包列表，前端能解释为什么触发。
- Non-Goals:
  - 不在本次支持任意复杂表达式、钱包权重或跨链组合。
  - 不自动把目标钱包加入聪明钱全局监控列表；若目标钱包没有事件，配置应暴露为无可触发状态而不是静默补数据。
  - 不改变实际开仓金额模式、延迟模式和撤仓跟单的既有语义。

## Decisions
- Decision: 在配置表上增加组合字段，并保留旧单钱包字段兼容。
  - 新字段建议包括 `target_wallet_addresses`、`trigger_mode`、`trigger_min_wallets`、`trigger_window_seconds`。
  - `target_wallet_addresses` 以规范化小写地址数组 JSON 保存；迁移时从 `target_wallet_address` 回填单元素数组。
  - `target_wallet_address` 继续保存主钱包或数组第一个地址，供旧查询和旧前端兼容。

- Decision: `any` 模式复用单事件作业生成。
  - worker 查询目标钱包组中任意钱包的新 `add/remove` 事件。
  - 开仓作业以该事件作为主触发事件，触发钱包列表只包含该钱包。
  - 去重仍需避免同一配置、同一事件、同一动作重复执行。

- Decision: `threshold` 模式只对开仓事件做组合确认。
  - 聚合键为 `chain_id + protocol + pool_address + tick_lower + tick_upper`，必要时包含 position ref 的可比较部分。
  - 在主触发事件时间向前看的 `trigger_window_seconds` 内，统计目标钱包组中不同钱包的 `add` 事件。
  - 达到 `trigger_min_wallets` 后创建一条开仓作业，主触发事件使用窗口内最新事件。
  - 作业需保存参与触发的钱包地址和事件 ID 列表，避免“满足几个钱包”不可追踪。

- Decision: 撤仓跟单保持按已建立映射处理。
  - `any` 模式下，如果目标钱包组中任一钱包发生对应 `remove`，且配置开启撤仓跟单，尝试撤出该配置创建的对应跟单仓位。
  - `threshold` 模式下，撤仓第一版也按任一目标钱包的对应 `remove` 触发；不要求多个钱包同时撤仓，避免无法及时退出。
  - 未找到对应跟单仓位时继续标记为 skipped/failed，并记录原因，不撤出手动仓位或其他配置仓位。

- Decision: 游标必须覆盖钱包组。
  - 配置启用或目标钱包组变更时，游标推进到这些钱包当前最新事件，避免回放历史。
  - 扫描时以配置级 `cursor_event_id` 或等价游标记录整体进度；如果实现需要按钱包游标，应显式建表并保持迁移清晰。

## Risks / Trade-offs
- 阈值模式容易被多个钱包在不同池子或不同区间的事件误聚合。
  - Mitigation: 聚合键必须包含池子、协议和 tick 区间；缺少 tick 区间的事件不得参与阈值确认。
- JSON 地址数组不利于按钱包检索。
  - Mitigation: 第一版配置量预计较小，可按配置扫描；如果配置量增大，再拆分 `smart_money_follow_config_wallets` 关系表。
- 多钱包配置修改可能导致旧游标和新钱包历史事件混用。
  - Mitigation: 目标钱包组或触发模式变更时重置游标到当前最新事件，并记录更新时间。

## Migration Plan
1. 为自动跟单配置表增加组合字段，并回填旧单钱包配置。
2. 扩展保存/查询 API，兼容旧 `target_wallet_address`，新增钱包组和触发规则校验。
3. 改造 worker 的事件扫描与作业创建逻辑，分别覆盖 `any` 和 `threshold`。
4. 扩展作业模型，记录触发钱包/事件列表。
5. 更新 WebApp/MiniApp 自动跟单表单和配置卡片展示。
6. 增加迁移、输入校验、任意钱包触发、阈值触发、撤仓隔离的测试。
