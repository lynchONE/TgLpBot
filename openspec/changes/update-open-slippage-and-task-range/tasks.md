## 1. Implementation
- [x] 1.1 Backend：`/api/open_position` 支持可选 `slippage_tolerance`（不传则用全局滑点），写入任务并供后续再平衡复用
- [x] 1.2 Bot：手动开仓输入支持末尾 `s=0.5` 覆写滑点，并落到 `strategy_tasks.slippage_tolerance`
- [x] 1.3 MiniApp：一键开仓弹窗增加滑点输入（可选），并透传到后端
- [x] 1.4 Backend：新增 `POST /api/task_update_range` 更新任务区间配置（下次再平衡生效）
- [x] 1.5 MiniApp：任务菜单增加“修改区间”弹窗并调用 `task_update_range`
- [x] 1.6 Bot：任务卡增加“修改区间”按钮与输入流程（`5` / `1 3`）
- [x] 1.7 Backend：`/api/realtime_positions` 返回任务策略区间（stable 百分比），MiniApp 仓位卡展示“策略区间（下次再平衡）”
- [x] 1.8 Backend：再平衡入场前刷新可编辑配置（range/slippage），保证区间修改能作用于下一次再平衡
- [x] 1.9 验证：`cd backend; go test ./...` 与 `cd miniapp; npm run build` 与 `openspec validate update-open-slippage-and-task-range --strict`
