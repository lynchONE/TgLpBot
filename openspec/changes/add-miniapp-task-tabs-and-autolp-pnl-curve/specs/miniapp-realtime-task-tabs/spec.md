## ADDED Requirements

### Requirement: 实时仓位任务标签
MiniApp 的「实时仓位」页面 MUST 提供任务筛选标签：`全部`、`手动任务`、`Auto任务`。

默认选中 MUST 为 `全部`。

#### Scenario: 默认显示全部
- **WHEN** 用户打开「实时仓位」页面
- **THEN** 默认选中 `全部` 标签并展示所有仓位/任务卡片

#### Scenario: 切换到手动任务
- **WHEN** 用户选择 `手动任务` 标签
- **THEN** 页面仅展示 `task_id > 0` 且 `task_is_auto=false` 的任务卡片

#### Scenario: 切换到 Auto 任务
- **WHEN** 用户选择 `Auto任务` 标签
- **THEN** 页面仅展示 `task_id > 0` 且 `task_is_auto=true` 的任务卡片

#### Scenario: 非任务仓位不出现在任务标签
- **WHEN** 用户选择 `手动任务` 或 `Auto任务` 标签
- **THEN** `task_id` 为空或为 `0` 的仓位卡片不会被展示

### Requirement: Realtime positions 返回任务类型
后端 `/api/realtime_positions` 返回的每个 `position` MUST 在 `task_id>0` 时包含 `task_is_auto` 字段（布尔值），用于前端区分手动/Auto 任务。

#### Scenario: V3/V4/pending 任务均可区分类型
- **WHEN** 用户存在 V3/V4 或 pending（无 tokenId 但处于撤出/再平衡等状态）的运行中任务
- **THEN** 对应 `position` 返回 `task_id` 与 `task_is_auto`

