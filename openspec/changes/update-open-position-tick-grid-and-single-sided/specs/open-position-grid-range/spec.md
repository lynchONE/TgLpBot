## ADDED Requirements
### Requirement: Open position MUST 支持按百分比、Tick 与格子三种方式定义区间
开仓与开仓预览接口 MUST 支持 `percentage`、`tick`、`grid` 三种区间输入模式，并始终在响应中返回规范化后的 `tick_lower` / `tick_upper` 作为最终执行结果。

#### Scenario: 百分比模式兼容旧客户端
- **WHEN** 客户端继续只传 `range_lower_pct` 与 `range_upper_pct`
- **THEN** 系统 SHALL 按现有逻辑完成百分比到 Tick 的换算，并返回规范化后的 `tick_lower` 与 `tick_upper`

#### Scenario: 直接按 Tick 输入区间
- **WHEN** 客户端以 `range_input_mode=tick` 提交 `tick_lower` 与 `tick_upper`
- **THEN** 系统 SHALL 校验 Tick 顺序、Tick Spacing 对齐关系与池子合法性，并返回规范化后的 Tick 区间

#### Scenario: 围绕当前价按格子数量输入区间
- **WHEN** 客户端以 `range_input_mode=grid` 提交 `tick_lower` 与 `tick_upper`
- **THEN** 系统 SHALL 基于池子 Tick Spacing 规范化最终的 `tick_lower` 与 `tick_upper`

### Requirement: Open position preview MUST 返回区间几何与单边形态
开仓 `prepare` 或 `preview` MUST 返回区间编辑所需的几何信息，包括当前 Tick、Tick Spacing、价格边界和仓位形态。

#### Scenario: 返回格子编辑器所需的中心信息
- **WHEN** 客户端请求开仓 `prepare` 或 `preview`
- **THEN** 系统 SHALL 返回至少 `current_tick`、`tick_spacing`、规范化后的 `tick_lower`、`tick_upper` 与对应价格边界

#### Scenario: 返回单边或双边形态
- **WHEN** 用户选择的区间完全位于当前价一侧或跨越当前价
- **THEN** 系统 SHALL 返回 `position_shape`，明确标识 `single_token0`、`single_token1` 或 `dual_sided`

### Requirement: 价格区间输入 MUST 在客户端映射为规范化 Tick 区间
WebApp 与 MiniApp 的价格区间编辑 MUST 使用预览上下文将价格换算为 Tick 区间，并继续通过现有 `tick_lower` / `tick_upper` 预览与开仓链路提交。

#### Scenario: 用户直接输入价格上下限
- **WHEN** 用户在前端输入价格上下限，例如 `0.05 - 0.07`
- **THEN** 客户端 SHALL 依据当前计价方向、token decimals 与 Tick Spacing 将其映射为 Tick 区间并调用现有 Tick 预览/提交流程

#### Scenario: 预览返回规范化后的价格与 Tick
- **WHEN** 前端按价格输入发起预览，后端对 Tick 再次规范化
- **THEN** 前端 SHALL 以响应中的 `tick_lower`、`tick_upper`、`range_lower_price`、`range_upper_price` 作为最终回显与执行依据

### Requirement: Open position MUST 支持单边池开仓心智
当用户选择的区间完全位于当前价一侧时，系统 MUST 将其视为单边仓位，并在预览中明确展示预估资产侧。

#### Scenario: 区间位于当前价下方形成单边仓位
- **WHEN** 规范化后的 `tick_upper` 小于或等于当前 Tick
- **THEN** 系统 SHALL 将该仓位标记为单边仓位，并在预览中说明其预估主要持有的资产侧

#### Scenario: 区间跨越当前价形成双边仓位
- **WHEN** 规范化后的区间覆盖当前 Tick
- **THEN** 系统 SHALL 将该仓位标记为双边仓位，并返回双边资产占比或等效摘要信息
