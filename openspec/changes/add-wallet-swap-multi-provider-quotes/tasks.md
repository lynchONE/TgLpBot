## 1. Implementation
- [ ] 1.1 新增 `wallet-swap` spec delta，定义多提供商报价、手续费净值口径、路径展示和 provider 执行行为
- [ ] 1.2 后端：为 OKX、0x、LI.FI 建立统一报价 / 执行 adapter，并补充 0x / LI.FI 配置读取
- [ ] 1.3 后端：扩展 `wallet_swap_single` 报价响应，返回多个 provider 的净到手、手续费、Gas、路径和可用状态
- [ ] 1.4 后端：扩展 `wallet_swap_single` 执行请求，支持按 provider 执行并返回最终 provider 信息
- [ ] 1.5 前端：改造 `webapp` `SwapPanel` / `api.js`，展示多 provider 报价卡片、路径、费用和选择态
- [ ] 1.6 验证：补充后端单测并执行 `cd backend && go test ./...` 与 `cd webapp && npm run build`
