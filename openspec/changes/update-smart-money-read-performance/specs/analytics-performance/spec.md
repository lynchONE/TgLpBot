## ADDED Requirements
### Requirement: 聪明钱展示接口 MUST 避免在请求链路中执行全量链上刷新
聪明钱活跃池子列表、收益火焰图和仓位详情接口 MUST 将链上校准、批量手续费刷新等高成本操作从用户请求主链路中隔离出来。列表类接口 MUST 优先返回持久化状态和已有快照，不得为了刷新所有过期仓位而阻塞当前响应。

#### Scenario: 收益火焰图存在过期手续费快照
- **GIVEN** 多个活跃仓位的 `fee_updated_at` 已超过刷新间隔
- **WHEN** 客户端请求收益火焰图
- **THEN** 后端 MUST 使用已有快照构建当前响应
- **AND** 后端 MUST 以后台方式触发过期快照刷新
- **AND** 当前响应 MUST NOT 等待全部过期仓位链上刷新完成

#### Scenario: 活跃池子列表触发状态修复窗口
- **GIVEN** 后台活跃仓位状态修复已达到下一次执行时间
- **WHEN** 客户端请求活跃池子列表
- **THEN** 后端 MUST 返回基于持久化状态的池子列表
- **AND** 链上状态修复 MUST NOT 作为该请求的阻塞前置步骤

### Requirement: 聪明钱仓位详情 MUST 使用独立只读 RPC 并发查询
聪明钱仓位详情接口 MUST 使用与交易执行路径隔离的只读 RPC 查询能力读取链上详情。该能力 MUST 能从 RPC 池中选择多个可用 HTTP endpoint，并以有界并发和超时执行只读调用。

#### Scenario: 多个 RPC endpoint 可用
- **GIVEN** RPC 池中同链 HTTP transport 存在多个可用 endpoint
- **WHEN** 用户打开聪明钱仓位详情
- **THEN** 详情读取 MAY 将只读链上调用分散到多个 endpoint
- **AND** 单个 endpoint 限速或延迟 MUST NOT 直接阻塞所有只读调用
- **AND** 开仓、撤仓、交易发送路径 MUST NOT 使用该只读执行器

#### Scenario: RPC 池只有一个可用 endpoint
- **GIVEN** RPC 池中同链 HTTP transport 只有一个可用 endpoint
- **WHEN** 用户打开聪明钱仓位详情
- **THEN** 系统 MUST 继续使用该 endpoint 完成只读查询
- **AND** 行为 MUST 与现有单 RPC 部署兼容

### Requirement: 聪明钱性能定位 MUST 提供阶段耗时观测
聪明钱展示接口 MUST 提供阶段耗时日志或指标，用于区分 SQL 聚合、链上 RPC、后台刷新、元数据加载等耗时来源。日志 MUST 不泄露 RPC URL 中的密钥、用户私钥或其他敏感信息。

#### Scenario: 接口响应超过慢请求阈值
- **WHEN** 任一聪明钱展示接口响应耗时超过配置阈值
- **THEN** 后端 MUST 输出包含接口名、阶段耗时和记录数量的日志
- **AND** 日志 MUST 对 RPC endpoint 做脱敏或只输出 endpoint ID
