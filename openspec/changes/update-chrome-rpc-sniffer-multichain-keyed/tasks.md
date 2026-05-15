## 1. Implementation
- [x] 1.1 将后台状态模型增加 `chain`、`chainName`、`providerAuth` / `credentialKind` 等字段，并保留旧 storage 的兼容读取。
- [x] 1.2 扩展 page hook 的方法识别规则，覆盖 EVM 与 Solana JSON-RPC 方法。
- [x] 1.3 为 EVM 链实现按目标 chainId 的多链校验，覆盖 BSC/Base/Ethereum。
- [x] 1.4 为 Solana RPC 实现主动校验，至少校验 `getHealth` 或 `getVersion`，并读取 `getSlot`。
- [x] 1.5 实现 key/认证端点判定，只允许 URL 中带 API key/token/project id 或 headers 中带认证信息的端点进入“可用输出”。
- [x] 1.6 更新 popup 文案、明细展示、导出文件名和 README。
- [x] 1.7 做针对性 diff 检查，确认链 ID、导出筛选、Solana 方法识别、headers 转发规则没有回归。

## 2. Verification
- [x] 2.1 手工检查 OpenSpec delta 格式；如果环境可用，执行 `openspec validate update-chrome-rpc-sniffer-multichain-keyed --strict`。
- [ ] 2.2 在浏览器加载扩展后，分别用带 key 的 EVM/Base/Ethereum/Solana RPC 请求页面验证可用输出。
- [ ] 2.3 验证公共 RPC 请求只出现在抓包明细中，不进入导出 JSON。
