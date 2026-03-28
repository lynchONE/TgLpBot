## ADDED Requirements

### Requirement: 聪明钱钱包接口必须返回钱包总余额
系统 MUST 为聪明钱钱包列表与单钱包详情返回统一的钱包总余额字段 `wallet_balance_usd`，其口径复用现有聪明钱资产服务中的钱包总资产 `TotalUSD`。

#### Scenario: 请求钱包列表
- **WHEN** 客户端调用聪明钱钱包列表接口获取某一页钱包数据
- **THEN** 每个钱包条目都返回 `wallet_balance_usd`
- **AND** 该值表示该钱包当前识别资产口径下的总余额（USD）

#### Scenario: 请求钱包详情
- **WHEN** 客户端调用聪明钱单钱包详情接口获取某个钱包数据
- **THEN** 返回结果包含 `wallet_balance_usd`
- **AND** 该值与同一钱包在钱包列表中的余额口径保持一致

### Requirement: WebApp 和 MiniApp 钱包视图必须展示钱包余额
WebApp 和 MiniApp SHALL 在聪明钱钱包列表与钱包详情页展示钱包余额，并在余额缺失时做降级显示而不影响其他信息渲染。

#### Scenario: 钱包列表展示余额
- **WHEN** 钱包列表接口返回某个钱包的 `wallet_balance_usd`
- **THEN** WebApp 和 MiniApp 的钱包列表都展示该钱包余额

#### Scenario: 钱包详情展示余额
- **WHEN** 用户进入某个聪明钱钱包详情页
- **THEN** WebApp 和 MiniApp 都展示该钱包的余额信息
- **AND** 余额字段缺失时使用 `--` 等占位内容降级显示
