# Change: 统一任务越界处理为再平衡或撤仓终止

## Why
- 当前 CLMM 任务的越界处理同时受 `rebalance_enabled`、`stop_loss_enabled` 和上下方向影响，真实行为已经偏离用户心智：上涨越界走再平衡、下跌越界优先走止损、关闭再平衡后又可能只是提醒不处理。
- 用户希望把越界策略收敛成更直观的两种模式：开启再平衡时，任意方向越界都在缓冲时间后自动再平衡；关闭再平衡时，任意方向越界都在缓冲时间后自动撤仓并终止任务。只有任务被用户主动暂停时，系统才不自动撤出。
- 单边池当前会被天然判定为“已在区间外”，如果直接套用上述规则，会导致单边池刚开仓就立刻再平衡或撤仓；需要补充“首次进入区间后才开始按正常越界规则处理”的状态机。

## What Changes
- 调整 CLMM 任务越界执行语义：
  - `rebalance_enabled=true` 时，价格向上或向下越界都统一走“缓冲后再平衡”。
  - `rebalance_enabled=false` 时，价格向上或向下越界都统一走“缓冲后撤仓并终止任务”。
  - `paused=true` 的任务继续不参与自动越界处理。
- 单边池新增“区间激活”概念：
  - 若任务创建或重开时价格尚未进入配置区间，则在首次进入区间前不启动越界倒计时，也不自动再平衡/撤仓。
  - 一旦价格首次进入区间，该任务后续再按普通双边池的越界规则处理。
- WebApp 与 MiniApp 的开仓页统一改为单一越界执行心智：
  - 不再把“止损”作为越界执行模式入口。
  - 明确展示“超出区间后会自动再平衡”或“超出区间后会自动撤仓终止”的文案。
  - 单边池文案增加“首次进入区间前不会触发自动处理”的说明。
- 兼容旧客户端与旧任务：
  - 后端继续兼容 `stop_loss_enabled` 请求字段与库表字段，但 CLMM 越界策略不再依赖该字段做分支判断。

## Impact
- Affected specs:
  - `task-out-of-range-policy`
- Affected code:
  - Backend: `backend/base/models/strategy.go`, `backend/service/strategy/strategy_service.go`, `backend/service/strategy/strategy_exit_retry.go`, `backend/service/web_server/open_position.go`
  - Frontend WebApp: `webapp/src/components/OpenPositionModal.jsx`
  - Frontend MiniApp: `miniapp/src/App.jsx`
  - Tests / docs: `backend/service/strategy/strategy_service_test.go`, 相关 README / 文案说明
