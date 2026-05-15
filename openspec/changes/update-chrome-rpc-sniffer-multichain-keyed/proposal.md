# Change: 扩展 Chrome RPC 抓取器支持多链 key 节点

## Why
当前 `chrome-bsc-rpc-sniffer` 只识别并校验 BSC EVM RPC，无法从页面请求中收集 Base、Ethereum 和 Solana 的 RPC。实际运维需要的是可复用的带 API key 或认证头的节点，公共 RPC 即使可访问也不应该进入“可用输出”。

## What Changes
- 将插件从 BSC 单链抓取扩展为多链 RPC 抓取，至少支持 `bsc`、`base`、`eth`、`solana`。
- EVM 链按 `eth_chainId` 校验：BSC=`56`、Base=`8453`、Ethereum=`1`；Solana 按 Solana JSON-RPC 方法校验。
- 页面 hook 同时识别 EVM JSON-RPC 与 Solana JSON-RPC 请求，并保留 URL、transport、headers、source page 和 observed methods。
- “可用输出”只包含带 key 或认证信息的端点；公共 RPC 端点可以在明细中展示，但不得进入导出结果。
- 弹窗、导出文件名和 README 从 BSC 单链表述改为多链 key RPC 抓取器表述。

## Impact
- Affected specs: `chrome-rpc-sniffer`
- Affected code:
  - `chrome-bsc-rpc-sniffer/background.js`
  - `chrome-bsc-rpc-sniffer/page-hook.js`
  - `chrome-bsc-rpc-sniffer/content-script.js`
  - `chrome-bsc-rpc-sniffer/popup.html`
  - `chrome-bsc-rpc-sniffer/popup.js`
  - `chrome-bsc-rpc-sniffer/manifest.json`
  - `chrome-bsc-rpc-sniffer/README.md`

