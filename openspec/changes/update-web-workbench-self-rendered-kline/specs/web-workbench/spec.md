## ADDED Requirements

### Requirement: Web workbench 本地渲染池子 K 线
Web workbench SHALL 使用本地图表能力渲染选中池子的 K 线，而不是依赖第三方嵌入式 `iframe` 图表。

#### Scenario: 用户选择池子
- **WHEN** 用户在 Hot Pools 或 Smart Money 中选择一个池子
- **THEN** K 线面板根据池子推导展示代币并请求 `token_candles`
- **AND** 在 Web workbench 内部渲染蜡烛图和成交量
- **AND** 保留 GMGN 外跳入口

#### Scenario: OHLCV 不可用
- **WHEN** `token_candles` 请求失败或未返回 candle 数据
- **THEN** K 线面板展示空态或错误态
- **AND** Workbench 其他部分仍可正常使用

### Requirement: Web workbench K 线使用 OKX token candles
Web workbench SHALL 通过后端代理的 OKX Market API 获取 token 维度 K 线，而不是继续依赖 GeckoTerminal 作为主数据源。

#### Scenario: 使用 OKX 获取 K 线
- **WHEN** 用户打开 K 线面板且存在有效展示代币地址
- **THEN** Web workbench 请求 `token_candles`
- **AND** 后端使用 OKX `GET /api/v6/dex/market/candles` 获取数据

#### Scenario: 双非稳定币池子切换展示代币
- **WHEN** 选中池子同时包含两个非稳定币
- **THEN** K 线面板允许用户在 `token0 / token1` 之间切换
- **AND** 切换后重新请求对应代币的 `token_candles`

### Requirement: Web workbench 支持聪明钱 K 线覆盖层
Web workbench SHALL 为选中池子提供可选的聪明钱覆盖层，使用户可以在同一张图上查看监控钱包活动。

#### Scenario: 覆盖层数据可用
- **WHEN** 用户拥有聪明钱权限并开启覆盖层
- **THEN** Web workbench 请求池子维度的 marker 数据
- **AND** 按当前 candle 周期展示聚合后的活动标记
- **AND** 点击 marker 后展示钱包、动作、金额和交易详情

#### Scenario: 覆盖层数据不可用
- **WHEN** 用户没有聪明钱权限、ClickHouse 不可用，或当前池子没有匹配事件
- **THEN** K 线面板在没有 marker 的情况下仍可正常使用
- **AND** 覆盖层隐藏或显示为不可用，但不能破坏蜡烛图渲染

### Requirement: 提供池子维度的聪明钱 marker API
后端 MUST 为 Web workbench 图表覆盖层提供 `GET /api/smart_money_pool_markers`。

The endpoint MUST:
- 要求有效的 Telegram `initData`
- 要求通过正常访问权限校验
- 要求具备聪明钱权限
- 接收 `chain`、`pool_version`、`pool_id`、`bucket_sec`、`window_hours` 和 `limit`
- 返回包含稳定事件标识与周期对齐时间戳的 JSON marker 数据

#### Scenario: 已授权调用方请求 marker
- **WHEN** 调用方已认证、具备聪明钱权限，并提供有效池子标识
- **THEN** `GET /api/smart_money_pool_markers` 返回 HTTP `200`
- **AND** 响应中包含池子元数据和用于图表覆盖层渲染的 `events` 数组

#### Scenario: 调用方缺少聪明钱权限
- **WHEN** 调用方已认证但不具备聪明钱权限
- **THEN** `GET /api/smart_money_pool_markers` 返回 HTTP `403`

### Requirement: 提供基于 OKX 的 token candles API
后端 MUST 为 Web workbench 提供 `GET /api/token_candles`，并由该接口代理 OKX token K 线数据。

The endpoint MUST:
- 要求有效的 Telegram `initData`
- 要求通过正常访问权限校验
- 接收 `chain`、`token_address`、`bar`、`limit`、`before` 和 `after`
- 使用 OKX Market API 获取 K 线数据
- 返回归一化后的 JSON candles 数据

#### Scenario: 已授权调用方请求 token candles
- **WHEN** 调用方已认证并提供有效链与代币地址
- **THEN** `GET /api/token_candles` 返回 HTTP `200`
- **AND** 响应中包含归一化后的 `candles` 数组
