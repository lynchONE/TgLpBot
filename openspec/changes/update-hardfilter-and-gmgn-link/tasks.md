## 1. Implementation
- [x] 1.1 扩展 `SystemConfig`/`HardFilterConfig`：新增 `autolp_filter_chinese_tokens` 与 `autolp_max_fee_percentage`
- [x] 1.2 后端 `/api/admin/system_config`：支持读取/更新新增字段，并在 defaults 中返回环境变量默认值
- [x] 1.3 AutoLP 执行逻辑：在分析/开仓/换仓目标选择中应用新增硬筛（中文 + 费率上限）
- [x] 1.4 MiniApp 管理员面板：在 `SystemConfigCard` 增加对应配置项（开关 + 输入框）
- [x] 1.5 MiniApp 仓位卡片：代币点击跳转 GMGN
- [x] 1.6 验证：`cd backend; go test ./...` 与 `cd miniapp; npm run build`
