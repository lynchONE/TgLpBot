## ADDED Requirements
### Requirement: 开仓前 MUST 展示代币风控检查
开仓 prepare 与 preview 接口 MUST 对目标池子的非稳定/非主流基础代币返回代币风控信息，并把风险信息纳入开仓检查项。

#### Scenario: 打开开仓面板
- **WHEN** 用户在 WebApp 或 MiniApp 打开非稳定币池子的开仓面板
- **THEN** 客户端 MUST 展示代币风险等级、貔貅盘状态、低流动性状态与风险提示

#### Scenario: 代币为貔貅盘
- **WHEN** OKX advanced-info 标记目标代币为 `honeypot`
- **THEN** prepare/preview 检查状态 MUST 为 `fail`
- **AND** 真实开仓接口 MUST 拒绝执行

#### Scenario: 代币为低流动性
- **WHEN** OKX advanced-info 标记目标代币为 `lowLiquidity`
- **THEN** prepare/preview 检查状态 MUST 为 `warn`
- **AND** 客户端 MUST 在开单前展示低流动性提示
