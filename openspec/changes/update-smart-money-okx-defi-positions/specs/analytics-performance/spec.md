## ADDED Requirements

### Requirement: OKX DeFi 详情请求不得阻塞聪明钱列表页
系统 MUST 将 OKX DeFi 平台详情请求限制在用户按需打开钱包详情或持仓详情时执行。列表页不得为每个钱包同步请求所有 OKX 平台详情。

#### Scenario: 打开聪明钱钱包列表
- **WHEN** 用户打开聪明钱钱包列表或特别关注钱包列表
- **THEN** 后端 MUST NOT 同步拉取每个钱包的所有 OKX DeFi 平台详情
- **AND** 页面仍可展示已有本地聪明钱列表数据

### Requirement: OKX DeFi 外部数据必须带状态并使用短 TTL 缓存
系统 SHALL 为 OKX DeFi 平台列表和平台详情读取设置请求超时与短 TTL 缓存。响应 MUST 包含数据状态、来源和更新时间；当 OKX 请求失败时，系统 MUST 返回明确错误或告警状态。

#### Scenario: OKX DeFi 请求失败
- **WHEN** OKX DeFi 外部接口返回错误、限流或超时
- **THEN** 后端 MUST 返回明确的错误或告警状态
- **AND** MUST NOT 用空列表或 0 金额掩盖失败

#### Scenario: 命中 OKX DeFi 短 TTL 缓存
- **GIVEN** 某钱包的 OKX DeFi 概览或平台详情仍在缓存有效期内
- **WHEN** 用户再次请求相同数据
- **THEN** 后端 SHOULD 返回缓存数据
- **AND** MUST 在响应中保留数据来源和更新时间
