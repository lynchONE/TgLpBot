## ADDED Requirements

### Requirement: Smart Money 告警中心界面
WebApp 与 MiniApp 的 Smart Money 模块 MUST 提供一个更紧凑的 `金狗通知` 告警中心界面。

该界面 MUST 同时展示：
- Bark 当前状态摘要
- 聪明钱聚集模式配置区
- 池子参数模式配置区
- 每种模式的通知强度选择器
- 每种模式的测试按钮

该界面 MUST 优先在首屏展示每种模式的核心控制项，不得依赖长滚动才能完成常见配置。

#### Scenario: 用户打开金狗通知页
- **WHEN** 用户在 WebApp 或 MiniApp 中打开 Smart Money 的 `金狗通知`
- **THEN** 页面以紧凑卡片或等价紧凑布局同时展示两种模式的摘要、开关、强度与主要输入项

### Requirement: 聪明钱聚集模式
系统 MUST 保留现有按交易对聚集聪明钱 LP 活跃度的告警模式。

该模式 MUST 支持以下配置项：
- `enabled`
- `min_wallets`
- `window_minutes`
- `cooldown_minutes`
- `notification_intensity`

该模式 MUST 在满足交易对聚合阈值后按模式冷却时间去重，并发送 Bark 通知。

#### Scenario: 聪明钱聚集模式触发
- **GIVEN** 聪明钱聚集模式已开启，且 `min_wallets = 3`
- **WHEN** 某个交易对在统计窗口内达到至少 3 个有效聪明钱 LP 活跃计数
- **THEN** 系统发送一条 Bark 通知，并为该模式写入冷却去重状态

### Requirement: 池子参数模式
系统 MUST 支持基于 PoolM 快照字段的池子参数监控模式。

该模式 MUST 支持以下可选阈值字段：
- `total_fees`
- `transaction_count`
- `current_pool_value`
- `total_volume`
- `poolm_fee_rate`
- `active_liquidity_ratio`

阈值为空时 MUST 视为“不参与过滤”。
当用户同时填写多个阈值时，系统 MUST 以 AND 逻辑筛选池子。
该模式 MUST 仅对新鲜 PoolM 快照生效。

#### Scenario: 池子参数模式按多阈值触发
- **GIVEN** 池子参数模式已开启，且用户设置了 `transaction_count` 与 `current_pool_value` 的最小阈值
- **WHEN** 某个池子的最新快照同时满足这两个阈值
- **THEN** 系统发送一条 Bark 通知，并按该池子写入模式级冷却去重状态

#### Scenario: 陈旧快照不触发
- **GIVEN** 池子参数模式已开启
- **WHEN** 某个池子的最新快照已超过允许的新鲜窗口
- **THEN** 该池子 MUST NOT 触发 Bark 通知

### Requirement: Bark 强度与测试通知
系统 MUST 为每种告警模式提供独立的 Bark 通知强度配置，并支持测试发送。

系统 MUST 至少支持以下三档强度：
- `ring`
- `persistent_ring`
- `critical_ring`

测试发送 MUST 不依赖真实扫描结果，也 MUST NOT 修改已保存的告警状态。

#### Scenario: 用户测试持续响铃
- **WHEN** 用户为某个模式选择 `persistent_ring` 并点击测试按钮
- **THEN** 后端立即向该用户发送一条 Bark 测试通知，并应用 `persistent_ring` 对应的 Bark 参数映射

#### Scenario: Bark 未就绪时测试失败
- **GIVEN** 用户尚未配置可用的 Bark Key 或 Bark 未启用
- **WHEN** 用户点击测试按钮
- **THEN** 后端返回失败结果，且不会写入任何告警冷却状态

### Requirement: 金狗通知配置与测试 API
后端 MUST 提供金狗通知配置读取、保存与测试发送接口。

这些接口 MUST：
- 要求 Telegram WebApp `initData` 认证
- 要求 MiniApp 权限校验
- 返回 Bark 配置状态摘要
- 返回双模式配置结构

#### Scenario: 成功读取配置
- **WHEN** 已授权用户请求金狗通知配置
- **THEN** 后端返回 HTTP `200`，其中包含 Bark 状态摘要和双模式配置

#### Scenario: 成功测试发送
- **WHEN** 已授权用户提交一组合法的模式草稿并请求测试发送
- **THEN** 后端返回 HTTP `200`，并触发一次与草稿强度一致的 Bark 测试通知
