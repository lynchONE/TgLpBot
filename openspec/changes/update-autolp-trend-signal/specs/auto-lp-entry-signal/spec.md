## ADDED Requirements

### Requirement: AutoLP 60m 趋势方向基于均线差
系统 MUST 使用 `MA5` 与 `MA60` 的均线差（`ma_cross_pct=(MA5-MA60)/MA60`）判定 60m 方向为 `UPTREND/DOWNTREND/SIDEWAYS`，而不是仅依赖 `Z60` 的正负作为方向依据。

#### Scenario: MA5 明显低于 MA60 判定为下跌
- **WHEN** `ma_cross_pct <= -entry_trend_cross_pct`
- **THEN** 该池子的 60m 趋势方向为 `DOWNTREND`

#### Scenario: MA5 明显高于 MA60 判定为上涨
- **WHEN** `ma_cross_pct >= entry_trend_cross_pct`
- **THEN** 该池子的 60m 趋势方向为 `UPTREND`

#### Scenario: MA5 与 MA60 接近判定为震荡
- **WHEN** `abs(ma_cross_pct) < entry_trend_cross_pct`
- **THEN** 该池子的 60m 趋势方向为 `SIDEWAYS`

### Requirement: AutoLP 下跌趋势禁止作为开仓候选
当趋势过滤开启时，若某池子的 60m 趋势方向被判定为 `DOWNTREND`，系统 MUST 禁止 AutoLP 将其作为自动开仓候选（即不应创建开仓任务，也不应作为 Top 候选用于自动执行）。

#### Scenario: 下跌趋势下跳过开仓候选
- **WHEN** `trend_filter_enabled=true`
- **AND** 某池的 60m 趋势方向为 `DOWNTREND`
- **THEN** AutoLP 不会将该池标记为可开仓候选

### Requirement: AutoLP 短期下跌动量禁止作为开仓候选
当趋势过滤开启时，若当前价格相对短窗均值 `MA5` 的偏离满足 `dev5_pct=(P-MA5)/MA5*100 <= -entry_block_dev5_pct`，系统 MUST 禁止 AutoLP 将其作为自动开仓候选，用于捕捉“回落/下跌中的横盘误判”。

#### Scenario: 当前价格显著低于 MA5 时跳过开仓候选
- **WHEN** `trend_filter_enabled=true`
- **AND** `dev5_pct <= -entry_block_dev5_pct`
- **THEN** AutoLP 不会将该池标记为可开仓候选

### Requirement: AutoLP 震荡（SIDEWAYS）状态仍允许作为开仓候选
系统 MUST 保留 V1 的可开仓状态集合包含 `SIDEWAYS`；当短期状态为 `SIDEWAYS` 且未触发本提案新增的趋势/动量门禁时，系统 MUST 不因 `SIDEWAYS` 而排除该池子的开仓候选资格（仍需满足硬筛等其他前置条件）。

#### Scenario: SIDEWAYS 且未命中门禁时不应被排除
- **WHEN** `trend_filter_enabled=true`
- **AND** 短期状态为 `SIDEWAYS`
- **AND** 60m 趋势方向不为 `DOWNTREND`
- **AND** `dev5_pct > -entry_block_dev5_pct`
- **THEN** AutoLP 不会因为 `SIDEWAYS` 而排除该池子的开仓候选资格

### Requirement: AutoLP 趋势过滤可配置且可回退
系统 MUST 提供以下系统级配置项：
- `trend_filter_enabled`（默认开启）
- `entry_trend_cross_pct`（默认值由部署侧提供）
- `entry_block_dev5_pct`（默认值由部署侧提供）

#### Scenario: 关闭趋势过滤后沿用旧逻辑
- **WHEN** `trend_filter_enabled=false`
- **THEN** AutoLP 候选开仓的判定不应因本提案新增的趋势/动量门禁而被阻止

### Requirement: MiniApp 可配置 AutoLP 进场门禁
管理员 MUST 能通过 MiniApp 管理员面板读取与更新 AutoLP 进场门禁配置项：
- `autolp_trend_filter_enabled`
- `autolp_entry_trend_cross_pct`
- `autolp_entry_block_dev5_pct`

#### Scenario: 管理员更新后立即生效
- **WHEN** 管理员在 MiniApp 更新 `autolp_trend_filter_enabled` / `autolp_entry_trend_cross_pct` / `autolp_entry_block_dev5_pct`
- **THEN** 后续 AutoLP 扫描/候选评估按最新配置执行
