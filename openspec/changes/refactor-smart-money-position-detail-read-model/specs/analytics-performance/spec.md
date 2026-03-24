## ADDED Requirements

### Requirement: Smart Money 仓位详情必须使用 MySQL 持久化读模型
系统 MUST 为聪明钱当前仓位详情提供独立的 MySQL 持久化读模型，而不是依赖 ClickHouse 或请求时链上 live resolve。

该读模型 MUST 至少持久化：
- `chain_id`
- `protocol`
- `wallet_address`
- `pool_address`
- `nft_token_id`
- `tick_lower`
- `tick_upper`
- `token0_address`
- `token1_address`
- `token0_symbol`
- `token1_symbol`
- `token0_decimals`
- `token1_decimals`
- `fee_tier`
- `tick_spacing`
- `current_liquidity`
- `is_active`
- `entry_total_usd`
- `net_total_usd`
- `fee_usd`
- `fee_status`
- `fee_updated_at`

#### Scenario: add 事件创建或更新 active row
- **WHEN** Smart Money watcher 写入一条 add 事件
- **THEN** 系统 MUST 在同一事务内创建或更新对应的 active position 读模型

#### Scenario: remove 事件关闭 active row
- **WHEN** Smart Money watcher 写入一条使净 liquidity 归零的 remove 事件
- **THEN** 系统 MUST 将对应 active position 标记为 `is_active=false`
- **AND** MUST 将 `current_liquidity` 归零

### Requirement: Smart Money 详情查询必须复用持久化元数据并最小化链上读取
聪明钱仓位详情查询 MUST 优先读取 `sm_lp_active_positions` 中持久化的池子 / token / manager / tick 区间等元数据，并仅对真正需要实时性的字段发起链上读取。

请求路径 MUST NOT 为了以下目的重复发起元数据类 RPC：
- 反查池子地址 / poolId
- 反查 token 地址、symbol、decimals
- 反查 tickSpacing、feeTier、position manager 关联关系

#### Scenario: 详情页轮询刷新
- **GIVEN** 前端正在轮询某个聪明钱仓位详情
- **WHEN** 后端处理详情请求
- **THEN** 系统 MUST 从持久化读模型中读取仓位元数据
- **AND** 仅对实时字段发起必要的链上读取
- **AND** MUST NOT 因轮询而重复做池子 / token 元数据反查

### Requirement: Smart Money 实时字段读取必须具备缓存与降级
聪明钱仓位详情中的高成本实时字段 MUST 具备缓存、并发去重和降级能力，避免轮询请求直接放大 RPC 压力。

#### Scenario: 手续费快照可用
- **GIVEN** 某活跃仓位已有最近一次成功的实时字段快照
- **WHEN** 客户端请求仓位详情
- **THEN** 接口 MAY 返回该快照值
- **AND** MUST 返回对应状态字段

#### Scenario: 手续费快照缺失或过期
- **GIVEN** 某活跃仓位的实时字段链上读取失败
- **WHEN** 客户端请求仓位详情
- **THEN** 接口 MUST 返回明确的 `fee_status` 或 `warnings`
- **AND** SHOULD 优先回退到最近一次成功快照，而不是直接失败
