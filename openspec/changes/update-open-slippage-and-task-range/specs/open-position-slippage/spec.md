## ADDED Requirements
### Requirement: 开仓支持按次滑点覆写
系统 SHALL 在开仓（MiniApp 一键开仓 / Bot 手动开仓）时允许用户提供可选滑点参数 `slippage_tolerance`（百分比）。

#### Scenario: MiniApp 一键开仓传入滑点
- **WHEN** 用户在 MiniApp 一键开仓弹窗填写滑点并提交
- **THEN** 后端创建的任务 SHALL 将该滑点写入 `strategy_tasks.slippage_tolerance`

#### Scenario: 开仓不传滑点则使用全局滑点
- **WHEN** 用户开仓时未传入 `slippage_tolerance`
- **THEN** 后端创建的任务 SHALL 使用全局配置的滑点值写入 `strategy_tasks.slippage_tolerance`

### Requirement: 按次滑点对后续再平衡生效
系统 SHALL 在该任务后续发生的再平衡（re-entry）中沿用 `strategy_tasks.slippage_tolerance`。

#### Scenario: 区间外触发再平衡
- **GIVEN** 任务已开仓且 `strategy_tasks.slippage_tolerance` 已设置
- **WHEN** 任务触发再平衡并重新开仓
- **THEN** 再平衡开仓流程 SHALL 使用该任务的 `slippage_tolerance`

