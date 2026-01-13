## 1. Implementation
- [x] 1.1 Backend：扩展 `SystemConfig` 增加首次开仓固定区间配置字段
- [x] 1.2 Backend：`/api/admin/system_config` 支持 GET/POST 读取/更新新字段
- [x] 1.3 Backend：AutoLP 创建任务时仅首次开仓使用固定总宽度计算 `tick_lower/tick_upper`
- [x] 1.4 MiniApp：管理员系统配置卡片增加开关与宽度输入
- [x] 1.5 验证：`cd backend; go test ./...` 与 `cd miniapp; npm run build` 与 `openspec validate add-autolp-first-open-fixed-width --strict`

