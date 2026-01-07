## Context
当前 AutoLP 在 `backend/service/auto_lp/auto_lp_service.go` 中会为每个开启的用户寻找候选池并自动开仓；当用户的 AutoLP 仓位数达到 `max_active_tasks` 后，逻辑直接返回，不会进行“换仓”。

代码中已经存在一套“换仓”状态机基础设施：
- `models.StrategyTask` 内已有 `SwitchTarget*` 字段与 `exit_pending_action=switch`
- `strategy/strategy_exit_retry.go` 支持在 `switch` 场景下“撤出 → 兑换 USDT → 重开仓”
- `models.AutoLPUserConfig` 已有 `switch_min_improvement_pct` 字段
- `AutoLPService.trySwitchWorstAutoTask` 存在但目前被硬编码禁用（`return false, nil`）

本变更的目标是在不引入新链上交互逻辑、不改变现有撤仓/开仓实现的前提下，**安全地启用**满仓换仓，并补齐用户级冷却与并发保护。

## Goals / Non-Goals
### Goals
- AutoLP 满仓时可“用更高收益池替换最低收益池”，提升资金利用率
- 支持用户级配置：
  - 收益提升阈值（`switch_min_improvement_pct`）
  - 冷却时间（`switch_cooldown_seconds`，默认 300 秒）
- 强并发安全：同一用户不会被重复调度换仓（避免重复撤仓/开仓）
- 不影响现有手动任务（`is_auto=false`）与现有 AutoLP 开仓/撤仓逻辑

### Non-Goals
- 不改变 PoolM 扫描/候选排序算法（仍使用现有 `AutoLPAnalysis` 结果）
- 不新增新的 swap/合约调用方式（严格复用现有 `ExitTaskToUSDTWithOptions` 与 `EnterTaskFromUSDTWithOptions` 以及 `switch` 状态机）
- 不做跨用户资金调度或统一资金池（用户隔离）

## Decisions
### Decision: 收益指标
- 默认以 `AutoLPAnalysis.FeeRate5mPct`（5m 手续费/TVL，百分比）作为“收益率”口径用于换仓对比。
- `switch_min_improvement_pct` 作为相对提升阈值：`target >= current * (1 + pct/100)` 且 `target > current`。

### Decision: 目标池选择
- 默认使用“Top1 候选池”（即当前扫描结果中排序后的第一个 `Action=CANDIDATE` 且对该用户可执行的候选）作为换仓目标，以避免频繁切换到非顶级池。

### Decision: 冷却时间与存储
- 在 `auto_lp_user_configs` 增加：
  - `switch_cooldown_seconds`（默认 300）
- 冷却判定 MUST 以“换仓完成（新仓开仓成功）”为起点：`now - last_switch_completed_at < switch_cooldown_seconds` 时禁止再次换仓。
- `last_switch_completed_at` 可通过读取该用户最新的 `AutoLPEventSwitch` 事件时间戳得到（无需额外持久化字段）。
- `switch_min_improvement_pct=0` 表示禁用换仓（不做任何换仓评估与调度）。

### Decision: 并发与幂等策略
- 用户级互斥：复用 AutoLP 的 `KeyedLimiter`，保证同一用户扫描执行不会并发。
- DB 原子性：调度换仓时使用事务/条件更新，确保：
  - 仅当被替换任务 `exit_pending_action` 为空且无 `rebalance_pending` 时才写入 `switch` 状态
  - 同时更新 `last_switch_at`，避免下一轮扫描重复调度
- 进行中保护：若用户存在任何 `exit_pending_action=switch` 的任务，则跳过本轮换仓评估。

## Risks / Trade-offs
- 误判风险：`FeeRate5mPct` 是短周期指标，可能噪声较大；通过提升阈值与冷却时间降低抖动。
- Gas 成本：换仓需要撤仓+开仓，Gas 明显增加；阈值应由用户按偏好设置。
- 数据缺失：若某任务对应池子在本轮快照中缺少 `FeeRate5mPct`，需要定义缺省行为（例如视为 0 或跳过）。

## Migration Plan
- 依赖 GORM AutoMigrate 自动添加 `auto_lp_user_configs` 新列
- 默认配置保持“不换仓”（阈值为 0 时禁用换仓），避免升级后用户自动行为变化

## Open Questions
- 无（已确认：收益口径 FeeRate5mPct；目标池=Top1 候选；冷却起点=换仓完成；阈值=0 禁用换仓）
