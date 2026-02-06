## 1. Backend: SmartLP net cashflow fields (recommended)
- [x] 1.1 Extend ClickHouse `smart_lp_events` schema with `net_amount0` and `net_amount1` (String) columns
- [x] 1.2 Update SmartLP monitor to fill `net_amount0/net_amount1` for V3 events using ERC20 `Transfer` logs (same approach as V4)
- [x] 1.3 Add helper(s) to prefer `net_amount*` for PnL computations (fallback to `amount*` when missing)

## 2. Backend: Smart Money MiniApp API
- [x] 2.1 Register `GET /api/smart_money_overview` in the web server
- [x] 2.2 Implement aggregation:
  - [x] 2.2.1 Last 1h top pools (wallet_count, added_liquidity)
  - [x] 2.2.2 Top wallets across the selected pools (by event count)
  - [x] 2.2.3 Last 24h wallet PnL for wallets surfaced by 2.2.2
- [x] 2.3 Add Go unit tests for handler status codes and response shape

## 3. MiniApp: Smart Money UI
- [x] 3.1 Add Vercel proxy `miniapp/api/smart_money.js`
- [x] 3.2 Add API wrapper `fetchSmartMoneyOverview` in `miniapp/src/lib/api.js`
- [x] 3.3 Add `SmartMoneyCard` component and integrate a new tab in `miniapp/src/App.jsx`

## 4. Validation
- [x] 4.1 Run `cd backend; go test ./...`
- [x] 4.2 Run `cd miniapp; npm run build`
