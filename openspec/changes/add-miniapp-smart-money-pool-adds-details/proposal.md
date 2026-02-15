# Change: Add Smart Money pool add-liquidity details in MiniApp

## Why
- Smart Money 目前只提供「最近参与池子」聚合榜单与「钱包 24h PnL」概览；缺少按池子下钻到“哪些钱包在最近 2h 加了池子、加了什么区间、加了多少”的信息。
- 用户需求是快速判断“热池子里聪明钱具体怎么加”的细节，并可进一步查看钱包仓位与跟单配置。

## What Changes
- Backend:
  - Add `GET /api/smart_money_pool_adds` (MiniApp auth + SmartMoney permission).
  - Data source: ClickHouse `smart_lp_events` (SmartLP ingestion).
  - Return aggregated “add liquidity” details for a given pool in a time window (default `window_hours=2`):
    - Per wallet (and per position/range when applicable): tick range, computed price range, net token amounts, and estimated USD value.
    - Optional: per-wallet **LP fee earnings estimate** for this pool (best-effort), for example:
      - V3: simulate `NonfungiblePositionManager.collect` via `eth_call` to get current claimable amounts for the tokenIds observed in the window, then convert to USD with current token prices.
      - V4: may be omitted initially (tokenId is not available from ModifyLiquidity events; can be added later by scanning PositionManager-owned tokenIds per wallet).
- MiniApp:
  - Make “recent pools” rows clickable; open a pool detail modal/drawer.
  - The modal lists wallets that added liquidity in the window, showing range + amounts.
  - Provide quick actions: copy wallet, open wallet positions modal, open follow-config modal.
- Vercel proxy:
  - Add `miniapp/api/smart_money_pool_adds.js` forwarding to backend `/api/smart_money_pool_adds`.
  - Add a MiniApp API wrapper `fetchSmartMoneyPoolAdds`.

## Impact
- Affected capability (delta spec):
  - `specs/miniapp-smart-money/spec.md`
- Affected code (implementation stage):
  - Backend:
    - `backend/service/web_server/server.go`
    - `backend/service/web_server/smart_money_pool_adds.go` (new)
  - MiniApp:
    - `miniapp/api/smart_money_pool_adds.js` (new)
    - `miniapp/src/lib/api.js`
    - `miniapp/src/components/SmartMoneyCard.jsx`
    - `miniapp/src/components/SmartMoneyPoolAddsModal.jsx` (new)
- Backwards compatibility: additive (new endpoint + UI entry)

## Decisions (proposed)
1. Window defaults:
   - Pool detail uses `window_hours=2` by default (align with Smart Money pool ranking default).
2. Amount definition:
   - Prefer `net_amount*` when non-zero; otherwise fall back to `amount*` (same rule as overview cashflow query).
3. Profit definition:
   - “盈利” is defined as the wallet’s LP fee situation on this pool.
   - Show **estimated claimable fee value (USD)** for positions tied to the pool (best-effort; may require on-chain calls).
   - The metric MUST be labeled as an estimate and MAY be missing when RPC/calls fail or when pool version does not expose tokenId from events.
