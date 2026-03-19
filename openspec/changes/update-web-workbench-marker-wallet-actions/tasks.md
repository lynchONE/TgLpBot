## 1. Implementation
- [x] 1.1 后端：调整 Smart Money 钱包保存逻辑，支持按地址保存/更新 `label`，并确保仅保存标签时不会自动开启活跃监控。
- [x] 1.2 前端：在 K 线 marker tooltip 中增加钱包标签编辑态、保存动作、错误提示和 loading 状态。
- [x] 1.3 前端：将特别关注蓝色开仓线派生逻辑改为“每个钱包只取当前池子窗口内最近一次 `add` 事件”。
- [x] 1.4 前端：确保 tooltip 的打开/关闭、复制地址、特别关注切换与标签编辑互不冲突。

## 2. Verification
- [x] 2.1 `cd webapp && npm run build`
- [x] 2.2 `cd backend && go test ./service/web_server/... ./service/smart_money/...`
- [ ] 2.3 手工验证：在 K 线 tooltip 中修改标签后，tooltip 与后续 marker 标签立即更新。
- [ ] 2.4 手工验证：对同一个特别关注钱包存在多次开仓时，图上仅保留最近一次蓝色开仓线。
