## ADDED Requirements
### Requirement: WebApp MUST 提供独立的创建池子模块
WebApp MUST 提供与现有开仓弹窗分离的「创建池子」模块，用于承载 BSC 一键建池、预览和提交流程。

#### Scenario: 从独立模块进入创建池子流程
- **WHEN** 用户在 WebApp 中打开「创建池子」模块
- **THEN** 系统 SHALL 展示面向 BSC 建池的独立表单、预览区和执行结果区

### Requirement: 建池模块 MUST 支持选择热门池子与聪明钱池子作为基础数据来源
WebApp MUST 允许用户在创建池子模块内选择「热门池子」和「聪明钱」池子作为基础数据来源，并把来源池的 token 基础信息作为建池表单的预填输入。

#### Scenario: 从热门池子来源列表预填
- **WHEN** 用户在创建池子模块中选择一个热门池子作为基础数据来源
- **THEN** 系统 SHALL 预填 token 地址、symbol、来源池价格提示和来源池标识

#### Scenario: 从聪明钱来源列表预填
- **WHEN** 用户在创建池子模块中选择一个聪明钱池子作为基础数据来源
- **THEN** 系统 SHALL 预填 token 地址、symbol、来源池价格提示和来源池标识

### Requirement: 默认表单 MUST 最小化人工输入
WebApp MUST 默认只展示必要的建池字段，并将高级参数折叠，避免用户直接面对过多底层协议参数。

#### Scenario: 默认状态仅显示必要字段
- **WHEN** 用户首次打开创建池子模块
- **THEN** 系统 SHALL 默认展示钱包、协议、Token A/Token B、初始价格、费率或模板、模式以及首注数量等必要字段

#### Scenario: 高级参数默认收起
- **WHEN** 用户未主动展开高级设置
- **THEN** 系统 MUST 隐藏自定义价格区间、deadline、slippage 和 V4 细粒度参数输入

### Requirement: WebApp MUST 在执行前展示 preview 摘要并在成功后展示结果卡
WebApp MUST 在用户提交执行前展示后端返回的归一化摘要、风险提示和池子已存在状态，并在成功后展示可直接使用的结果卡片。

#### Scenario: preview 阻止重复建池
- **WHEN** preview 返回目标池已存在
- **THEN** 系统 MUST 在 UI 中阻止用户继续执行，并明确展示已存在提示

#### Scenario: 建池成功展示结果
- **WHEN** execute 成功完成链上交易
- **THEN** 系统 SHALL 展示结果卡片，其中至少包含协议、pool address 或 poolId、tokenId（若有）和 tx hash
