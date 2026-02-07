## ADDED Requirements

### Requirement: Smart Money pools ranking defaults to last 2 hours
The Smart Money overview MUST rank pools by unique wallet participation over the last **2 hours** by default.

#### Scenario: Client omits pools window
- **WHEN** the client calls `GET /api/smart_money_overview` without `pools_window_hours`
- **THEN** the backend uses a 2 hour window for pool ranking

### Requirement: Smart Money wallets participation defaults to last 24 hours
The Smart Money overview MUST treat “wallet participation” as “wallet has SmartLP add/remove events within the last 24 hours” by default.

#### Scenario: Client omits pnl window
- **WHEN** the client calls `GET /api/smart_money_overview` without `pnl_window_hours`
- **THEN** the backend uses a 24 hour window for wallet participation and PnL computations

### Requirement: Smart Money wallet positions API
The backend MUST expose `GET /api/smart_money_wallet_positions` for the MiniApp.

The endpoint MUST:
- Require Telegram WebApp `initData` authentication and MiniApp permission checks
- Require Smart Money permission (or admin)
- Accept `wallet_address` and optional `chain` query parameters
- Return JSON (`Content-Type: application/json`)

#### Scenario: Unauthorized caller
- **WHEN** `initData` is missing/invalid
- **THEN** `GET /api/smart_money_wallet_positions` returns HTTP `401`

#### Scenario: Permission required
- **WHEN** the caller is authenticated but does not have Smart Money permission
- **THEN** `GET /api/smart_money_wallet_positions` returns HTTP `403`

#### Scenario: Successful response
- **WHEN** the caller is authenticated and has permission
- **THEN** `GET /api/smart_money_wallet_positions` returns HTTP `200` with a JSON payload containing `wallet_address` and `positions`

### Requirement: MiniApp provides TradingView-like wallet PnL visualization
The Smart Money MiniApp MUST provide a `lightweight-charts` based visualization for “last 24h wallet PnL” to improve readability.

#### Scenario: User views Smart Money wallets section
- **WHEN** the user opens the Smart Money tab and wallet data exists
- **THEN** the UI renders a chart view in addition to the Top wallets list

### Requirement: MiniApp wallet LP positions detail view
The Smart Money MiniApp MUST allow users to open a wallet detail view showing current LP positions and range amounts.

#### Scenario: User opens wallet details
- **WHEN** the user taps a wallet row in the “last 24h wallet PnL” list
- **THEN** the UI fetches `GET /api/smart_money_wallet_positions` and renders position range and valuation details

