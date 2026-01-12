# Change: 开仓支持按次滑点 + 运行任务支持修改区间（下次再平衡生效）

## Why
- 当前开仓（MiniApp 一键开仓 / Bot 手动开仓）只能使用全局滑点，无法针对某次开仓临时调整，导致用户需要频繁改全局参数。
- 正在运行的任务无法在不中断任务的情况下调整区间宽度/上下限，用户希望修改后从“下一次再平衡”开始生效，并在任务卡与 MiniApp 实时仓位中看到新的策略区间配置。

## What Changes
- 开仓新增可选参数 `slippage_tolerance`：
  - MiniApp 一键开仓弹窗支持填写滑点（可选；不填则沿用全局滑点）
  - Bot 手动开仓输入支持末尾追加 `s=0.5` 形式的滑点覆写（可选；不填则沿用全局滑点）
  - 覆写值写入 `strategy_tasks.slippage_tolerance`，对本次开仓及其后续再平衡生效
- 运行任务支持修改区间（下次再平衡生效）：
  - 新增后端 API：`POST /api/task_update_range`（MiniApp）
  - Bot 任务卡增加“修改区间”按钮，输入 `5` 或 `1 3` 更新策略区间
  - MiniApp 仓位卡菜单增加“修改区间”，弹窗提交新区间
  - MiniApp 仓位卡新增展示“策略区间（下次再平衡）”，反映任务配置而非当前链上区间

## Impact
- Affected code:
  - Backend: `backend/service/web_server/open_position.go`, `backend/service/web_server/task_update_range.go`（新增）, `backend/service/realtime/realtime_positions.go`, `backend/service/strategy/strategy_exit_retry.go`, `backend/service/web_server/server.go`
  - Bot: `backend/service/bot/input_handlers.go`, `backend/service/bot/position_callbacks.go`, `backend/service/bot/task_views.go`, `backend/service/bot/task_views_ext.go`, `backend/service/bot/task_callbacks.go`, `backend/service/bot/task_input_handlers.go`, `backend/service/bot/handlers.go`, `backend/service/bot/bot.go`
  - MiniApp: `miniapp/src/App.jsx`, `miniapp/src/lib/api.js`, `miniapp/src/components/PositionCard.jsx`, `miniapp/api/task_action.js`
- Backwards compatibility:
  - `/api/open_position` 仅新增可选字段；旧客户端不受影响
  - MiniApp 新增的 `/api/task_update_range` 为新增端点；旧客户端不调用则无影响

