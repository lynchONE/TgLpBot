# 多链 Key RPC 抓取器

这是一个独立 Chrome 插件，用来从目标网页里抓取真实使用的 JSON-RPC 端点。

它会监听页面里的：
- `fetch`
- `XMLHttpRequest`
- `WebSocket.send`

当前支持：
- BSC
- Base
- Ethereum
- Solana

## 关键规则

顶部“可导出输出”只包含同时满足以下条件的端点：

1. 主动链上校验成功。
2. URL 或 headers 中能识别到 API key、token、project id 或认证信息。

公共 RPC 即使链上可用，也只会出现在“抓包明细”里，不会进入导出 JSON。

## 校验方式

EVM 链：
- 请求 `eth_chainId`
- BSC 必须返回 `56` / `0x38`
- Base 必须返回 `8453` / `0x2105`
- Ethereum 必须返回 `1` / `0x1`
- 请求 `eth_blockNumber`
- 尝试请求 `web3_clientVersion`

Solana：
- 请求 `getSlot`
- 尝试请求 `getVersion`
- 如版本请求失败，会尝试 `getHealth`

## 安装方式

1. 打开 `chrome://extensions/`
2. 开启右上角“开发者模式”
3. 点击“加载已解压的扩展程序”
4. 选择目录 `E:\goProject\TgLpBot\chrome-bsc-rpc-sniffer`

## 使用方式

1. 打开你要抓包的链上项目网页。
2. 正常操作页面，让它产生链上请求。
3. 点击浏览器工具栏里的“多链 Key RPC 抓取器”。
4. 查看“可导出输出”和“抓包明细”。

如果页面请求发生得很早，建议打开弹窗后点击“重载当前页”，让插件从页面初始化阶段开始监听。

## 导出字段

每个可导出项至少包含：

- `chain`
- `chainName`
- `rpcFamily`
- `url`
- `transport`
- `headers`
- `credentialKind`
- `latencyMs`
- `clientVersion`
- `lastCheckedAt`
- `sourcePages`
- `observedMethods`

EVM 端点还包含：
- `chainId`
- `blockNumber`

Solana 端点还包含：
- `slot`

## 凭据识别

插件会识别常见凭据位置：

- URL query：`apiKey`、`apikey`、`key`、`token`、`projectId`、`auth` 等
- URL path：常见 provider 的路径 token，例如 Alchemy、Infura、QuickNode、Helius 等
- Headers：`authorization`、`x-api-key`、`api-key`、`x-project-id` 等

## 已知限制

- 插件只能抓到页面 JS 显式设置的 URL 和 headers。浏览器底层、代理层或 provider SDK 内部不可见的认证信息无法直接读取。
- WebSocket 不能像 HTTP 一样追加自定义 headers；如果站点依赖不可见 WS 认证，插件只能按可见 URL 做最佳努力校验。
- 导出文件可能包含真实 API key，不要提交到 git，也不要发给不可信的人。
