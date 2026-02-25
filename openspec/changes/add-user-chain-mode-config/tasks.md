## 1. Implementation
- [x] 1.1 Model/DB：为 `GlobalConfig` 新增 `multi_chain_enabled` + `default_chain`，并设置安全默认值。
- [x] 1.2 Backend：实现“有效链（effective chain）”解析逻辑，基于 `GlobalConfig` + 服务器启用链列表（`CHAINS` / `EnabledChains`）。
- [x] 1.3 Web API：调整开仓接口，multi-chain 关闭时覆盖/忽略请求里的 `chain`，统一走有效链。
- [x] 1.4 Bot：在全局配置菜单新增多链开关与默认链设置；并让开仓/钱包 swap 等链相关流程在 multi-chain 关闭时不再要求选链。
- [x] 1.5 Mini App：multi-chain 关闭时锁定/隐藏链选择，并在所有链相关请求中统一使用 `default_chain`。
- [x] 1.6 验证：`cd backend; go test ./...` 和 `cd miniapp; npm run build`（如可用），以及 `openspec validate add-user-chain-mode-config --strict`。
