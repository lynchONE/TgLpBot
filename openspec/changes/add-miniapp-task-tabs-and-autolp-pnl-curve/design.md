## Context
MiniApp「实时仓位」页面当前直接展示 `/api/realtime_positions` 的仓位/任务卡片，缺少对“手动任务 vs Auto 任务”的快速筛选能力。

同时，AutoLP 已有：
- `auto_lp_user_configs.last_enabled_at/last_disabled_at/enabled`：可确定“本次/上次开启窗口”
- `trade_records`：已记录每笔开/平仓的 `profit_usdt`（已扣除 Gas）与 `opened_at/closed_at`
- `strategy_tasks.is_auto`：可区分 Auto 任务与手动任务

因此可在不引入新表的前提下，为 MiniApp 增加任务标签筛选与本次 Auto 盈利曲线展示。

## Goals / Non-Goals
### Goals
- MiniApp 实时仓位页支持三个标签：全部 / 手动任务 / Auto 任务
- Auto 标签页展示“本次开启窗口”的累计已实现盈利曲线，并对每次开/平仓进行可视化标记
- 后端 API 与现有数据结构兼容（新增字段/新增端点，不破坏旧客户端）

### Non-Goals
- 不实现“未平仓浮动盈亏曲线”（除非确认需要；默认以已实现收益为主）
- 不新增复杂的聚合/抽样系统（数据点过多时仅做简单截断或后续再优化）

## Backend Design
### 1) `/api/realtime_positions` 增量字段
在 `position` 对象中增加：
- `task_is_auto: boolean`（当 `task_id>0` 时可靠）

实现时需要覆盖：
- V3 position（通过 task 关联）
- V4 position（task 直接存在）
- pending task 占位卡片（无 tokenId 但 `exit_pending_action`/`rebalance_pending` 等）

### 2) 新增 `/api/autolp_pnl_curve`
**Route**
- `POST /api/autolp_pnl_curve`
- Body: `{ "initData": "<telegram-initData>" }`

**Auth**
- 与现有 MiniApp API 一致：`initData` 校验 + `requireMiniAppPermission` + `requireAutoModePermission`

**Window**
- 复用 `auto_lp_user_configs` 推导窗口：
  - Auto 开启中：`[last_enabled_at, now]`（label: “本次开启至今”）
  - Auto 已关闭且有 `last_disabled_at`：`[last_enabled_at, last_disabled_at]`（label: “上次开启”）
  - 无 `last_enabled_at`：视为“全部历史”

**Data source**
- 从 `trade_records` 构造事件：
  - open event：`opened_at`（来自 `trade_records.opened_at`，status=open/closed 都可产生 open 事件）
  - close event：`closed_at`（仅 `status=closed` 且 `closed_at` 非空）
- 过滤条件：
  - `trade_records.user_id = user_id`
  - `JOIN strategy_tasks` 且 `strategy_tasks.is_auto = 1`
  - 按窗口起止过滤 `opened_at/closed_at`

**Processing**
- 将 open/close events 合并后按时间排序
- 计算累计已实现盈利 `cum_profit_usdt`：仅在 close event 时将 `profit_usdt` 累加
- 输出 `series_realized`（已实现累计折线）与 `events`（用于标记/列表）
- 计算当前未平仓浮动盈亏 `unrealized_profit_usdt`（基于运行中的 Auto 任务的当前估值）并返回：
  - `realized_profit_usdt`（窗口内已实现累计，含 Gas）
  - `unrealized_profit_usdt`（当前未平仓浮动，估算）
  - `total_profit_usdt = realized + unrealized`
  - `series_total`：在 `series_realized` 基础上追加一个 `now` 点（`value=total_profit_usdt`），用于在曲线上直观看到“当前总收益”

**Response shape（建议）**
```json
{
  "ok": true,
  "window_label": "本次开启至今",
  "window_start": "2026-01-11T12:00:00Z",
  "window_end": "2026-01-11T12:10:00Z",
  "realized_profit_usdt": 1.23,
  "unrealized_profit_usdt": -0.45,
  "total_profit_usdt": 0.78,
  "events": [
    { "type": "open", "t": 1730000000, "trade_id": 1, "task_id": 10, "pair": "AAA/USDT", "open_usdt": 100.0 },
    { "type": "close", "t": 1730000600, "trade_id": 1, "task_id": 10, "pair": "AAA/USDT", "profit_usdt": 1.23, "profit_pct": 0.12, "cum_profit_usdt": 1.23 }
  ],
  "series_realized": [
    { "t": 1730000000, "value": 0 },
    { "t": 1730000600, "value": 1.23 }
  ],
  "series_total": [
    { "t": 1730000000, "value": 0 },
    { "t": 1730000600, "value": 1.23 },
    { "t": 1730000666, "value": 0.78 }
  ]
}
```

> 注：`t` 建议使用 unix seconds，便于 lightweight-charts 直接使用。

## MiniApp UI Design
### Tabs
- 在「实时仓位」视图顶部（任务列表上方）增加三段式标签：
  - 全部（默认，保持当前展示逻辑）
  - 手动任务（`task_id>0 && task_is_auto===false`）
  - Auto 任务（`task_id>0 && task_is_auto===true`）

### Auto 盈利曲线卡片
- 仅在 `Auto任务` 标签下展示：
  - 累计盈利（USDT）
  - 曲线图（折线）
  - 开/平仓事件标记（或简短事件列表）

### Polling
- `realtime_positions` 仍按现有 1s 轮询
- `autolp_pnl_curve` 建议仅在 Auto 标签激活时拉取，并降低频率（例如 10–30s）或手动刷新，避免无意义的高频 DB 查询
