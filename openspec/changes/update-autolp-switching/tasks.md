## 1. Implementation
- [x] 1.1 扩展 `AutoLPUserConfig`：新增 `switch_cooldown_seconds` 字段，并补充字段默认值与校验规则
- [x] 1.2 Telegram `/auto` 菜单：展示并支持设置 `switch_min_improvement_pct` 与 `switch_cooldown_seconds`
- [x] 1.3 AutoLP 执行逻辑：当 AutoLP 满仓时，按 spec 评估并尝试调度一次换仓（复用现有 `switch` 流程）
- [x] 1.4 冷却与完成判定：基于“换仓完成时间”判定冷却（例如读取最新的 `AutoLPEventSwitch` 事件时间戳）
- [x] 1.5 并发与幂等：确保同一用户同一时刻最多调度一次换仓（用户级互斥 + DB 条件更新/事务），并在冷却窗口内拒绝重复换仓
- [x] 1.6 单元测试：覆盖阈值判定、冷却判定、候选/被替换任务选择的关键逻辑
- [x] 1.7 验证：运行 `cd backend; go test ./...`
