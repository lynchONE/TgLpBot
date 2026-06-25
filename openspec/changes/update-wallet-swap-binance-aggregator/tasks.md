## 1. Implementation
- [x] 1.1 确认 Binance `Get Aggregated Quote` 与 `Build Swap Transaction` 的官方路径、认证头、请求字段、响应字段和错误格式。
- [x] 1.2 后端：新增 Binance Trading API 配置、exchange adapter、响应结构和可注入 HTTP client 测试。
- [x] 1.3 后端：移除钱包单币兑换报价中的 0x/LI.FI 调用，改为返回 Binance 聚合多 route 报价。
- [x] 1.4 后端：执行接口支持提交 Binance route 选择，执行前重新调用构造交易接口，并拒绝 0x/LI.FI provider。
- [x] 1.5 前端：更新 WebApp 兑换面板，展示 Binance 多 route、支持选择 route 并提交执行。
- [x] 1.6 前端：更新 MiniApp 兑换模块和详情抽屉，展示 Binance 多 route、支持选择 route 并提交执行。
- [x] 1.7 验证：运行针对性后端测试、`cd backend && go test ./...`、`cd webapp && npm run build`、`cd miniapp && npm run build`。
- [x] 1.8 检查：修改后执行针对性 diff 检查，确认没有遗漏调用更新、API 字段不匹配或 0x/LI.FI fallback。
