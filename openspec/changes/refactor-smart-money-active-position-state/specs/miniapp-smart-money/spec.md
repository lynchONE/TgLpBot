## MODIFIED Requirements

### Requirement: Smart Money 钱包仓位接口按 tokenId 统一查询 V3/V4 当前仓位
`GET /api/smart_money_wallet_positions` MUST 优先使用持久化的活跃仓位状态作为当前仓位查询入口，而不是在请求链路中依赖链上 RPC 先判断仓位是否仍然活跃。

系统 MUST 满足以下约束：
- 当前活跃仓位真源 MUST 为持久化状态表，而不是临时扫描 `smart_lp_events` 明细后再做 live 过滤
- 对于 `is_active=false` 或 `current_liquidity<=0` 的仓位，接口 MUST 不再返回给前端
- 当接口需要构建当前仓位详情时，SHOULD 直接从 active state 提供的仓位引用出发
- 请求路径 MAY 继续做金额、手续费等明细补充，但 MUST NOT 以“是否活跃”为目的对每条结果执行链上 RPC 校验

#### Scenario: 钱包详情查询当前活跃仓位
- **GIVEN** 某钱包在持久化 active state 中存在 `is_active=true` 的仓位记录
- **WHEN** 客户端请求 `GET /api/smart_money_wallet_positions`
- **THEN** 接口 MUST 基于该 active state 返回当前仓位
- **AND** 不得为了确认“这条仓位是否活跃”再逐条发起链上 RPC

#### Scenario: 钱包详情过滤已关闭仓位
- **GIVEN** 某仓位在 `smart_lp_events` 中仍有历史 add 事件
- **AND** 该仓位在 active state 中已被标记为 `is_active=false`
- **WHEN** 客户端请求 `GET /api/smart_money_wallet_positions`
- **THEN** 接口 MUST 不返回该仓位

### Requirement: Smart Money 池子详情接口使用同一仓位引用模型
`GET /api/smart_money_pool_adds` MUST 从持久化 active state 读取当前池子的活跃钱包与仓位，而不是在请求时先展示事件聚合行，再靠点开详情触发链上校验剔除空壳行。

系统 MUST 满足以下约束：
- 钱包列表数据源 MUST 只包含 active state 中 `is_active=true` 的仓位
- 前端列表中不得出现“钱包行仍展示，但详情提示没有匹配活跃仓位”的常态路径
- 当后台暂时无法提供 active state 时，接口 MUST 明确返回降级告警，而不是静默退回高频 RPC 过滤

#### Scenario: 池子钱包列表只返回活跃钱包
- **GIVEN** 某池子存在历史 add/remove 事件
- **AND** 其中一部分仓位在 active state 中已变为非活跃
- **WHEN** 客户端请求 `GET /api/smart_money_pool_adds`
- **THEN** 接口 MUST 仅返回仍处于活跃状态的钱包行

#### Scenario: active state 不可用时返回明确降级状态
- **GIVEN** active state 表暂时不可用或尚未初始化
- **WHEN** 客户端请求聪明钱池子详情
- **THEN** 接口 MUST 返回明确 warning / error
- **AND** MUST NOT 回退为对每条钱包行逐条发起链上活跃校验
