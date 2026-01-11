## 1. Implementation
- [x] 1.1 Backend：扩展 `/api/realtime_positions` 返回的 `position` 结构，补充 `task_is_auto`（覆盖 V3/V4/占位 pending task 三类）
- [x] 1.2 Backend：新增 `POST /api/autolp_pnl_curve`，按窗口查询 `trade_records` 并返回累计已实现盈利曲线 + 当前未平仓浮动盈亏（含开/平仓事件标记，限制最近 400 笔）
- [x] 1.3 MiniApp API：通过 Vercel proxy 增加对应转发（更新 `miniapp/api/settings.js` 或新增 `miniapp/api/autolp_pnl_curve.js`），并在 `miniapp/src/lib/api.js` 增加 `fetchAutoLPPnLCurve`
- [x] 1.4 MiniApp UI：在「实时仓位」页增加标签（全部/手动/Auto）并按 `task_is_auto` 过滤任务卡片
- [x] 1.5 MiniApp UI：新增 `AutoPnLCurveCard` 组件，在 `Auto任务` 标签展示“本次开启收益曲线”
- [x] 1.6 验证：`cd backend; go test ./...` 与 `cd miniapp; npm run build`
