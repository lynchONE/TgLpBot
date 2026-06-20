# Change: 跟单仓位增加下破保底撤出与状态说明

## Why
自动跟单仓位目前依赖目标钱包撤仓事件来平仓，普通出区间逻辑会跳过 `is_follow` 任务。若目标钱包撤仓事件漏扫或延迟，用户仓位在价格跌破区间下沿后可能持续暴露在下跌风险中。

## What Changes
- 跟单仓位保留“跟随目标钱包撤仓”的主路径，但新增下破区间保底：当价格低于跟单仓位区间下沿时，系统 MUST 立即撤出该跟单仓位并停止任务。
- 跟单仓位价格上破区间时仍不自动再平衡或撤出，继续等待目标钱包撤仓事件，避免偏离跟单语义。
- WebApp 与 MiniApp 的仓位卡片 MUST 对跟单仓位展示当前策略说明：跟随目标钱包撤仓、下破保底撤出、上破继续跟随。
- 保底撤出 MUST 只作用于 `is_follow=true` 的跟单任务，不影响普通手动仓位、AutoLP 仓位或其他用户任务。

## Impact
- Affected specs: `miniapp-smart-money`, `task-out-of-range-policy`
- Affected code: `backend/service/strategy`, `backend/service/smart_money_follow`, `backend/service/realtime`, `webapp/src/components/PositionsPanel.jsx`, `miniapp/src/components/PositionCard.jsx`
