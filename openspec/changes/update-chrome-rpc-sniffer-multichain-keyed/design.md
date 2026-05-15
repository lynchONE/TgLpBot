## Context
现有插件在页面主世界 hook `fetch`、`XMLHttpRequest` 和 `WebSocket.send`，把识别到的 JSON-RPC 请求传给 background，再由 background 主动请求 `eth_chainId`、`eth_blockNumber`、`web3_clientVersion` 判定是否为 BSC 可用节点。

这次需求不是简单增加几个公共 RPC URL，而是从真实网页请求中抓取带 key 或认证头的 RPC 配置，并支持 EVM 与 Solana 两类 JSON-RPC。

## Goals / Non-Goals
- Goals:
  - 支持 BSC、Base、Ethereum、Solana 四条链的 RPC 识别和主动校验。
  - 导出的“可用输出”只保留具备 API key、project id、token 或认证 header 的端点。
  - 明细中保留被抓到但不满足 key 条件的公共端点，并明确不可导出的原因。
- Non-Goals:
  - 不内置任何第三方 RPC key。
  - 不绕过浏览器或站点安全策略读取非 JS 显式设置的认证信息。
  - 不把抓到的 key 自动写入后端 RPC 池；本次只做 Chrome 插件抓取与导出。

## Decisions
- Decision: 使用链配置表描述 `chain`、`displayName`、`rpcFamily`、`chainId` 和校验方法。
  - Why: 避免继续散落 `TARGET_CHAIN_ID` 这类单链硬编码，后续新增链只需要补配置和校验器。
- Decision: EVM 端点用同一个候选 URL 对目标链逐一校验，匹配到目标 chainId 后记录对应链。
  - Why: 页面请求本身未必显式携带链名，`eth_chainId` 是最可靠的归属来源。
- Decision: Solana 端点只接受 Solana JSON-RPC 方法触发的候选，再用 `getSlot` 与 `getVersion` / `getHealth` 校验。
  - Why: Solana 没有 EVM `chainId`，必须按协议方法和返回结构确认。
- Decision: key/认证判定基于 URL 与 headers 的显式凭据特征。
  - URL 特征包括 path 或 query 中出现非空 project id / api key / token，例如常见 provider 的路径 token、`apiKey`、`apikey`、`key`、`token`、`projectId`、`auth`。
  - Header 特征包括 `authorization`、`x-api-key`、`api-key`、`x-project-id` 等。
  - Why: 公共 RPC 通常只有裸域名或公开路径；带 key 节点才更可能可复用并满足运维需求。

## Risks / Trade-offs
- 风险：部分 provider 使用路径 token，但字段名不包含 key；需要维护 provider token pattern。
  - Mitigation: 先覆盖常见路径 token 形式，并在明细中展示“未识别为带 key”的端点，便于后续补规则。
- 风险：headers 中的凭据可能包含敏感信息。
  - Mitigation: 插件仅本地存储和导出；UI 明细可以显示完整 headers 以便复用，但 README 必须提示不要提交导出文件。
- 风险：WebSocket 自定义 headers 不能由浏览器 JS 设置，主动校验时无法复用不可见认证。
  - Mitigation: 只承诺抓取 JS 显式可见 URL / headers；不可见认证按不可复用处理。

