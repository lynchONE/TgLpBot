## ADDED Requirements
### Requirement: 池子列表 MUST 展示非稳定币代币风控信息
池子列表接口 MUST 对池子中的非 BNB/WBNB/USDT/USDC 及其他稳定币代币返回 OKX 风控信息快照，包含风险等级、貔貅盘标记、低流动性标记与可读风险提示。池子列表接口 MUST 优先读取本地持久化快照，不得在每次列表刷新时对所有代币同步调用 OKX advanced-info。

#### Scenario: 池子包含普通 meme token
- **WHEN** 客户端请求池子列表且池子交易对为 `TOKEN/WBNB`
- **THEN** 服务端返回 `TOKEN` 的 `token_risk` 快照
- **AND** 当快照缺失或过期时，服务端将该代币加入后台刷新队列

#### Scenario: 池子仅包含基础资产
- **WHEN** 客户端请求池子列表且池子交易对为 `WBNB/USDT`
- **THEN** 服务端不查询 OKX advanced-info
- **AND** 响应中的池子项不包含 `token_risk`

#### Scenario: OKX 风控查询被限流
- **WHEN** OKX advanced-info 返回 429 或 too many request
- **THEN** 服务端 MUST 保存或保留明确的未知/限流状态
- **AND** 后续池子列表刷新 MUST 在限流 TTL 内复用数据库快照，不得立即重复请求 OKX
- **AND** 不得把失败结果伪装成低风险或安全

### Requirement: 代币风控快照 MUST 持久化
系统 MUST 按 `chain + token_address` 持久化 OKX 代币风控快照，并保存下次刷新时间，供池子列表、搜索结果和开仓链路复用。

#### Scenario: 风控查询成功
- **WHEN** OKX advanced-info 返回代币风控数据
- **THEN** 服务端 MUST 写入或更新 `token_risk_snapshots`
- **AND** 后续同一代币的池子列表响应 MUST 使用该快照，直到需要后台刷新

#### Scenario: 风控快照过期
- **WHEN** 池子列表读取到已过期的风控快照
- **THEN** 服务端 MUST 先返回已有快照
- **AND** 服务端 MUST 将该代币加入后台限速刷新队列

### Requirement: 双端池子 UI MUST 展示风险等级
WebApp 和 MiniApp 的池子列表 MUST 在已有池子摘要中展示代币风险等级；当存在貔貅盘或低流动性标签时 MUST 展示更醒目的风险状态。

#### Scenario: 风险等级为高
- **WHEN** 池子项返回 `token_risk.risk_control_level >= 4`
- **THEN** WebApp 和 MiniApp MUST 在池子列表显示高风险标识

#### Scenario: 标记为貔貅盘
- **WHEN** 池子项返回 `token_risk.has_honeypot = true`
- **THEN** WebApp 和 MiniApp MUST 明确展示貔貅盘提示
