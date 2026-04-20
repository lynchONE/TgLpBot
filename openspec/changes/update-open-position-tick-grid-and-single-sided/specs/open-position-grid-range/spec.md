## ADDED Requirements
### Requirement: Open position MUST 支持按百分比、Tick 与格子三种方式定义区间
开仓与开仓预览接口 MUST 支持 `percentage`、`tick`、`grid` 三种区间输入模式，并始终在响应中返回规范化后的 tick 下上界作为最终执行结果。

#### Scenario: 百分比模式兼容旧客户端
- **WHEN** 客户端继续只传 `range_lower_pct` 与 `range_upper_pct`
- **THEN** 系统 SHALL 按现有逻辑完成百分比到 tick 的换算，并返回规范化后的 `tick_lower` 与 `tick_upper`

#### Scenario: 直接按 Tick 输入区间
- **WHEN** 客户端以 `range_input_mode=tick` 提交 `tick_lower` 与 `tick_upper`
- **THEN** 系统 SHALL 校验 tick 顺序、tick spacing 对齐关系与池子合法性，并返回规范化后的 tick 区间

#### Scenario: 围绕当前价按格子数量输入区间
- **WHEN** 客户端以 `range_input_mode=grid` 提交 `grid_left_count` 与 `grid_right_count`
- **THEN** 系统 SHALL 基于当前 tick 与池子 tick spacing 计算最终 `tick_lower` 与 `tick_upper`，并在响应中返回规范化结果

### Requirement: Open position preview MUST 返回区间几何与单边形态
开仓 prepare 或 preview MUST 返回区间编辑所需的几何信息，包括当前 tick、tick spacing、价格边界、格子数量和仓位形态。

#### Scenario: 返回格子编辑器所需的中心信息
- **WHEN** 客户端请求开仓 prepare 或 preview
- **THEN** 系统 SHALL 返回至少 `current_tick`、`tick_spacing`、规范化后的 `tick_lower`、`tick_upper` 与对应价格边界

#### Scenario: 返回单边或双边形态
- **WHEN** 用户选择的区间完全位于当前价一侧或跨越当前价
- **THEN** 系统 SHALL 返回 `position_shape`，明确标识 `single_token0`、`single_token1` 或 `dual_sided`

### Requirement: Open position MUST 支持单边池开仓心智
当用户选择的区间完全位于当前价一侧时，系统 MUST 将其视为单边仓位并在预览中明确展示预估资产侧。

#### Scenario: 区间位于当前价下方形成单边仓位
- **WHEN** 规范化后的 `tick_upper` 小于或等于当前 tick
- **THEN** 系统 SHALL 将该仓位标记为单边仓位，并在预览中说明其预估主要持有的资产侧

#### Scenario: 区间跨越当前价形成双边仓位
- **WHEN** 规范化后的区间覆盖当前 tick
- **THEN** 系统 SHALL 将该仓位标记为双边仓位，并返回双边资产占比或等效摘要信息
