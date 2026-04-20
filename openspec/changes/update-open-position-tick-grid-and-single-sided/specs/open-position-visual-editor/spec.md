## ADDED Requirements
### Requirement: WebApp 与 MiniApp MUST 提供可视化的区间编辑器
WebApp 与 MiniApp MUST 提供面向 CLMM 的可视化区间编辑器，让用户能够围绕当前价查看离散格子区间，而不是只依赖两个百分比输入框。

#### Scenario: WebApp 显示双栏式区间编辑器
- **WHEN** 用户在桌面端打开开仓弹窗
- **THEN** 界面 SHALL 采用区间编辑器与资金摘要分栏结构，左侧突出区间选择，右侧突出钱包、金额、参数与确认摘要

#### Scenario: MiniApp 显示分步卡片式区间编辑器
- **WHEN** 用户在 MiniApp 打开开仓面板
- **THEN** 界面 SHALL 将池子信息、区间模式、格子编辑、金额参数和确认摘要拆分为适合触控的分步卡片

### Requirement: 区间编辑器 MUST 提供自动与手动两种模式
区间编辑器 MUST 提供自动与手动两种模式，其中自动模式用于推荐区间，手动模式用于精确输入百分比、Tick 或格子数量。

#### Scenario: 自动模式展示快捷区间
- **WHEN** 用户切换到自动模式
- **THEN** 界面 SHALL 提供推荐区间或快捷区间按钮，并允许一键应用到当前编辑器

#### Scenario: 手动模式展示精确编辑能力
- **WHEN** 用户切换到手动模式
- **THEN** 界面 SHALL 提供百分比、Tick 或格子数量的精确编辑入口，并同步回显规范化结果

### Requirement: 界面 MUST 明确展示单边池结果与区间摘要
无论用户通过哪种方式选区间，界面 MUST 在确认前展示这是单边还是双边仓位，以及规范化后的 tick 和价格边界摘要。

#### Scenario: 单边池摘要展示
- **WHEN** 预览结果标记当前区间为 `single_token0` 或 `single_token1`
- **THEN** 界面 SHALL 在摘要区明确展示“单边池”状态，并说明预估偏向的资产侧

#### Scenario: 双边池摘要展示
- **WHEN** 预览结果标记当前区间为 `dual_sided`
- **THEN** 界面 SHALL 在摘要区展示双边状态、价格边界和资产占比摘要

### Requirement: 界面 MUST 适配参考图所体现的信息层级
界面 MUST 参考用户提供的竞品设计，突出“池子信息、区间模式、可视化区间、资金输入、执行参数、确认摘要”这六类信息层级，但允许根据本项目从 USDT 一键开仓的链路做本地化调整。

#### Scenario: 保留 USDT 开仓主输入
- **WHEN** 用户在开仓界面输入资金
- **THEN** 界面 SHALL 继续以 USDT 金额作为主输入，不得强制要求用户手填 token0/token1 双币金额

#### Scenario: 通过摘要解释最终资产配比
- **WHEN** 预览完成
- **THEN** 界面 SHALL 用摘要区向用户解释最终会形成的资产配比与单边/双边结果
