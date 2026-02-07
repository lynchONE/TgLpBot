## 1. Backend: Smart Money overview window update
- [x] 1.1 Update `GET /api/smart_money_overview` default `pools_window_hours` to 2
- [x] 1.2 Ensure wallet ranking window matches 24h participation (aligned with `pnl_window_hours` default 24)
- [ ] 1.3 Add/adjust unit tests to cover window defaults and response fields

## 2. Backend: Wallet LP positions endpoint
- [x] 2.1 Register `GET /api/smart_money_wallet_positions`
- [x] 2.2 Implement on-chain position fetch for a given `wallet_address`:
  - [x] 2.2.1 Support V3 (Pancake/Uniswap NPM) active liquidity positions
  - [x] 2.2.2 Support V4 (Uniswap V4 PositionManager) token scan
  - [x] 2.2.3 Compute per-position amounts (token0/token1) and USD value
  - [x] 2.2.4 Add caching + concurrency limits + timeouts
- [ ] 2.3 Add unit tests for handler auth/permission and response shape

## 3. MiniApp: Smart Money UI improvements
- [x] 3.1 Update pools section copy to reflect 2h window (prefer using API returned window seconds)
- [x] 3.2 Add `lightweight-charts` visualization for wallet 24h PnL distribution/trend
- [x] 3.3 Add wallet detail drawer/modal that loads and renders current LP positions

## 4. Validation
- [x] 4.1 Run `cd backend; go test ./...`
- [x] 4.2 Run `cd miniapp; npm run build`
