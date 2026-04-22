## RENAMED Requirements
- FROM: `### Requirement: CLMM 任务越界策略 MUST 仅由再平衡开关决定`
- TO: `### Requirement: CLMM 任务越界策略 MUST 由任务模式决定`

## MODIFIED Requirements
### Requirement: CLMM 任务越界策略 MUST 由任务模式决定
CLMM 任务的自动越界处理 SHALL 由任务模式与暂停状态共同决定，而不再仅由 `rebalance_enabled` 单独表达。

系统 MUST 支持以下自动处理模式：
- `rebalance_all`：上破区间和下破区间都自动再平衡
- `exit_all`：上破区间和下破区间都自动撤出流动性并结束任务
- `rebalance_up_exit_down`：上破区间自动再平衡，下破区间自动撤出流动性并结束任务

系统 MUST 继续支持 `paused=true` 的暂停态；当任务处于暂停态时，系统 SHALL NOT 自动触发再平衡或撤出终止。

对旧客户端与旧任务数据：
- 当仅存在 `rebalance_enabled=true` 时，系统 SHALL 将其视为 `rebalance_all`
- 当仅存在 `rebalance_enabled=false` 时，系统 SHALL 将其视为 `exit_all`

#### Scenario: 双向再平衡模式上破区间
- **GIVEN** 某个 CLMM 任务当前模式为 `rebalance_all`
- **AND** 该任务未暂停
- **AND** 该任务已经处于“区间已激活”状态
- **WHEN** 当前价格向上越过任务区间并持续超过 `reopen_delay_seconds`
- **THEN** 系统 SHALL 自动撤出当前流动性并执行再平衡重开

#### Scenario: 双向撤出模式下破区间
- **GIVEN** 某个 CLMM 任务当前模式为 `exit_all`
- **AND** 该任务未暂停
- **AND** 该任务已经处于“区间已激活”状态
- **WHEN** 当前价格向下越过任务区间并持续超过 `reopen_delay_seconds`
- **THEN** 系统 SHALL 自动撤出当前流动性并兑换回 USDT
- **AND** 任务 SHALL 在撤出完成后进入终止状态

#### Scenario: 非对称模式上破区间
- **GIVEN** 某个 CLMM 任务当前模式为 `rebalance_up_exit_down`
- **AND** 该任务未暂停
- **AND** 该任务已经处于“区间已激活”状态
- **WHEN** 当前价格向上越过任务区间并持续超过 `reopen_delay_seconds`
- **THEN** 系统 SHALL 自动撤出当前流动性并执行再平衡重开

#### Scenario: 非对称模式下破区间
- **GIVEN** 某个 CLMM 任务当前模式为 `rebalance_up_exit_down`
- **AND** 该任务未暂停
- **AND** 该任务已经处于“区间已激活”状态
- **WHEN** 当前价格向下越过任务区间并持续超过 `reopen_delay_seconds`
- **THEN** 系统 SHALL 自动撤出当前流动性并兑换回 USDT
- **AND** 任务 SHALL 在撤出完成后进入终止状态

#### Scenario: 任务被用户暂停时越界
- **GIVEN** 某个 CLMM 任务 `paused=true`
- **WHEN** 当前价格越过任务区间
- **THEN** 系统 SHALL NOT 自动触发再平衡或撤出终止

### Requirement: 双端开仓页 MUST 展示统一的越界执行心智
WebApp 与 MiniApp 的 CLMM 开仓页和仓位卡 SHALL 使用统一的任务模式心智，而不再只暴露单一“再平衡开关”。

开仓页 MUST 提供以下模式入口：
- 双向再平衡
- 双向撤出并结束
- 上破再平衡 / 下破撤出
- 暂停任务

仓位卡 MUST 展示当前任务模式，并允许用户直接切换模式；当用户选择“暂停任务”时，系统 SHALL 进入暂停态且保留上一次自动处理模式用于恢复。

#### Scenario: 开仓页默认使用双向撤出模式
- **GIVEN** 用户首次打开 WebApp 或 MiniApp 的开仓页
- **WHEN** 页面完成默认状态初始化
- **THEN** 默认任务模式 SHALL 为 `exit_all`
- **AND** 页面 SHALL 明确说明“超出区间后会自动撤出并结束任务”

#### Scenario: 仓位卡切换为非对称模式
- **GIVEN** 某个运行中任务当前模式不是 `rebalance_up_exit_down`
- **WHEN** 用户在仓位卡上点击“上破再平衡 / 下破撤出”模式按钮
- **THEN** 前端 SHALL 调用任务模式更新接口
- **AND** 更新成功后仓位卡 SHALL 立即显示新的模式标签与说明

#### Scenario: 仓位卡从暂停恢复到自动模式
- **GIVEN** 某个任务当前处于暂停态
- **WHEN** 用户在仓位卡上点击某个自动模式按钮
- **THEN** 系统 SHALL 取消暂停状态
- **AND** SHALL 将任务模式更新为用户刚选择的自动模式

### Requirement: 旧请求字段与旧任务数据 MUST 保持兼容
系统 SHALL 继续兼容旧客户端和历史任务中的 `rebalance_enabled` 字段，但新实现 MUST 以任务模式为统一语义来源。

#### Scenario: 旧客户端继续提交 rebalance_enabled=true
- **GIVEN** 旧客户端仍向开仓接口提交 `rebalance_enabled=true`
- **WHEN** 后端创建新的 CLMM 任务
- **THEN** 接口 SHALL 保持兼容且不报错
- **AND** 后端 SHALL 将该任务的模式写为 `rebalance_all`

#### Scenario: 旧客户端继续调用 toggle_rebalance 关闭自动再平衡
- **GIVEN** 旧客户端仍调用 `task_toggle_rebalance`
- **WHEN** 请求将自动再平衡切换为关闭
- **THEN** 后端 SHALL 将该任务的模式写为 `exit_all`
- **AND** SHALL 继续返回兼容旧客户端的成功响应
