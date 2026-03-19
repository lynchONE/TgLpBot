# BSC RPC 抓取器

这是一个独立的 Chrome 插件，和当前仓库业务代码无关。它的目标很单一：

- 监听页面里的 `fetch` / `XMLHttpRequest` / `WebSocket.send`
- 识别 EVM JSON-RPC 请求
- 提取可能的 RPC 地址和显式请求头
- 主动调用 `eth_chainId`、`eth_blockNumber`、`web3_clientVersion`
- 只有确认是 `BSC Mainnet (chainId=56)` 且能正常返回的端点，才标记为“可用”

## 目录结构

```text
chrome-bsc-rpc-sniffer/
├─ manifest.json
├─ background.js
├─ content-script.js
├─ page-hook.js
├─ popup.html
├─ popup.css
├─ popup.js
└─ README.md
```

## 安装方式

1. 打开 `chrome://extensions/`
2. 开启右上角“开发者模式”
3. 点击“加载已解压的扩展程序”
4. 选择目录 `E:\goProject\TgLpBot\chrome-bsc-rpc-sniffer`

## 使用方式

1. 打开你要抓包的 BSC 项目网页
2. 正常操作页面，让它产生链上请求
3. 点击浏览器工具栏里的“BSC RPC 抓取器”
4. 查看“确认可用输出”和“抓包明细”

如果页面请求发生得很早，建议在打开弹窗后点击一次“重载当前页”，让插件从页面初始化阶段开始监听。

## 可用性判定规则

插件不会只看网页有没有发请求，而是会主动二次校验：

1. 请求 `eth_chainId`
2. 必须返回 `0x38` 或十进制 `56`
3. 请求 `eth_blockNumber`
4. 必须返回有效区块高度
5. 额外尝试请求 `web3_clientVersion`

只有满足前四条的端点才会出现在顶部“确认可用输出”里。

## 关于请求头

有些 RPC 提供商不把密钥放在 URL，而是通过请求头传递，比如：

- `authorization`
- `x-api-key`
- 其他自定义头

插件会尽量捕获网页显式设置的请求头，并在导出的 JSON 中一并保留。这样导出的结果更接近“真正可复用”的 RPC 配置。

## 输出说明

顶部文本框输出的是 JSON 数组，每一项包含：

- `url`
- `transport`
- `headers`
- `chainId`
- `blockNumber`
- `latencyMs`
- `clientVersion`
- `lastCheckedAt`
- `sourcePages`
- `observedMethods`

## 已知限制

- 如果目标站点通过浏览器底层网络层注入认证信息，而不是通过 JS 显式设置请求头，插件无法直接拿到这部分信息。
- 某些页面若在插件注入前就完成了 RPC 请求，需要手动重载页面再抓一次。
- `WebSocket` 校验属于浏览器环境内的最佳努力实现，个别站点可能只暴露 HTTP RPC。
