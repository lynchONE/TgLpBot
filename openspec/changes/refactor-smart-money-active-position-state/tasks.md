## 1. Implementation
- [ ] 1.1 为 ClickHouse 新增 `smart_lp_active_positions` 表与迁移逻辑
- [ ] 1.2 新增基于 `smart_lp_events` 回放最近窗口生成 active state 的初始化逻辑
- [ ] 1.3 在 SmartLP monitor 的 add/remove 入库路径中同步 upsert `smart_lp_active_positions`
- [ ] 1.4 将 `GET /api/smart_money_pool_adds` 切到 `smart_lp_active_positions` 读路径
- [ ] 1.5 将 `GET /api/smart_money_wallet_positions` 切到 `smart_lp_active_positions` 读路径
- [ ] 1.6 新增低频 reconciliation / 修复任务，避免漏采导致状态长期漂移
- [ ] 1.7 为新增表回放、状态更新、接口读路径补单测/集成测试
- [ ] 1.8 验证：`cd backend; go test ./...`、`cd webapp; npm run build`
- [ ] 1.9 验证 OpenSpec 变更文件完整性
