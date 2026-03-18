## 1. 实现
- [x] 1.1 删除 Smart Money 中基于 Explorer `txlist` 的扫描与解析逻辑。
- [x] 1.2 将 Smart Money 的 LP 监听与监控合约钱包发现收敛为单一的 HTTP 轮询监控链路。
- [x] 1.3 在统一链路中实现基于 BSC HTTP RPC 的监控合约交易识别与 LP 日志处理，按区块范围推进并持久化 `last_scanned_block`。
- [x] 1.4 让统一链路的 RPC 解析遵循“RPC 池优先，`.env` 回退”的统一规则。
- [x] 1.5 调整 `last_scanned_block=0` 的初始化语义，避免前端长期显示“未扫描”。
- [x] 1.6 删除 Smart Money 对 `BSCSCAN_API_KEY` / `ETHERSCAN_API_KEY` / 旧 crawler 配置的运行时依赖，并更新 `.env.example` / 配置说明。
- [x] 1.7 更新 MiniApp / WebApp 的状态文案，使其与增量监控语义一致。
- [x] 1.8 验证：已执行 `cd backend && go test ./...`、`cd miniapp && npm run build`、`cd webapp && npm run build`。
