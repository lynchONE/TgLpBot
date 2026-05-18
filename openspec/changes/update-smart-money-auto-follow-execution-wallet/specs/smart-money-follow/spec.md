## ADDED Requirements

### Requirement: 自动跟单配置支持执行钱包
系统 SHALL 允许用户在自动跟单配置中指定一个属于自己的执行钱包，并在创建跟单开仓任务时使用该钱包写入 `StrategyTask.wallet_id` 与 `StrategyTask.wallet_address`。

#### Scenario: 用户选择执行钱包
- **WHEN** 用户保存自动跟单配置并选择合法的执行钱包
- **THEN** 系统 MUST 将该钱包 ID 与地址保存到自动跟单配置
- **AND** 新创建的跟单开仓任务 MUST 使用该执行钱包开仓

#### Scenario: 执行钱包不属于当前用户
- **WHEN** 用户保存的执行钱包 ID 不属于当前用户
- **THEN** 系统 MUST 拒绝保存配置并返回错误
- **AND** MUST NOT 使用其他钱包或默认钱包静默代替

#### Scenario: 历史配置未设置执行钱包
- **GIVEN** 已存在的自动跟单配置没有执行钱包字段值
- **WHEN** 系统执行该历史配置创建的跟单任务
- **THEN** 系统 MUST 继续使用用户默认钱包保持兼容
- **AND** 接口 MUST 返回解析后的执行钱包信息，便于前端明确展示
