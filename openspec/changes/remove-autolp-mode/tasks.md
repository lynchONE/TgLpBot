## 1. 实现
- [ ] 1.1 删除后端 AutoLP 服务、PoolM 扫描、候选评估、自动开仓/换仓与相关生命周期接入
- [ ] 1.2 删除 Telegram Bot 中的 `/auto` 命令、AutoLP 回调、输入态处理与帮助文案
- [ ] 1.3 删除 Web API 中的 AutoLP 配置、监控、盈利曲线、管理员统计/关闭接口与兼容路由
- [ ] 1.4 删除策略层基于 AutoLP 池子扫描结果判断是否允许开单/重开的逻辑
- [ ] 1.5 删除 MiniApp 中的 AutoLP 页面入口、监控卡片、盈利曲线、管理员 Auto 控制与相关 API 调用
- [ ] 1.6 清理编译残留引用，按需保留仅用于历史兼容但不再参与运行的模型/字段
- [ ] 1.7 验证：`cd backend; go test ./...` 与 `cd miniapp; npm run build`
