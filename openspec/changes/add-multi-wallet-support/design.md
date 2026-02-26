# Design: Multi-wallet support

## Data Model

### GlobalConfig
Add:
- `multi_wallet_enabled` (bool, default `false`)

Rationale:
- Keeps default UX unchanged (single-wallet).
- When enabled, forces explicit wallet choice for manual opens to reduce “wrong wallet” operations.

### StrategyTask
Add:
- `wallet_id` (uint, default `0`, indexed)
- `wallet_address` (string, default `''`, indexed)

Wallet resolution rules (runtime):
1) If `wallet_id != 0` and belongs to `user_id` → use that wallet.
2) Else if `wallet_address` is set and matches a wallet record for `user_id` → use that wallet.
3) Else → fallback to the user’s default wallet (legacy compatibility).

All task-related on-chain transactions MUST use the resolved wallet.

## Bot UX (manual open)

### Single-wallet mode (`multi_wallet_enabled=false`)
Unchanged:
- `/newposition` uses default wallet and does not prompt wallet selection.

### Multi-wallet mode (`multi_wallet_enabled=true`)
Flow (high level):
1) Select chain (existing behavior)
2) Select wallet (new):
   - Show each wallet: name + address (short) + native/stable balance for the selected chain
3) Continue pool input → range → amount → confirm

Persist wallet choice:
- Store selected `wallet_id` in session during the wizard
- On task creation: set `strategy_tasks.wallet_id` + `wallet_address`

## Mini App UX (open position modal)
- When `multi_wallet_enabled=false`: keep current UI and do not send `wallet_id`.
- When `multi_wallet_enabled=true` and user has > 1 wallet:
  - Fetch wallets via `POST /api/settings?endpoint=wallets` with current `chain`
  - Show wallet selector with balances
  - Require a selection and send `wallet_id` to open-position API

## Web API

### Wallets endpoint
`POST /api/settings?endpoint=wallets`

Request JSON:
- `initData` (string, required)
- `chain` (string, optional; default `bsc`)

Response JSON:
- `ok` (bool)
- `wallets` (array):
  - `id`, `name`, `address`, `is_default`
  - `balances` (optional best-effort): `native_symbol`, `native_balance`, `stable_symbol`, `stable_balance`

### Open position
`POST /api/trading?endpoint=open_position`
- Add optional `wallet_id`
- If `multi_wallet_enabled=true` and user has > 1 wallet: `wallet_id` required and must belong to user
- On task creation: always persist `wallet_id` + `wallet_address` for manual opens (Bot/Mini App)

## Realtime Positions / Task Views
- Wallet-specific fields should not break existing clients:
  - Keep `wallet` field as default wallet summary for backward compatibility
  - Compute per-position wallet balances using the task’s resolved wallet address
  - Summary token de-duplication key should include wallet address to avoid cross-wallet undercounting

