## ADDED Requirements
### Requirement: 池子列表 MUST 展示非稳定币代币风控信息
池子列表接口 MUST 对池子中的非 BNB/WBNB/USDT/USDC 及其他稳定币代币查询 OKX advanced-info，并返回风险等级、貔貅盘标记、低流动性标记与可读风险提示。

#### Scenario: 池子包含普通 meme token
- **WHEN** 客户端请求池子列表且池子交易对为 `TOKEN/WBNB`
- **THEN** 服务端查询 `TOKEN` 的 OKX advanced-info
- **AND** 响应中的池子项包含 `token_risk`

#### Scenario: 池子仅包含基础资产
- **WHEN** 客户端请求池子列表且池子交易对为 `WBNB/USDT`
- **THEN** 服务端不查询 OKX advanced-info
- **AND** 响应中的池子项不包含 `token_risk`

#### Scenario: OKX 风控查询失败
- **WHEN** OKX advanced-info 查询失败
- **THEN** 服务端 MUST 在 `token_risk` 中暴露未知风险与查询失败提示
- **AND** 不得把失败结果伪装成低风险或安全

### Requirement: 双端池子 UI MUST 展示风险等级
WebApp 与 MiniApp 的池子列表 MUST 在已有池子摘要中展示代币风险等级；当存在貔貅盘或低流动性标签时 MUST 展示更醒目的风险状态。

#### Scenario: 风险等级为高
- **WHEN** 池子项返回 `token_risk.risk_control_level >= 4`
- **THEN** WebApp 与 MiniApp MUST 在池子列表显示高风险标识

#### Scenario: 标记为貔貅盘
- **WHEN** 池子项返回 `token_risk.has_honeypot = true`
- **THEN** WebApp 与 MiniApp MUST 明确展示貔貅盘提示
