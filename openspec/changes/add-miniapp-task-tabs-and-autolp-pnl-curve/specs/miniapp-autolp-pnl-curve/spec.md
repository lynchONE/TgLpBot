## ADDED Requirements

### Requirement: Auto 任务页展示本次开启盈利曲线
当用户在 MiniApp「实时仓位」页面切换到 `Auto任务` 标签时，页面 MUST 展示“本次 AutoLP 开启窗口”的盈利曲线图。

盈利曲线 MUST 展示“累计已实现盈利（USDT，已扣除 Gas）”，并同时展示“未平仓浮动盈亏（USDT，当前估算）”与“总收益（已实现+未实现）”。

#### Scenario: Auto 标签展示盈利曲线
- **WHEN** 用户切换到 `Auto任务` 标签
- **THEN** 页面展示盈利曲线组件并加载曲线数据

#### Scenario: 无交易时展示 0 曲线
- **WHEN** 本次开启窗口内不存在任何已平仓交易记录
- **THEN** 页面仍可展示可用的曲线视图（累计盈利为 0），并提示“暂无已实现收益”

#### Scenario: 每次开/平仓可直观看到变化
- **WHEN** 本次开启窗口内发生过开仓与平仓
- **THEN** 曲线视图对开仓/平仓事件做标记，开仓点展示开仓金额与交易对，平仓点展示收益与交易对

### Requirement: AutoLP 盈利曲线 API
后端 MUST 提供 `POST /api/autolp_pnl_curve` API，并要求 Telegram WebApp `initData` 认证与 MiniApp 权限校验。

API MUST 返回用于绘制曲线的时间序列数据，并包含窗口信息：
- `window_start` / `window_end`（若可用）
- `window_label`（例如“本次开启至今 / 上次开启 / 全部历史”）

曲线数据源 MUST 基于：
- `trade_records`（使用 `profit_usdt` 作为已实现收益，且包含 Gas 影响）
- `strategy_tasks.is_auto = 1`
- 时间窗口由 `auto_lp_user_configs.last_enabled_at/last_disabled_at/enabled` 推导
- 当前未平仓浮动盈亏 MUST 基于用户当前运行中的 Auto 任务计算（不要求回溯历史浮动曲线）

#### Scenario: 仅返回窗口内数据
- **WHEN** 用户存在 `last_enabled_at`，且本次开启窗口为 `[last_enabled_at, window_end]`
- **THEN** API 仅返回该窗口内的交易/事件数据用于绘制曲线

### Requirement: 交易数量限制
`/api/autolp_pnl_curve` MUST 限制返回的交易记录数量为最近 400 笔（按时间倒序截断），并在发生截断时返回 `truncated=true`。

#### Scenario: 超过 400 笔时截断
- **WHEN** 窗口内交易记录数量超过 400
- **THEN** API 仅返回最近 400 笔，并设置 `truncated=true`
