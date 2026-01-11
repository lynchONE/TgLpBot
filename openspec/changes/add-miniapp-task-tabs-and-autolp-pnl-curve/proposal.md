# Change: MiniApp 实时仓位任务标签（全部/手动/Auto）+ AutoLP 本次开启盈利曲线

## Why
- MiniApp「实时仓位」当前把手动任务与 Auto 任务混在一起展示，用户想快速按类型筛选正在运行的任务。
- AutoLP 开启后缺少“本次开启”的直观收益展示，希望能看到盈利曲线，并能关联每次开/平仓后的盈亏变化。

## What Changes
- MiniApp「实时仓位」页面增加 3 个标签：`全部`（默认）、`手动任务`、`Auto任务`，支持按标签仅查看对应类型的正在运行任务。
- 后端 `/api/realtime_positions` 的 `position` 对象新增字段 `task_is_auto`（当 `task_id>0`），用于前端准确区分手动/Auto 任务。
- 新增后端 API：`POST /api/autolp_pnl_curve`（需 Telegram WebApp `initData` 认证），返回本次 AutoLP 开启窗口内的收益曲线数据：
  - 数据源基于 `trade_records`（`profit_usdt`，已扣除 Gas）+ `strategy_tasks.is_auto=1`
  - 窗口基于 `auto_lp_user_configs.last_enabled_at/last_disabled_at/enabled`
  - 返回按时间排序的事件/点，用于前端绘制累计盈利曲线，并对开/平仓事件做标记
- MiniApp 在 `Auto任务` 标签页中，除仓位卡片外，增加“盈利曲线”展示区（折线图 + 累计收益摘要）。

## Impact
- Affected specs (new):
  - `specs/miniapp-realtime-task-tabs/spec.md`
  - `specs/miniapp-autolp-pnl-curve/spec.md`
- Affected code (implementation stage):
  - Backend: `backend/service/realtime/realtime_positions.go`, `backend/service/web_server/server.go`, `backend/service/web_server/autolp_pnl_curve.go`（新增）
  - MiniApp: `miniapp/src/App.jsx`, `miniapp/src/lib/api.js`, `miniapp/api/settings.js` 或新增 `miniapp/api/autolp_pnl_curve.js`, `miniapp/src/components/AutoPnLCurveCard.jsx`（新增）
- Data model: 复用现有表 `trade_records` 与 `auto_lp_user_configs`（无需新增表）
- Backwards compatibility: `/api/realtime_positions` 仅新增字段；新增 API 不影响旧客户端

## Open Questions (need your confirmation)
1. ✅ 盈利口径：曲线展示“已实现累计盈利（profit_usdt，已扣除 Gas）”，并叠加展示“未平仓浮动盈亏”（当前估算）与总计。
2. ✅ 窗口口径：当 AutoLP 已关闭时，`Auto任务` 标签展示“上次开启窗口”的曲线。
3. ✅ 事件展示：开仓点展示开仓金额与交易对；平仓点展示收益与交易对。
4. ✅ 数据量：限制最近 400 笔交易记录（trade_records），超出则截断并在响应中标记 truncated。
