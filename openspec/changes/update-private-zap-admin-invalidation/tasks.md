## 1. Implementation
- [x] 1.1 新增 `zap-contracts` spec delta，定义管理员按链失效与新的解析链路。
- [x] 1.2 后端：移除 Private Zap 运行时版本失效判断，改为 Redis -> DB -> 部署 的解析逻辑。
- [x] 1.3 后端：新增管理员按链失效 Private Zap 的 API，并接入 `/api/admin` 兼容路由。
- [x] 1.4 Mini App：在管理员页增加按链失效按钮，并调用新的 admin API。
- [x] 1.5 测试：更新 Private Zap 相关单测，补充 admin handler 测试。
- [x] 1.6 验证：运行 `cd backend; go test ./service/liquidity ./service/web_server`，以及 `cd miniapp; npm run build`。
