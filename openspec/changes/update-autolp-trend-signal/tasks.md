## 1. Implementation
- [x] 1.1 配置项：新增 `AUTO_LP_TREND_FILTER_ENABLED` / `AUTO_LP_ENTRY_TREND_CROSS_PCT` / `AUTO_LP_ENTRY_BLOCK_DEV5_PCT`
- [x] 1.2 计算信号：在分析阶段产出 `ma_cross_pct`、`dev5_pct` 并用于 `Trend60` 判定
- [x] 1.3 候选门禁：`CANDIDATE` 需通过“非 DOWNTREND + 非短期显著下跌”过滤（可回退旧逻辑）
- [x] 1.4 文案/可观测性：Top 推送与日志补充展示新信号与阻止原因；同步 `/auto` 策略说明（如需要）
- [x] 1.5 单测：覆盖趋势判定与门禁（UP/DOWN/SIDEWAYS/UNKNOWN）边界样本
- [x] 1.6 验证：`cd backend; go test ./...` + `openspec validate update-autolp-trend-signal --strict`
- [x] 1.7 管理员动态配置：MiniApp 系统配置支持进场门禁 3 项参数的读取/更新，并在 AutoLP 扫描中生效
- [x] 1.8 验证：`cd miniapp; npm run build`
