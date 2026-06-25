## ADDED Requirements
### Requirement: 钱包一键兑换 MUST 返回 Binance 聚合多路由报价
系统 MUST 在钱包单币兑换报价中使用 Binance Web3 Wallet Trading API 的聚合报价能力，并向 WebApp 与 MiniApp 返回多个可比较、可展示、可选择的 route。系统 MUST NOT 在该报价链路中返回 0x 或 LI.FI 报价。

#### Scenario: Binance 返回多个聚合路由
- **WHEN** 用户在 WebApp 或 MiniApp 发起一笔链内钱包单币兑换报价
- **THEN** 后端 MUST 调用 Binance `Get Aggregated Quote`
- **AND** 响应 MUST 包含 Binance 聚合返回的多个 route
- **AND** 每个 route MUST 包含 route 标识、预计到账、最少到账、预计 Gas、可执行状态和路由摘要
- **AND** WebApp 与 MiniApp MUST 展示这些 route，并允许用户区分不同 route

#### Scenario: 旧 0x 与 LI.FI 报价不再出现
- **WHEN** 用户请求钱包单币兑换报价
- **THEN** 后端 MUST NOT 调用 0x 或 LI.FI 报价接口
- **AND** 响应 MUST NOT 包含 provider 为 `0x`、`li.fi` 或 `lifi` 的报价项

### Requirement: 钱包一键兑换 MUST 支持按 Binance route 执行
系统 MUST 允许用户基于已展示的 Binance 聚合 route 选择执行兑换，并在执行前使用 Binance `Build Swap Transaction` 重新构造交易数据。

#### Scenario: 用户选择 Binance route 后执行兑换
- **GIVEN** 页面已展示 Binance 聚合报价中的多个 route
- **WHEN** 用户选择其中一个 route 并确认兑换
- **THEN** 客户端 MUST 将选定 route 的后端认可标识提交给执行接口
- **AND** 后端 MUST 调用 Binance `Build Swap Transaction` 重新构造交易
- **AND** 后端 MUST 校验交易链、钱包、token、amount、spender、tx.to、tx.value 和 tx.data 后再发送交易
- **AND** 成功响应 MUST 返回最终使用的 provider、route 标识、交易哈希、交易链接和实际到账数量

#### Scenario: route 过期或构造交易失败
- **GIVEN** 用户选择的 Binance route 已过期或不再可执行
- **WHEN** 用户确认兑换
- **THEN** 后端 MUST 返回明确失败原因
- **AND** 后端 MUST NOT silent fallback 到其他 route、0x 或 LI.FI
- **AND** WebApp 与 MiniApp MUST 保留用户输入并提示刷新报价

### Requirement: 钱包一键兑换 MUST 拒绝旧 provider 执行
系统 MUST 在钱包单币兑换执行接口拒绝 0x 与 LI.FI provider，避免旧客户端或旧状态继续触发已移除渠道。

#### Scenario: 旧客户端提交 0x 或 LI.FI provider
- **WHEN** 执行接口收到 `provider=0x`、`provider=li.fi` 或 `provider=lifi`
- **THEN** 后端 MUST 返回不支持该 provider 的错误
- **AND** MUST NOT 自动 fallback 到 Binance、OKX 或其他 provider
