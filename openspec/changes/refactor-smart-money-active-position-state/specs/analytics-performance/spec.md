## ADDED Requirements

### Requirement: Smart Money 活跃仓位查询使用持久化状态表
后端 MUST 为聪明钱“当前活跃仓位”提供独立的持久化状态表，并让查询接口优先读取该状态表，而不是在请求时扫描明细事件并补充链上 RPC 校验。

该状态表 MUST 至少持久化：
- `chain`
- `pool_version`
- `pool_id`
- `wallet_address`
- `contract_address`
- `token_id`
- `tick_lower`
- `tick_upper`
- `current_liquidity`
- `is_active`
- `last_event_seq`
- `updated_at`

#### Scenario: 查询池子活跃钱包
- **WHEN** 客户端请求聪明钱池子钱包列表
- **THEN** 后端 MUST 从活跃仓位状态表读取当前活跃仓位
- **AND** 不再依赖扫描 `smart_lp_events` 明细并逐条补链上 RPC

#### Scenario: 查询钱包当前仓位
- **WHEN** 客户端请求某钱包的聪明钱当前仓位
- **THEN** 后端 MUST 以活跃仓位状态表作为仓位引用真源

### Requirement: SmartLP 采集链路持续维护活跃仓位状态
SmartLP monitor MUST 在写入 `smart_lp_events` 的同时，持续维护对应仓位的活跃状态。

系统 MUST 满足以下约束：
- add 事件写入后，必须增加该仓位的 `current_liquidity`
- remove 事件写入后，必须扣减该仓位的 `current_liquidity`
- 当 `current_liquidity <= 0` 时，必须将该仓位标记为非活跃
- 状态更新 MUST 按 `event_seq` 单调应用，避免旧事件覆盖新状态

#### Scenario: add 事件激活仓位
- **GIVEN** 某仓位当前不存在 active state 或 `current_liquidity=0`
- **WHEN** SmartLP monitor 写入一条 add 事件
- **THEN** 系统 MUST 创建或更新该仓位状态为活跃

#### Scenario: remove 事件关闭仓位
- **GIVEN** 某仓位当前处于活跃状态
- **WHEN** SmartLP monitor 写入使其净 liquidity 归零的 remove 事件
- **THEN** 系统 MUST 将该仓位标记为非活跃

### Requirement: 活跃仓位状态必须具备后台校准能力
系统 MUST 提供不进入用户请求主链路的后台校准机制，用于修正漏采、重组或异常导致的活跃仓位状态漂移。

#### Scenario: 发现状态与链上不一致
- **GIVEN** 后台校准任务检测到某仓位 active state 与链上实际状态不一致
- **WHEN** 校准任务完成修正
- **THEN** 系统 MUST 更新活跃仓位状态表
- **AND** 后续查询结果 MUST 以修正后的状态为准
