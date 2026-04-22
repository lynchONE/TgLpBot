# Change: CLMM任务模式升级与开仓界面简化

## Why
- 当前任务只有 `rebalance_enabled` 和 `paused` 两个核心开关，实际只能表达“双向再平衡”“双向撤出终止”“暂停”三种状态，无法支持“上破再平衡、下破撤出终止”这种非对称模式。
- `paused` 虽然已经存在，但在开仓页和仓位卡里没有被统一成“任务模式”心智；用户无法在开仓时直接以暂停态创建任务，也无法在仓位卡上用一组明确按钮切换模式。
- 开仓页当前同时展示保守/中性/激进仓位建议、完整的前置兑换说明块、额外的“确认本次前置兑换”复选框，信息密度偏高，影响主决策路径。

## What Changes
- 为 CLMM 任务新增可持久化的越界模式字段，支持以下自动处理模式：
  - 双向越界后自动再平衡
  - 双向越界后自动撤出并结束任务
  - 上破区间自动再平衡、下破区间自动撤出并结束任务
- 保留现有 `paused` 作为独立暂停态，但在前端统一映射为“暂停任务”模式按钮；开仓时允许直接选择暂停态创建任务。
- 保持旧字段兼容：
  - 开仓接口若仍只传 `rebalance_enabled`，继续映射为“双向再平衡 / 双向撤出终止”
  - 旧的 `task_toggle_rebalance` 接口继续保留，用于兼容旧客户端
- 新增任务模式更新入口，供 WebApp / MiniApp 仓位卡直接切换模式。
- WebApp / MiniApp 开仓页同步调整：
  - 默认模式改为“再平衡关闭 / 双向撤出终止”
  - 去掉保守 / 中性 / 激进三档建议展示
  - 压缩前置兑换展示，只保留关键信息与滑点输入，不再显示额外的“我已确认本次前置兑换”复选框
  - 当本次开仓需要前置兑换时，用户点击最终“确认开仓”即视为确认当前预览
- WebApp / MiniApp 仓位卡同步调整：
  - 以任务模式按钮替换单一“再平衡开关”
  - 在卡片上直观展示当前模式，并允许一键切换或暂停/恢复

## Impact
- Affected specs:
  - `task-out-of-range-policy`
  - `open-position-safety`
- Affected code:
  - Backend:
    - `backend/base/models/strategy.go`
    - `backend/service/strategy/strategy_service.go`
    - `backend/service/strategy/strategy_service_test.go`
    - `backend/service/realtime/realtime_positions.go`
    - `backend/service/web_server/open_position.go`
    - `backend/service/web_server/task_toggle_rebalance.go`
    - `backend/service/web_server/task_pause.go`
    - `backend/service/web_server/compat_routes.go`
    - `backend/service/web_server/server.go`
  - MiniApp:
    - `miniapp/src/App.jsx`
    - `miniapp/src/components/PositionCard.jsx`
    - `miniapp/src/lib/api.js`
    - `miniapp/api/task_action.js`
  - WebApp:
    - `webapp/src/components/OpenPositionModal.jsx`
    - `webapp/src/components/TaskActionMenu.jsx`
    - `webapp/src/App.jsx`
    - `webapp/src/api.js`
