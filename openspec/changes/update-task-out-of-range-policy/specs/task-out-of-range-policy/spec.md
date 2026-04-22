## ADDED Requirements
### Requirement: CLMM 任务越界策略 MUST 仅由再平衡开关决定
CLMM 任务的自动越界处理 SHALL 仅由 `rebalance_enabled` 决定，而不再因价格向上/向下越界或 `stop_loss_enabled` 的取值而产生不同执行路径。

#### Scenario: 开启再平衡时向上越界
- **GIVEN** 某个 CLMM 任务 `rebalance_enabled=true`
- **AND** 该任务已经处于“区间激活”状态
- **WHEN** 当前价格向上越过任务区间并持续超过 `reopen_delay_seconds`
- **THEN** 系统 SHALL 自动撤出当前流动性并执行再平衡重开

#### Scenario: 开启再平衡时向下越界
- **GIVEN** 某个 CLMM 任务 `rebalance_enabled=true`
- **AND** 该任务已经处于“区间激活”状态
- **WHEN** 当前价格向下越过任务区间并持续超过 `reopen_delay_seconds`
- **THEN** 系统 SHALL 自动撤出当前流动性并执行再平衡重开

#### Scenario: 关闭再平衡时任意方向越界
- **GIVEN** 某个 CLMM 任务 `rebalance_enabled=false`
- **AND** 该任务已经处于“区间激活”状态
- **WHEN** 当前价格向上或向下越过任务区间并持续超过 `reopen_delay_seconds`
- **THEN** 系统 SHALL 自动撤出当前流动性并兑换回 USDT
- **AND** 任务 SHALL 在撤出完成后进入终止状态

#### Scenario: 任务被用户暂停时越界
- **GIVEN** 某个 CLMM 任务 `paused=true`
- **WHEN** 当前价格越过任务区间
- **THEN** 系统 SHALL NOT 自动触发再平衡或撤仓终止

### Requirement: 单边池 MUST 在首次进入区间后才激活越界监控
若 CLMM 任务在创建或重开时价格尚未进入配置区间，系统 SHALL 将其视为“未激活区间”的单边池任务；该任务只有在价格首次进入区间后，才开始按普通双边池的越界规则处理。

#### Scenario: 单边池初始未进入区间
- **GIVEN** 某个 CLMM 任务在创建或重开时当前价格位于区间外
- **WHEN** 策略轮询检测到该任务仍未进入区间
- **THEN** 系统 SHALL NOT 启动 `out_of_range_since` 倒计时
- **AND** 系统 SHALL NOT 自动触发再平衡或撤仓终止

#### Scenario: 单边池首次进入区间
- **GIVEN** 某个 CLMM 任务此前处于“未激活区间”状态
- **WHEN** 当前价格首次进入该任务的配置区间
- **THEN** 系统 SHALL 将该任务标记为“区间已激活”
- **AND** 该任务后续越界时 SHALL 按普通双边池规则处理

#### Scenario: 已激活单边池再次越界
- **GIVEN** 某个单边池任务已经完成过首次进入区间
- **WHEN** 当前价格随后再次越出区间并满足缓冲时间
- **THEN** 系统 SHALL 按 `rebalance_enabled` 对应的自动处理策略执行

### Requirement: 双端开仓页 MUST 展示统一的越界执行心智
WebApp 与 MiniApp 的 CLMM 开仓页 SHALL 使用与后端一致的越界执行说明，不再把止损作为独立的越界执行模式入口。

#### Scenario: WebApp 展示自动模式
- **GIVEN** 用户在 WebApp 开仓页开启再平衡
- **WHEN** 页面展示越界执行说明
- **THEN** 页面 SHALL 明确说明“超出区间后会在缓冲时间后自动再平衡”

#### Scenario: MiniApp 展示手动模式
- **GIVEN** 用户在 MiniApp 开仓页关闭再平衡
- **WHEN** 页面展示越界执行说明
- **THEN** 页面 SHALL 明确说明“超出区间后会在缓冲时间后自动撤仓并终止任务”

#### Scenario: 单边池展示额外说明
- **GIVEN** 用户当前配置的是单边池区间
- **WHEN** 页面展示单边池提示
- **THEN** 页面 SHALL 明确说明“首次进入区间前不会触发自动再平衡或自动撤仓”

### Requirement: 旧请求字段与旧任务数据 MUST 保持兼容
系统 SHALL 继续接受现有请求和历史任务数据中的 `stop_loss_enabled` 及相关字段，但 CLMM 越界执行逻辑 MUST 不再依赖这些字段决定自动处理分支。

#### Scenario: 旧客户端继续传递 stop_loss_enabled
- **GIVEN** 旧客户端仍向开仓接口传递 `stop_loss_enabled`
- **WHEN** 后端接收并创建 CLMM 任务
- **THEN** 接口 SHALL 保持兼容，不因该字段报错
- **AND** CLMM 越界执行策略 SHALL 仍仅由 `rebalance_enabled` 决定

#### Scenario: 历史任务保留 stop_loss_enabled=true
- **GIVEN** 某个历史 CLMM 任务保留 `stop_loss_enabled=true`
- **WHEN** 该任务在新逻辑下发生越界
- **THEN** 系统 SHALL 忽略该字段对 CLMM 越界分支的影响
- **AND** 仍按 `rebalance_enabled` 对应的统一策略执行
