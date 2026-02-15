## ADDED Requirements

### Requirement: Smart Money pool adds detail view
The Smart Money MiniApp MUST allow the user to open a pool detail view from the “recent pools” list.

The pool detail view MUST list SmartLP wallets that added liquidity to the selected pool within the requested window (default 2 hours).
The pool detail view MUST exclude wallet/position rows whose net liquidity delta within the window is `<= 0` (e.g., added then fully撤销, or net removed).

Each wallet row MUST include:
- `wallet_address`
- `tick_lower` and `tick_upper`
- computed price range for the ticks (lower/upper + base/quote symbols)
- added amounts (token0/token1) and an estimated USD value

The view SHOULD include a per-wallet “盈利” metric describing LP fee earnings for this pool (best-effort).
The metric SHOULD be an estimated “claimable fees” value converted to USD, and SHOULD be clearly labeled as an estimate.

#### Scenario: User opens pool adds detail
- **WHEN** the user taps a pool row in Smart Money “recent pools”
- **THEN** the UI fetches pool add-liquidity details and renders a wallet list with ranges and amounts

#### Scenario: Empty pool details
- **WHEN** the selected pool has no SmartLP add events within the window
- **THEN** the UI renders an empty state without errors

### Requirement: Smart Money pool adds API
The backend MUST expose `GET /api/smart_money_pool_adds` for the MiniApp.

The endpoint MUST:
- Require Telegram WebApp `initData` authentication and MiniApp permission checks
- Require Smart Money permission (or admin)
- Return JSON (`Content-Type: application/json`)
- Enforce sane defaults and response limits
- Return `503` when ClickHouse is not configured

#### Scenario: Missing pool parameters
- **WHEN** the client calls `GET /api/smart_money_pool_adds` without `pool_version` or `pool_id`
- **THEN** the backend returns HTTP `400`

#### Scenario: ClickHouse not configured
- **WHEN** ClickHouse is not configured or unavailable
- **THEN** `GET /api/smart_money_pool_adds` returns HTTP `503`

#### Scenario: Successful response
- **WHEN** the caller is authenticated and has Smart Money permission
- **THEN** `GET /api/smart_money_pool_adds` returns HTTP `200` with a JSON payload containing `pool` and `wallets`
