## 1. Implementation
- [x] 1.1 新增 OKX advanced-info API client 与响应解析。
- [x] 1.2 新增后端代币风险聚合、稳定币/主流币过滤与开仓检查。
- [x] 1.3 新增 `token_risk_snapshots` 数据库快照模型并接入 AutoMigrate。
- [x] 1.4 将池子列表/搜索结果改为读取数据库快照，并对缺失或过期代币排队后台限速刷新。
- [x] 1.5 在开仓 `prepare/preview/execute` 中接入单币即时刷新、写库和貔貅盘阻断。
- [x] 1.6 更新 WebApp 池子列表与开单弹窗风险提示。
- [x] 1.7 更新 MiniApp 池子卡片与开单面板风险提示。
- [x] 1.8 运行 gofmt、针对性测试/build 与 diff 检查。
