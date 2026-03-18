## 1. 实现
- [x] 1.1 为 `pools` 新增 MySQL 模型与迁移，字段参考原 ClickHouse `pools` 表
- [x] 1.2 复用现有外部抓取逻辑，将抓取到的池子数据保存到 MySQL `pools` 新表
- [x] 1.3 将 `/api/pools` 及原先依赖 ClickHouse `pools` 表的保留业务改为读取 MySQL
- [x] 1.4 删除 ClickHouse 初始化、配置、环境变量、依赖和 `base/clickhouse` 代码
- [x] 1.5 删除 Hot Pools 之外的 ClickHouse 相关后端服务、路由、Bot 入口与测试
- [x] 1.6 删除前端中 ClickHouse 相关 API 代理、页面、组件和入口，保留 `pools` 相关能力及非 ClickHouse 既有能力
- [x] 1.7 清理残留引用并完成验证：`cd backend && go test ./...`、`cd miniapp && npm run build`、`cd webapp && npm run build`
