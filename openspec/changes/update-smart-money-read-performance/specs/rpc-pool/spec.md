## ADDED Requirements
### Requirement: RPC 池 MUST 支持聪明钱只读查询枚举多个可用 endpoint
RPC 池 MUST 为只读查询提供按 `chain` 和 `transport` 枚举多个可用 endpoint 的能力。该能力 MUST 遵守现有 endpoint 的禁用状态和健康状态，并不得改变当前 endpoint 选择语义。

#### Scenario: 查询 BSC HTTP 可用只读 endpoint
- **GIVEN** RPC 池中存在多个 `chain=bsc`、`transport=http` 的 endpoint
- **WHEN** 聪明钱只读查询请求枚举可用 endpoint
- **THEN** RPC 池 MUST 返回未被禁用且 URL 有效的 endpoint 列表
- **AND** 该枚举 MUST NOT 修改 `is_current`
- **AND** 全局交易 RPC 当前选择 MUST 保持不变

#### Scenario: 没有 DB endpoint 时兼容环境变量
- **GIVEN** RPC 池没有配置同链 HTTP endpoint
- **WHEN** 聪明钱只读查询请求可用 endpoint
- **THEN** 系统 MUST 继续兼容现有环境变量 RPC 配置
- **AND** 不得因为缺少 DB endpoint 导致聪明钱展示接口整体不可用
