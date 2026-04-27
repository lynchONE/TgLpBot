## ADDED Requirements

### Requirement: Pools 同步必须支持数据库管理的数据源池
系统 MUST 支持通过 MySQL 管理池子同步数据源，并允许同一链和时间窗口下配置多个 enabled 数据源，其中最多只有一个数据源处于 current 状态。

#### Scenario: 数据库没有配置数据源
- **WHEN** `pool_sync` 启动或执行同步且数据库中没有任何池子数据源
- **THEN** 系统继续使用现有 `POOLS_SYNC_POOLM_BASE_URL` 或默认 PoolM 地址执行同步
- **AND** 不要求管理员先创建数据库数据源

#### Scenario: 管理员配置当前数据源
- **WHEN** 管理员将某个 enabled 数据源切换为 current
- **THEN** 下一轮 `pool_sync` 使用该数据源抓取池子快照
- **AND** 同一链和时间窗口下其它数据源不再是 current

### Requirement: Pools 同步必须归一化不同来源的响应字段
系统 MUST 将 PoolM `top-fees/5` 与备用 `market/pools` 来源归一化为同一内部池子快照格式，后续入库逻辑 SHALL 不依赖上游原始字段命名。

#### Scenario: 来源返回 PoolM snake_case 字段
- **WHEN** 数据源返回 `pool_address`、`fee_rate`、`current_pool_value` 等字段
- **THEN** 系统正确写入 `pools.address`、`poolm_fee_rate`、`current_pool_value` 等现有字段

#### Scenario: 来源返回 market/pools camelCase 字段
- **WHEN** 数据源返回 `poolAddress`、`feeRate`、`currentPoolValue` 等字段
- **THEN** 系统将这些字段归一化为现有 `pools` 写入字段
- **AND** 不因为字段命名不同而丢弃该池子

#### Scenario: v4 来源只返回 poolId
- **WHEN** v4 池子返回 `poolAddress=null` 且 `poolId` 有值
- **THEN** 系统使用 `poolId` 作为池子主标识写入 `pools.id` 和 `pools.address`
- **AND** 读接口继续以 `pool_address` 返回该 v4 池子的主标识

### Requirement: 管理员必须能够管理池子数据源
系统 MUST 提供管理员专用接口和 MiniApp 管理入口，用于查看、新增、切换、启用、禁用、删除和检查池子数据源。

#### Scenario: 非管理员访问数据源管理
- **WHEN** 非管理员请求池子数据源管理接口
- **THEN** 系统返回 forbidden
- **AND** 不执行任何数据源变更

#### Scenario: 管理员新增备用来源
- **WHEN** 管理员提交名称、来源类型、base URL、链、窗口、协议和 DEX 参数
- **THEN** 系统创建一个 enabled 数据源
- **AND** 返回更新后的数据源列表和当前来源信息

#### Scenario: 管理员切换当前来源
- **WHEN** 管理员请求切换到某个 enabled 数据源
- **THEN** 系统将该数据源设置为 current
- **AND** 该链和时间窗口下原 current 数据源被取消 current

### Requirement: Pools 同步必须记录数据源健康状态
系统 MUST 在数据源检查或同步失败时记录最后检查时间、最后成功时间、延迟和错误摘要，并在管理员界面展示。

#### Scenario: 当前来源同步失败
- **WHEN** 当前数据源请求失败或响应无法归一化
- **THEN** 系统记录该来源的 `last_error` 和 `last_checked_at`
- **AND** 不清空现有 `pools` 快照数据

#### Scenario: 来源检查成功
- **WHEN** 管理员对某个数据源执行连通性检查且响应可归一化
- **THEN** 系统记录 `last_success_at` 和 `last_latency_ms`
- **AND** 管理接口返回关键字段覆盖情况
