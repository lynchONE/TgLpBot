## ADDED Requirements

### Requirement: AutoLP 硬筛支持中文交易对开关
系统 MUST 提供系统级配置项 `autolp_filter_chinese_tokens`（默认关闭）。

当 `autolp_filter_chinese_tokens=true` 时，系统 MUST 禁止 AutoLP 对“交易对或代币符号包含中文字符”的池子开单（包括创建开仓任务与作为换仓目标）。

#### Scenario: 开启过滤中文后禁止开单
- **WHEN** `autolp_filter_chinese_tokens=true` 且某候选池的 `trading_pair`（或代币符号）包含中文字符
- **THEN** AutoLP 不会为该池子创建开仓任务，也不会选择其作为换仓目标

### Requirement: AutoLP 硬筛支持费率上限
系统 MUST 提供系统级配置项 `autolp_max_fee_percentage`（默认 `0` 表示不启用）。

当 `autolp_max_fee_percentage > 0` 时，若池子 `fee_percentage > autolp_max_fee_percentage`，系统 MUST 禁止 AutoLP 对该池子开单（包括创建开仓任务与作为换仓目标）。

#### Scenario: 超过费率上限则禁止开单
- **WHEN** `autolp_max_fee_percentage > 0` 且某池 `fee_percentage > autolp_max_fee_percentage`
- **THEN** AutoLP 不会为该池子创建开仓任务，也不会选择其作为换仓目标

### Requirement: MiniApp 可配置系统级硬筛
管理员 MUST 能通过 MiniApp 管理员面板读取与更新系统级硬筛配置项：
- `autolp_filter_chinese_tokens`
- `autolp_max_fee_percentage`

#### Scenario: 管理员更新硬筛后立即生效
- **WHEN** 管理员在 MiniApp 更新 `autolp_filter_chinese_tokens` 或 `autolp_max_fee_percentage`
- **THEN** 后续 AutoLP 扫描/开仓/换仓评估按最新配置执行

