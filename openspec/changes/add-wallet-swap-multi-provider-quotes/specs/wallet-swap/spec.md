## ADDED Requirements
### Requirement: 钱包一键兑换 MUST 返回多提供商统一报价
系统 MUST 在同一次钱包单币兑换报价请求中返回 OKX、0x 和 LI.FI 的统一报价结果，允许前端在同一页面中并列展示与比较。

#### Scenario: 多个 provider 同时返回报价
- **WHEN** 用户在 `webapp` 为某个钱包发起一笔同链单币兑换报价
- **THEN** 后端 MUST 在单次响应中返回 OKX、0x 和 LI.FI 的报价集合
- **AND** 每个报价 MUST 包含 provider 标识、预估到账、最小到账、预估 Gas、可执行状态和更新时间
- **AND** 每个报价 MUST 包含可供页面展示的交易路径摘要

#### Scenario: 仅部分 provider 可用
- **WHEN** 某个 provider 因链不支持、无流动性或接口失败而无法生成报价
- **THEN** 后端 MUST 继续返回其他可用 provider 的报价
- **AND** MUST 为不可用 provider 返回明确状态或错误原因

### Requirement: 钱包一键兑换 MUST 按净到手口径展示 provider 报价
系统 MUST 将 provider 手续费规则纳入报价展示与比较逻辑，向用户返回可直接比较的净到手结果，而不是仅返回未扣费的裸路由值。

#### Scenario: 固定比例手续费 provider 报价
- **WHEN** 系统请求 0x 与 LI.FI 的报价
- **THEN** 系统 MUST 将 0x 交易额 `0.15%` 与 LI.FI 交易额 `0.25%` 的手续费纳入报价口径
- **AND** MUST 返回对应的手续费明细或费率说明
- **AND** 前端 MUST 以净到手金额展示并比较这些报价

#### Scenario: 正滑点手续费规则 provider 报价
- **WHEN** 系统展示 OKX 报价
- **THEN** 系统 MUST 明示“正滑点部分收取 `10%`”这一手续费规则
- **AND** MUST 以用户可承诺获得的到账值参与报价比较
- **AND** MUST NOT 将不可保证的正滑点收益计入默认排序

### Requirement: 钱包一键兑换 MUST 支持按 provider 执行
系统 MUST 允许用户基于已展示的 provider 报价选择 OKX、0x 或 LI.FI 执行兑换，并在执行结果中返回最终使用的 provider。

#### Scenario: 用户按选定 provider 提交兑换
- **WHEN** 用户在 `webapp` 选中某个 provider 并确认提交兑换
- **THEN** 执行接口 MUST 按该 provider 重新获取可执行交易数据并发起交易
- **AND** MUST 在成功响应中返回最终执行的 provider 标识
- **AND** 前端 MUST 在确认弹窗和成功反馈中展示最终执行的 provider

#### Scenario: 选定 provider 在执行前失效
- **WHEN** 用户确认提交时所选 provider 的报价已过期或重新取价失败
- **THEN** 后端 MUST 返回明确失败原因
- **AND** 前端 MUST 保留用户输入并提示重新获取报价
