## ADDED Requirements
### Requirement: 运行任务支持修改区间（下次再平衡生效）
系统 SHALL 允许用户在任务运行期间修改该任务的区间配置，且修改 SHALL 在下一次再平衡重新开仓时生效。

#### Scenario: 修改区间不立即影响当前链上区间
- **GIVEN** 任务当前有链上仓位且正在运行
- **WHEN** 用户修改任务区间
- **THEN** 修改 SHALL 不直接改变当前链上仓位的区间
- **AND** 修改 SHALL 在下一次再平衡重新开仓时生效

### Requirement: MiniApp 支持修改区间
系统 SHALL 在 MiniApp 中提供修改区间入口，并通过后端 API 更新任务区间配置。

#### Scenario: MiniApp 调用更新区间接口
- **WHEN** MiniApp 提交 `taskId` 与 `range_lower_pct/range_upper_pct`
- **THEN** 后端 SHALL 校验并更新任务的区间配置

### Requirement: Bot 支持修改区间
系统 SHALL 在 Bot 的任务卡中提供“修改区间”入口，允许用户输入 `5` 或 `1 3` 形式更新区间配置。

#### Scenario: Bot 修改区间
- **WHEN** 用户在 Bot 中修改区间
- **THEN** Bot SHALL 更新任务区间配置并提示“下次再平衡生效”

### Requirement: MiniApp 仓位卡展示策略区间
MiniApp 的实时仓位卡 SHALL 展示任务的“策略区间（下次再平衡）”，以反映任务配置而非当前链上区间。

#### Scenario: 修改后可见
- **GIVEN** 用户已修改任务区间
- **WHEN** MiniApp 拉取实时仓位数据
- **THEN** 仓位卡 SHALL 展示更新后的策略区间信息
