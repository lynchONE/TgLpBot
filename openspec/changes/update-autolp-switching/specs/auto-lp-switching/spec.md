## ADDED Requirements

### Requirement: AutoLP 满仓换仓触发条件
当用户的 AutoLP 已开启且达到其 `max_active_tasks` 上限时，系统 MUST 评估是否需要进行“换仓”。

当用户未满仓时，系统 MUST 保持现有行为（按候选池开新仓），不得因为换仓逻辑而提前撤出已有仓位。

#### Scenario: 未满仓不触发换仓
- **WHEN** 用户的 AutoLP 处于开启状态且 `active_auto_tasks < max_active_tasks`
- **THEN** 系统不会撤出任何已有 AutoLP 仓位以进行换仓

#### Scenario: 满仓时进入换仓评估
- **WHEN** 用户的 AutoLP 处于开启状态且 `active_auto_tasks >= max_active_tasks`
- **THEN** 系统进入换仓评估流程

### Requirement: 目标池选择（Top1 候选）
换仓的目标池 MUST 来自当前扫描结果中对该用户“可执行”的 Top1 候选池（`Action=CANDIDATE`），并满足：
- 目标池不应已存在该用户的活跃任务（避免重复开同一池）
- 目标池满足用户任务的入场约束（例如 `allow_entry_swap` 为 false 时，目标池包含 USDT 才可作为目标）

#### Scenario: 目标池已存在活跃任务则跳过
- **WHEN** Top1 候选池已经存在该用户的活跃任务
- **THEN** 系统不调度换仓

#### Scenario: 入场约束不满足则跳过
- **WHEN** Top1 候选池不满足该用户的入场约束（例如不含 USDT 且不允许 entry swap）
- **THEN** 系统不调度换仓

### Requirement: 被替换仓位选择（最低收益 AutoLP 仓位）
系统 MUST 从该用户当前的 AutoLP 活跃仓位中选择一个“最低收益”的仓位作为被替换对象：
- 必须是 `is_auto=true` 的任务
- 必须存在可撤出的链上仓位（exit 可执行）
- 必须没有正在进行的 exit/rebalance（`exit_pending_action` 为空且 `rebalance_pending=false`）

“收益率”对比口径 MUST 使用当前扫描快照中的 `FeeRate5mPct`（5m 手续费/TVL，百分比）。

#### Scenario: 选择最低 FeeRate5mPct 的任务
- **WHEN** 用户存在多个满足条件的 AutoLP 仓位
- **THEN** 系统选择 `FeeRate5mPct` 最低的那个作为被替换对象

### Requirement: 换仓阈值（用户可配置）
系统 MUST 仅在目标池收益相对提升达到用户配置阈值 `switch_min_improvement_pct` 时才允许换仓。

阈值规则 MUST 满足：
- 目标收益 `target` 必须大于当前最低收益 `current`
- 当 `switch_min_improvement_pct > 0` 时，必须满足 `target >= current * (1 + pct/100)`
- 当 `switch_min_improvement_pct <= 0` 时，换仓 MUST 被视为禁用，系统 MUST NOT 调度换仓

#### Scenario: 阈值为 0 时禁用换仓
- **WHEN** `switch_min_improvement_pct <= 0`
- **THEN** 系统不调度换仓

#### Scenario: 提升未达阈值则不换仓
- **WHEN** `target` 未达到 `current * (1 + pct/100)`
- **THEN** 系统不调度换仓

#### Scenario: 提升达阈值则允许换仓
- **WHEN** `target` 达到或超过 `current * (1 + pct/100)`
- **THEN** 系统允许调度换仓（仍需满足冷却与并发要求）

### Requirement: 换仓冷却（用户可配置）
系统 MUST 支持用户级冷却时间 `switch_cooldown_seconds`（默认 300 秒）。

冷却起点 MUST 以“换仓完成（新仓开仓成功）”为准。

`last_switch_completed_at` MUST 表示该用户最近一次换仓完成的时间戳（若从未换仓完成，则视为不存在）。

当 `now - last_switch_completed_at < switch_cooldown_seconds` 时，系统 MUST NOT 调度新的换仓。

#### Scenario: 冷却窗口内禁止换仓
- **WHEN** 用户在 `switch_cooldown_seconds` 窗口内已经完成过一次换仓
- **THEN** 系统不调度新的换仓

#### Scenario: 冷却结束后允许再次换仓
- **WHEN** 距离 `last_switch_completed_at` 已超过 `switch_cooldown_seconds`
- **THEN** 系统可再次评估并在满足阈值时调度换仓

### Requirement: 换仓调度方式与用户隔离
当系统调度一次换仓时，系统 MUST 复用现有撤仓/开仓流程，并以任务状态机方式推进：
- 被替换任务必须设置 `exit_pending_action=switch`
- 必须设置 `switch_target_pool_version`、`switch_target_pool_id` 与目标开仓区间参数（`switch_target_tick_*_pct`）
- 所有 DB 查询与更新 MUST 以 `user_id` 作为约束，避免跨用户影响

#### Scenario: 换仓仅影响当前用户的任务
- **WHEN** 系统为某个用户调度换仓
- **THEN** 只会更新该用户的任务记录，不会修改其他用户的任务

### Requirement: 并发与幂等保护
系统 MUST 保证同一用户同一时刻最多调度一次换仓，并避免在并发扫描/重复触发时产生重复的 `switch` 调度。

#### Scenario: 已有 switch 进行中则不重复调度
- **WHEN** 该用户存在任何 `exit_pending_action=switch` 的任务
- **THEN** 系统不调度新的换仓
