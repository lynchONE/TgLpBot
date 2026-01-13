## ADDED Requirements

### Requirement: Pool 搜索 API
后端 MUST 提供 `GET /api/search_pools` API，并要求 Telegram WebApp `initData` 认证与 MiniApp 权限校验。

API MUST 支持按以下两类关键字搜索：
- 池子ID：V3 pool address 或 V4 poolId
- 代币名称/符号：按 `trading_pair` 不区分大小写子串匹配

API 返回的数据结构 MUST 与 MiniApp 热门池子卡片兼容（至少包含 `protocol_version`, `pool_address`, `trading_pair`, `current_pool_value` 字段）。

#### Scenario: 代币搜索按 TVL 倒序且限制 10 条
- **WHEN** `q` 为代币名称/符号并命中多个池子
- **THEN** 返回结果按 `current_pool_value` 倒序排序，且最多返回 10 条

#### Scenario: 按池子ID返回单池子
- **WHEN** `q` 为合法的 V3 pool address 或 V4 poolId
- **THEN** API 返回该池子的结果（如可用）

