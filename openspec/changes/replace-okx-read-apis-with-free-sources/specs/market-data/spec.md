## ADDED Requirements

### Requirement: 价格查询不得依赖 OKX market API
系统的 token 实时价格查询 MUST 使用非 OKX 的免费市场数据源，并 MUST 支持按 `chain + token_address[]` 批量查询。

#### Scenario: 批量查询多个 token 价格
- **WHEN** 前端或服务端请求同一条链上的多个 token 价格
- **THEN** 后端 MUST 尽量合并成 provider 支持的批量请求
- **AND** MUST NOT 为每个 token 单独调用 OKX market price API

#### Scenario: 并发请求同一批价格
- **WHEN** 多个请求在极短时间内查询相同或高度重叠的 token 集
- **THEN** 后端 MUST 合并正在飞行中的相同 provider 请求
- **AND** MAY 复用该次请求结果完成这些并发响应

### Requirement: 实时价格不得被长缓存影响
实时价格查询 MUST NOT 使用会影响查询准确性的长时间缓存。实现 MAY 使用秒级请求合并或正在飞行中的请求复用来降低重复调用。

#### Scenario: 查询实时价格
- **WHEN** 用户打开价格面板或刷新 token 价格
- **THEN** 后端 MUST 查询当前 provider 数据或复用正在飞行中的同批请求
- **AND** MUST NOT 因分钟级或更长缓存返回明显过期的实时价格

### Requirement: K 线查询使用免费 OHLCV 数据源
K 线查询 MUST 使用非 OKX 免费 OHLCV 数据源。已收盘 K 线 MAY 缓存，正在形成的最后一根 K 线 MUST 保持实时或仅秒级请求合并。

#### Scenario: 查询已收盘 K 线
- **WHEN** 用户查询历史时间范围内已经收盘的 K 线
- **THEN** 后端 MAY 使用本地缓存返回已确认历史 K 线
- **AND** MUST 保持缓存 key 至少包含 `chain + pool + interval + time_range`

#### Scenario: 查询包含最新 K 线
- **WHEN** 用户查询包含当前正在形成 K 线的时间范围
- **THEN** 后端 MUST 刷新最后一根 K 线或仅复用秒级合并请求
- **AND** MUST NOT 用长缓存覆盖最后一根 K 线

### Requirement: token metadata 使用 RPC 和免费市场数据源
token 的链上静态 metadata MUST 通过 RPC 合约调用读取；展示增强 metadata MAY 使用免费市场数据源补充。

#### Scenario: 读取 token 链上静态信息
- **WHEN** 系统需要 token `decimals`、`symbol` 或 `name`
- **THEN** 后端 MUST 通过 RPC 调用 token 合约读取
- **AND** MAY 将成功结果持久化缓存

#### Scenario: 读取 token 展示增强信息
- **WHEN** 系统需要 logo 等展示字段
- **THEN** 后端 MAY 调用非 OKX 免费市场数据源
- **AND** MAY 按顺序尝试 GeckoTerminal、DexScreener、Trust Wallet 静态资产等来源补充 logo
- **AND** MUST 在 provider 失败时暴露缺失或错误状态，而不是生成误导性默认值
