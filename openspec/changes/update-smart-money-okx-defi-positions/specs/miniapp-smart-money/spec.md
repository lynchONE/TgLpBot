## ADDED Requirements

### Requirement: MiniApp 聪明钱钱包详情必须展示 OKX DeFi 多链仓位概览
MiniApp SHALL 在聪明钱钱包详情页展示来自 OKX DeFi 的钱包当前 DeFi 仓位概览。概览 MUST 至少包含总金额、各链金额、平台/协议名称、平台金额、数据来源、更新时间和状态。

#### Scenario: 查看 MiniApp 钱包详情 OKX DeFi 概览
- **WHEN** 用户打开 MiniApp 聪明钱钱包详情页
- **THEN** 页面 MUST 请求并展示该钱包的 OKX DeFi 概览
- **AND** 页面 MUST 展示各链金额和平台金额
- **AND** 页面 MUST 标明数据来源为 OKX DeFi

#### Scenario: OKX DeFi 概览不可用
- **WHEN** OKX DeFi 概览接口返回错误、超时或不可用状态
- **THEN** MiniApp MUST 展示明确的状态文案
- **AND** MUST NOT 将失败结果静默显示为 0 仓位

### Requirement: MiniApp 特别关注钱包必须展示 OKX DeFi 仓位概况
MiniApp SHALL 在特别关注钱包视图中展示关注钱包的 OKX DeFi 总额与链维度概况，并允许进入钱包详情查看平台列表。

#### Scenario: 查看特别关注钱包的 OKX DeFi 概况
- **GIVEN** 用户已经添加一个或多个特别关注钱包
- **WHEN** 用户进入 MiniApp 聪明钱特别关注视图
- **THEN** 每个可用的钱包条目 SHOULD 展示 OKX DeFi 总额
- **AND** SHOULD 展示该钱包有仓位的链维度摘要

### Requirement: MiniApp OKX DeFi 持仓必须支持点击查看详情
MiniApp SHALL 允许用户点击某个 OKX DeFi 平台或持仓查看详情。详情 MUST 突出显示手续费、仓位金额、区间、投资品名称、链、平台和 position 列表。

#### Scenario: 点击 MiniApp OKX DeFi 持仓详情
- **WHEN** 用户点击钱包详情中的某个 OKX DeFi 平台或持仓
- **THEN** MiniApp MUST 按需请求 OKX DeFi 平台详情
- **AND** MUST 展示手续费、仓位金额和区间字段
- **AND** MUST 保留平台详情接口返回的更新时间或状态信息
