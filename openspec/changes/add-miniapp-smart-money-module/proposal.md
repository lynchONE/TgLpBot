# Change: Add Smart Money module to MiniApp

## Why
- Users want a MiniApp view of recent "smart money" activity already tracked via SmartLP.
- The bot has `/smart_money` ranking, but MiniApp currently lacks a dedicated dashboard.
- The goal is to quickly answer:
  - In the last 1 hour, which pools did SmartLP wallets participate in?
  - In the last 24 hours, what is the profit/performance of those wallets?

## What Changes
- MiniApp:
  - Add a new "Smart Money" tab/module.
  - Show a "Last 1h Pools" list ranked by unique wallet participation (SmartLP add-liquidity events).
  - Show a "Last 24h Wallet PnL" list for wallets surfaced by the 1h pools list.

- Backend:
  - Add a MiniApp API endpoint that returns the aggregated data needed by the UI.
  - Reuse existing SmartLP ClickHouse ingestion (`smart_lp_events`) as the data source.

- SmartLP ingestion (recommended for PnL accuracy):
  - Record per-event wallet net token cashflow (via ERC20 `Transfer` logs) for V3 events as well (V4 already uses receipt transfer magnitudes).
  - Use these net flows to compute a "cashflow PnL" approximation for the last 24 hours.

## Impact
- Affected specs (new):
  - `specs/miniapp-smart-money/spec.md`
- Affected code (implementation stage):
  - Backend:
    - `backend/service/web_server/server.go`
    - `backend/service/web_server/smart_money.go` (new)
    - `backend/service/smart_lp/*` (SmartLP query helpers if needed)
    - `backend/service/smart_lp/smart_lp_monitor.go` (net flow fields, recommended)
    - `backend/base/clickhouse/clickhouse.go` (schema migration, recommended)
  - MiniApp:
    - `miniapp/src/App.jsx`
    - `miniapp/src/lib/api.js`
    - `miniapp/src/components/SmartMoneyCard.jsx` (new)
    - `miniapp/api/smart_money.js` (new Vercel proxy)
- Backwards compatibility: additive (new endpoint + new UI tab)

## Decisions (confirmed)
1. Profit definition: **Option A** (cashflow PnL = value_in(remove/collect) - value_out(add), converted to USDT using current token USD prices).
2. Default limits: show **top 10 pools** (last 1h) and **top 50 wallets** (wallet PnL list).
3. Access control: add a **SmartMoneyEnabled** permission; admins can grant/revoke this permission per user (configurable).
