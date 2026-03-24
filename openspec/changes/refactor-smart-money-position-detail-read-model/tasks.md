## 1. Proposal
- [ ] 1.1 确认旧的 ClickHouse 相关聪明钱提案不再作为实现基线，本提案作为新的唯一落地方案

## 2. Backend 数据模型
- [ ] 2.1 新增 MySQL `sm_lp_active_positions` 模型、索引与迁移逻辑
- [ ] 2.2 为 active position 定义稳定的 `position_ref` 编码 / 解码规则
- [ ] 2.3 为手续费快照增加 `fee_status`、`fee_updated_at` 等字段

## 3. 入库与补算
- [ ] 3.1 修改 Smart Money watcher，在 add/remove 事件入库事务中同步 upsert `sm_lp_active_positions`
- [ ] 3.2 基于现有 `sm_lp_events` 增加 active position 回放初始化 / 补算逻辑
- [ ] 3.3 为高成本动态字段增加短 TTL 缓存、singleflight、限流与降级快照

## 4. 查询接口
- [ ] 4.1 列表接口返回 `position_ref`，用于前端点开详情
- [ ] 4.2 新增统一仓位详情接口，按 `position_ref` 读取 active position 并返回卡片化字段
- [ ] 4.3 详情查询优先使用 `sm_lp_active_positions` 提供的元数据，只对实时字段做最小化链上读取
- [ ] 4.4 为缺失 / 过期的链上实时字段返回明确 `fee_status` / `warnings`

## 5. WebApp / MiniApp
- [ ] 5.1 WebApp 聪明钱仓位行支持点开详情，并按 WebApp 仓位卡片样式展示
- [ ] 5.2 MiniApp 聪明钱仓位行支持点开详情，并按实时 `PositionCard` 样式展示
- [ ] 5.3 两端详情页按后端返回的 `poll_interval_sec` 自动刷新

## 6. Validation
- [ ] 6.1 `cd backend && go test ./...`
- [ ] 6.2 `cd webapp && npm run build`
- [ ] 6.3 `cd miniapp && npm run build`
- [ ] 6.4 `openspec validate refactor-smart-money-position-detail-read-model --strict`
