## ADDED Requirements
### Requirement: WebApp MUST 提供独立的创建池子面板和来源预填
WebApp MUST 提供与现有开仓弹窗分离的创建池子面板，并允许从热门池子或聪明钱池子预填 token、协议、费率及来源信息。

#### Scenario: 从来源池预填协议信息
- **WHEN** 用户在创建池子面板中选择一个来源池
- **THEN** 系统 SHALL 预填 token 地址、币对、协议、费率或 `tick_spacing`、价格提示，并以协议图标和版本标识展示来源

#### Scenario: 手动输入仍然可用
- **WHEN** 用户不选择来源池而直接手动输入 token
- **THEN** 系统 SHALL 保留完整的手动建池入口

### Requirement: WebApp MUST 按协议展示费率与区间控件
WebApp MUST 根据所选协议展示正确的费率与区间输入控件，不得把各协议的 fee 规则混用。

#### Scenario: Uniswap V3 展示固定费率档位
- **WHEN** 用户选择 `Uniswap V3`
- **THEN** 系统 SHALL 仅展示 `0.01%`、`0.05%`、`0.3%`、`1%` 四个建池费率档位

#### Scenario: PancakeSwap V3 展示固定费率档位
- **WHEN** 用户选择 `PancakeSwap V3`
- **THEN** 系统 SHALL 仅展示 `0.01%`、`0.05%`、`0.25%`、`1%` 四个建池费率档位

#### Scenario: Uniswap V4 展示任意 fee 与 tick spacing 输入
- **WHEN** 用户选择 `Uniswap V4`
- **THEN** 系统 SHALL 展示任意静态 fee 输入框、`tick_spacing` 输入框，以及 `full_range` / `custom_range` 切换

### Requirement: WebApp MUST 在输入一侧金额后展示另一侧金额估算，并支持单币自动换币模式
WebApp MUST 在创建池子面板中支持用户输入单侧金额后查看另一侧估算值，并允许用户显式选择单币自动换币建池。

#### Scenario: 输入稳定币或任一侧金额后显示镜像估算
- **WHEN** 用户只输入 `Token A` 或 `Token B` 的金额
- **THEN** 系统 SHALL 基于 preview 结果展示另一侧 token 的镜像金额估算、估算来源和提示文案

#### Scenario: 双币精确输入模式
- **WHEN** 用户选择 `dual_exact`
- **THEN** 系统 SHALL 要求用户同时确认两侧 token 数量，再允许执行

#### Scenario: 单币自动换币模式
- **WHEN** 用户选择 `single_auto_swap`
- **THEN** 系统 SHALL 允许用户只保留一侧输入，并在界面中展示自动换币和滑点风险提示

### Requirement: WebApp MUST 在执行前展示 preview 摘要，并在失败后保留上下文
WebApp MUST 在执行前展示后端返回的归一化摘要、区间、金额模式、风险提示和池子存在性状态，并在失败后保留当前输入供用户继续调整或重试。

#### Scenario: preview 展示归一化摘要
- **WHEN** 用户完成 preview
- **THEN** 系统 SHALL 展示归一化后的 token 顺序、fee、`tick_spacing`、区间、金额摘要与风险提示

#### Scenario: 已存在池子时阻止执行
- **WHEN** preview 返回目标池已存在
- **THEN** 系统 MUST 在界面中阻止用户继续提交 execute

#### Scenario: 执行失败后保留输入
- **WHEN** execute 失败
- **THEN** 系统 SHALL 保留当前协议、区间、金额模式与 token 输入，便于用户继续调整或重试
