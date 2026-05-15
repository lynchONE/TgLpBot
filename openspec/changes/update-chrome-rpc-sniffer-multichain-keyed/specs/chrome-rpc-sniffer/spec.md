## ADDED Requirements

### Requirement: 多链 RPC 识别与校验
Chrome RPC 抓取器 MUST 从页面的 JSON-RPC 请求中识别并主动校验 BSC、Base、Ethereum 和 Solana RPC 端点。

EVM RPC 端点 MUST 通过 `eth_chainId` 判定链归属，并至少支持：
- `bsc`: chainId `56` / `0x38`
- `base`: chainId `8453` / `0x2105`
- `eth`: chainId `1` / `0x1`

Solana RPC 端点 MUST 通过 Solana JSON-RPC 方法和返回结构判定，并至少主动读取 slot 或版本信息。

#### Scenario: 抓到 Base RPC
- **WHEN** 页面向某个 HTTP RPC 发送 `eth_blockNumber` 或其他 EVM JSON-RPC 请求
- **AND** 抓取器主动调用该端点的 `eth_chainId` 返回 `0x2105`
- **THEN** 抓取器 MUST 将该端点标记为 `base`
- **AND** 校验通过时记录 block number、latency 和 client/version 信息

#### Scenario: 抓到 Solana RPC
- **WHEN** 页面向某个 HTTP RPC 发送 `getLatestBlockhash`、`getSlot` 或其他 Solana JSON-RPC 请求
- **AND** 抓取器主动调用该端点的 Solana 校验方法返回有效结果
- **THEN** 抓取器 MUST 将该端点标记为 `solana`
- **AND** 校验通过时记录 slot、latency 和 version 信息

### Requirement: 只导出带 key 或认证信息的可用 RPC
Chrome RPC 抓取器 MUST 区分“链上校验可用”和“可导出的带 key 可用”。只有同时满足链上校验成功且包含显式 API key、project id、token 或认证 header 的端点，才允许进入顶部“可用输出”和 JSON 导出。

公共 RPC 或无法识别凭据的端点 MAY 出现在抓包明细中，但 MUST NOT 出现在可用导出结果中。

#### Scenario: 公共 RPC 不进入导出
- **WHEN** 页面请求 `https://bsc-dataseed.binance.org/` 且链上校验成功
- **AND** URL 与 headers 中没有可识别的 key、token、project id 或认证信息
- **THEN** 抓取器 MUST 在明细中展示该端点
- **AND** MUST NOT 将该端点放入“可用输出”或导出 JSON

#### Scenario: 带认证头的 RPC 进入导出
- **WHEN** 页面请求某个 RPC 时显式设置 `authorization` 或 `x-api-key` header
- **AND** 主动链上校验成功
- **THEN** 抓取器 MUST 将该端点放入“可用输出”
- **AND** 导出项 MUST 保留复用所需的 headers

### Requirement: 多链导出结构
Chrome RPC 抓取器 MUST 在导出 JSON 的每个端点中包含链归属、transport、URL、headers、校验数据和来源信息，便于后续人工导入运维配置。

每个导出项 MUST 至少包含：
- `chain`
- `url`
- `transport`
- `headers`
- `credentialKind`
- `latencyMs`
- `lastCheckedAt`
- `sourcePages`
- `observedMethods`

EVM 端点 MUST 包含 `chainId` 和 `blockNumber`；Solana 端点 MUST 包含 `slot` 或等效最新高度字段。

#### Scenario: 导出 Ethereum 带 key RPC
- **GIVEN** 抓取器已确认一个 Ethereum HTTP RPC 链上可用且 URL 中包含 provider key
- **WHEN** 用户复制或导出可用结果
- **THEN** JSON 项 MUST 包含 `chain: "eth"`、`chainId: 1`、URL、headers、credential kind、block number 和来源页面

