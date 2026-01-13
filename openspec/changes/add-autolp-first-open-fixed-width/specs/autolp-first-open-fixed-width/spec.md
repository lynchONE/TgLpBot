## ADDED Requirements

### Requirement: AutoLP 首次开仓固定区间宽度开关
系统 MUST 提供系统级配置项：
- `autolp_first_open_fixed_width_enabled`（默认关闭）
- `autolp_first_open_fixed_width_percent`（首次开仓固定“总宽度(%)”）

#### Scenario: 未开启则保持原行为
- **WHEN** `autolp_first_open_fixed_width_enabled=false`
- **THEN** AutoLP 首次开仓区间宽度计算 SHALL 保持原逻辑

### Requirement: 固定区间仅影响首次开仓
当 `autolp_first_open_fixed_width_enabled=true` 且 `autolp_first_open_fixed_width_percent > 0` 时，系统 MUST 仅在首次开仓阶段应用该固定总宽度，并确保后续再平衡仍按任务原有区间逻辑执行。

- 具体要求：
- AutoLP 在创建 Auto 任务并首次开仓时 MUST 使用该固定总宽度计算实际开仓 `tick_lower/tick_upper`
- 该固定区间 MUST NOT 覆写任务用于后续再平衡的 `range_lower_percentage/range_upper_percentage`

#### Scenario: 首次开仓使用固定总宽度，但再平衡仍按原逻辑
- **GIVEN** `autolp_first_open_fixed_width_enabled=true` 且 `autolp_first_open_fixed_width_percent=10`
- **WHEN** AutoLP 创建任务并首次开仓
- **THEN** 该次开仓使用固定总宽度（10%）计算 `tick_lower/tick_upper`
- **AND** 任务后续再平衡重新开仓时，区间计算 SHALL 使用任务自身的 `range_lower_percentage/range_upper_percentage`（原逻辑）

### Requirement: MiniApp 管理员可配置首次开仓固定区间
管理员 MUST 能通过 MiniApp 管理员面板读取与更新以下系统级配置项：
- `autolp_first_open_fixed_width_enabled`
- `autolp_first_open_fixed_width_percent`

#### Scenario: 更新后对后续新开仓生效
- **WHEN** 管理员在 MiniApp 更新上述配置项
- **THEN** 后续 AutoLP 新创建任务的首次开仓 SHALL 按最新配置执行
