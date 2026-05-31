## ADDED Requirements

### Requirement: WebApp 聪明钱钱包详情必须展示 OKX DeFi 多链仓位概览
WebApp SHALL 在聪明钱钱包详情页展示来自 OKX DeFi 的钱包当前 DeFi 仓位概览。概览 MUST 至少包含总金额、各链金额、平台/协议名称、平台金额、数据来源、更新时间和状态。

#### Scenario: 查看钱包详情的 OKX DeFi 概览
- **WHEN** 用户打开 WebApp 聪明钱钱包详情页
- **THEN** 页面 MUST 请求并展示该钱包的 OKX DeFi 概览
- **AND** 页面 MUST 展示各链金额和平台金额
- **AND** 页面 MUST 标明数据来源为 OKX DeFi

#### Scenario: OKX DeFi 概览加载失败
- **WHEN** OKX DeFi 概览接口返回错误或超时
- **THEN** WebApp MUST 展示明确的加载失败状态
- **AND** MUST NOT 将失败结果静默显示为 0 仓位

### Requirement: WebApp 特别关注钱包必须展示 OKX DeFi 仓位概况
WebApp SHALL 在聪明钱特别关注钱包视图中展示关注钱包的 OKX DeFi 总额与链维度概况，并允许进入钱包详情查看平台列表。

#### Scenario: 查看特别关注钱包 OKX DeFi 概况
- **GIVEN** 用户已经添加一个或多个特别关注钱包
- **WHEN** 用户进入 WebApp 聪明钱特别关注视图
- **THEN** 每个可用的钱包条目 SHOULD 展示 OKX DeFi 总额
- **AND** SHOULD 展示该钱包有仓位的链维度摘要

### Requirement: WebApp OKX DeFi 持仓必须支持点击查看详情
WebApp SHALL 允许用户点击某个 OKX DeFi 平台或持仓查看详情。详情 MUST 突出显示手续费、仓位金额、区间、投资品名称、链、平台和 position 列表。

#### Scenario: 点击 OKX DeFi 持仓查看详情
- **WHEN** 用户点击钱包详情中的某个 OKX DeFi 平台或持仓
- **THEN** WebApp MUST 按需请求 OKX DeFi 平台详情
- **AND** MUST 展示手续费、仓位金额和区间字段
- **AND** MUST 保留平台详情接口返回的更新时间或状态信息
