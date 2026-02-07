# Change: Update Smart Money windows + wallet LP positions details

## Why
- Smart Money「参与池子」榜更适合看短周期热度；将统计口径从 24h 调整为 2h 更符合“近期参与”的直觉。
- MiniApp 目前的 24h 钱包盈亏展示偏表格化，信息密度高但不够直观；希望用 TradingView 风格（`lightweight-charts`）提升可读性。
- Smart Money 的核心价值是“跟单观察”：需要能查看 **最近 24h 有参与记录的钱包** 当前 LP 仓位（区间、金额、是否在区间内、仓位价值等），而不仅是历史流入/流出与 PnL 汇总。

## What Changes
- Backend:
  - 调整 `GET /api/smart_money_overview` 默认窗口：
    - pools 参与榜：默认 `pools_window_hours=2`
    - wallets 参与口径：默认仍以 24h 口径（与 PnL 窗口一致）
  - 新增 `GET /api/smart_money_wallet_positions`：
    - 输入：`wallet_address`（必填）、`chain`（默认 bsc）
    - 输出：该钱包当前 LP 仓位列表（V3/V4），包含区间信息与估值字段
    - 权限：Telegram WebApp initData + MiniApp + SmartMoney 权限（或 admin）
    - 性能：加缓存/限流/超时，避免 RPC 过载

- MiniApp:
  - 「参与池子」标题与描述改为明确的 2h（并根据后端返回窗口动态展示）。
  - 「钱包盈亏」在保留 Top 列表的同时，增加 TradingView 风格的可视化（`lightweight-charts`）。
  - 钱包行支持进入“仓位详情”抽屉/弹窗，按需请求 `smart_money_wallet_positions` 并展示区间金额等细节。

## Impact
- 影响范围：Smart Money 模块（MiniApp + Backend）
- 风险点：新增链上 RPC 读取（钱包仓位），可能带来延迟与限流；通过缓存、并发限制、返回上限控制风险。
- 兼容性：对现有客户端保持兼容（`smart_money_overview` 仅调整默认窗口；新增接口为 additive）。

