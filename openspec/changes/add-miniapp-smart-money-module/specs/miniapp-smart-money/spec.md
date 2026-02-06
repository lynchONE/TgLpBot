## ADDED Requirements

### Requirement: Smart Money Tab In MiniApp
The MiniApp MUST expose a "Smart Money" tab/module.

The module MUST show a "last 1 hour" pool participation list derived from SmartLP events.
Each pool row MUST include:
- `pool_version` and `pool_id`
- unique participating wallet count for the window

#### Scenario: User opens Smart Money tab
- **WHEN** the user switches to the Smart Money tab
- **THEN** the UI fetches Smart Money data from the backend API and renders the pools list

#### Scenario: Empty state when no recent events
- **WHEN** there are no SmartLP "add" events in the last 1 hour
- **THEN** the UI shows an empty state (no crash / no infinite loading)

### Requirement: Smart Money Overview API
The backend MUST expose `GET /api/smart_money_overview` for the MiniApp.

The endpoint MUST:
- Require Telegram WebApp `initData` authentication and MiniApp permission checks
- Require Smart Money permission (or admin)
- Return JSON (`Content-Type: application/json`)
- Enforce sane defaults and response limits (pool limit, wallet limit)
- Return `503` when ClickHouse is not configured

#### Scenario: ClickHouse not configured
- **WHEN** ClickHouse is not configured or unavailable
- **THEN** `GET /api/smart_money_overview` returns HTTP `503`

#### Scenario: Successful response
- **WHEN** the caller is authenticated and ClickHouse is available
- **THEN** `GET /api/smart_money_overview` returns HTTP `200` with a JSON payload containing `pools` and `wallets_24h`

#### Scenario: Smart Money permission required
- **WHEN** the caller is authenticated but does not have Smart Money permission
- **THEN** `GET /api/smart_money_overview` returns HTTP `403`

### Requirement: Last 24h Wallet Profit
The Smart Money overview MUST include a "last 24 hours" profit summary for the wallets surfaced by the "last 1 hour" pool list.

The profit summary MUST include, per wallet:
- `wallet_address`
- `pnl_usdt_24h` (profit metric as defined by the change proposal)

#### Scenario: Wallet list is derived from recent pools
- **WHEN** a wallet appears in the last 1 hour pools list
- **THEN** the same wallet appears in the `wallets_24h` section of the API response (subject to response limits)
