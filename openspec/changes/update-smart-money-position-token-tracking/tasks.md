## 1. Implementation
- [x] 1.1 SmartLP：在 `smart_lp_monitor.go` 中解析 V4 `ModifyLiquidity` 的 `salt` 并写入 `smart_lp_events.token_id`
- [x] 1.2 Backend：抽取统一的 Smart Money position-ref 查询 helper，优先从 `smart_lp_events` 直接加载 V3/V4 的 `token_id`
- [x] 1.3 Backend：改造 `GET /api/smart_money_wallet_positions`，统一按 `token_id` 查询 V3/V4 当前仓位，并新增手续费字段
- [x] 1.4 Backend：改造 `GET /api/smart_money_pool_adds`，统一按 position ref 计算 V3/V4 手续费估算；移除 V3 专属的 `collect` 模拟主路径
- [x] 1.5 Compatibility：为历史 V4 空 `token_id` 数据保留 fallback，并在响应 `warnings / fee_status` 中体现降级状态
- [x] 1.6 Tests：补充/更新 Smart Money 相关后端测试，覆盖 V4 `salt -> token_id`、统一 position ref、手续费字段与 fallback 行为
- [ ] 1.7 Verification：执行 `cd backend; go test ./...`，并手工验证 `smart_money_wallet_positions` 与 `smart_money_pool_adds` 的 V3/V4 返回结果
