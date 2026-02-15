## 1. Backend: Smart Money pool adds details API
- [x] 1.1 Register `GET /api/smart_money_pool_adds` route
- [x] 1.2 Implement handler with:
  - [x] 1.2.1 MiniApp auth + SmartMoney permission checks
  - [x] 1.2.2 Query ClickHouse for aggregated add events per wallet/position within `window_hours`
  - [x] 1.2.3 Enrich with pool metadata, tick range, computed price range, net amounts, USD estimates
  - [x] 1.2.4 (Optional) Compute per-wallet LP fee earnings estimate for the pool (best-effort; may use on-chain `eth_call` simulation)
  - [x] 1.2.5 Enforce limits and return stable empty states
- [x] 1.3 Add unit tests for handler auth/validation and response shape

## 2. MiniApp: Pool adds modal + API wiring
- [x] 2.1 Add Vercel proxy `miniapp/api/smart_money_pool_adds.js`
- [x] 2.2 Add API wrapper `fetchSmartMoneyPoolAdds` in `miniapp/src/lib/api.js`
- [x] 2.3 Add `SmartMoneyPoolAddsModal` UI component and integrate into `miniapp/src/components/SmartMoneyCard.jsx`
- [x] 2.4 Add loading/empty/error states and wallet actions (copy / follow / positions)

## 3. Validation
- [x] 3.1 Run `cd backend; go test ./...`
- [x] 3.2 Run `cd miniapp; npm run build`
- [x] 3.3 Run `openspec validate add-miniapp-smart-money-pool-adds-details --strict`
