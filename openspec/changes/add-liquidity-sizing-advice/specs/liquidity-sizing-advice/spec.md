## ADDED Requirements

### Requirement: 系统 MUST 提供结构化的仓位规模建议结果
系统 MUST 在现有开仓预检流程中提供结构化的仓位规模建议结果，而不要求用户额外手填价格、tick、活跃流动性和目标占比配置。

系统 MUST 自动解析建议所需的关键输入，包括：
- 当前价格或 `current_tick`
- 用户请求区间对应的 `tick_lower` / `tick_upper`
- 当前区间活跃流动性估值 `L_active`
- 默认目标占比范围

接口返回 MUST 至少包含：
- `recommended_positions`
- `warnings`

`recommended_positions` 中 MUST 固定包含以下三个模式：
- `conservative`
- `neutral`
- `aggressive`

每个推荐项 MUST 包含：
- `mode`
- `liquidity_to_add`
- `expected_share`
- `risk_exposure`
- `efficiency`

#### Scenario: 用户请求三档建议
- **GIVEN** 用户按现有开仓方式提供了合法的金额、区间和滑点参数
- **WHEN** 系统执行仓位规模建议计算
- **THEN** 系统 MUST 返回 `conservative`、`neutral`、`aggressive` 三档建议
- **AND** 每一档 MUST 返回推荐金额、预期占比、风险暴露和效率等级

### Requirement: 系统 MUST 按统一占比公式反推建议金额并输出实际占比
系统 MUST 使用以下公式计算目标占比和建议金额：
- `share = user_liquidity / (L_active + user_liquidity)`
- `user_liquidity = L_active * target_share / (1 - target_share)`

系统 MUST 先基于档位目标占比反推建议金额，再按最终应用后的目标占比输出 `expected_share`。

#### Scenario: 常规档位按活跃流动性直接反推
- **GIVEN** 某一档位的目标占比位于允许区间内
- **WHEN** 系统生成该档位建议
- **THEN** 系统 MUST 基于 `L_active` 与目标占比直接计算 `liquidity_to_add`
- **AND** MUST 返回与该推荐金额对应的 `expected_share`

### Requirement: 系统 MUST 控制 80% 以上目标占比的边际效应
当任一档位的目标占比请求高于 `0.8` 时，系统 MUST 将该档位的计算目标强制截断为 `0.8`，并输出边际效率警告。

当用户请求进入 `> 80%` 占比区间时，系统 MUST 将该档位的 `efficiency` 标记为 `low`，并明确说明其属于低效率区间。

#### Scenario: 用户请求高于 80% 的激进占比
- **GIVEN** 用户输入的目标占比范围或某档默认目标会使档位目标高于 `0.8`
- **WHEN** 系统执行建议计算
- **THEN** 系统 MUST 使用 `0.8` 作为该档位的最高计算目标
- **AND** MUST 在 `warnings` 中写入边际效率下降提示
- **AND** MUST 将该档位标记为 `low` 效率

### Requirement: 系统 MUST 允许建议结果独立于钱包余额生成
系统 MUST 不依赖当前钱包 stable 余额、总资金上限或比例风控去截断 `recommended_positions` 中的推荐金额。

建议结果 MUST 表示“达到目标占比所需的理论规模建议”，而不是“当前钱包一定能够直接执行的金额”。

#### Scenario: 钱包余额低于理论建议金额
- **GIVEN** 某一档位按目标占比反推得到的理论建议金额高于当前钱包可用余额
- **WHEN** 系统生成建议结果
- **THEN** 系统 MUST 仍返回按活跃流动性计算出的理论建议金额
- **AND** MUST NOT 因钱包余额不足而把三档建议压缩为同一个金额

### Requirement: 系统 MUST 自动解析建议输入并允许 best-effort 回退
系统 MUST 在不增加用户输入负担的前提下，自动从现有开仓上下文、池子快照、链上读数和配置中解析建议输入。

系统 MUST 至少支持以下自动来源：
- current tick / pool price：链上或池子快照
- `tick_lower` / `tick_upper`：由用户输入区间百分比反推
- `L_active`：优先池子快照中的活跃流动性美元值，失败时回退到其他池子流动性估值来源
- 目标占比范围：默认配置

当部分输入暂时无法解析时，系统 SHOULD 对建议结果做 best-effort 降级，并 MUST 通过 `warnings` 标明数据来源或缺失原因。

#### Scenario: 活跃流动性快照缺失时回退其他来源
- **GIVEN** 池子快照中没有可用的 `active_liquidity_usd`
- **WHEN** 系统执行建议计算
- **THEN** 系统 MUST 尝试使用其他可用的池子流动性估值来源继续计算
- **AND** MUST 在 `warnings` 中标记发生了数据回退

### Requirement: 系统 MUST 返回可解释的风险暴露和计算说明
系统 MUST 返回 `risk_exposure`，用于表示该档位在最保守估计下的最大可能单边资产暴露。

在 V1 中，当输入仅提供 U 计价的区间流动性时，系统 MUST 允许将 `risk_exposure` 近似为该档位最终的 `liquidity_to_add`，并在计算说明中明确这是保守近似而非链上精确拆分。

系统 SHOULD 返回规范化后的关键输入和每档计算摘要，以便调用方解释推荐结果。

#### Scenario: 调用方需要解释结果来源
- **GIVEN** 调用方拿到了三档加仓建议结果
- **WHEN** 调用方展示或记录这些结果
- **THEN** 返回值 MUST 能说明每档建议的目标占比、最终推荐金额和是否发生截断
- **AND** 风险暴露口径 MUST 被标记为保守近似
