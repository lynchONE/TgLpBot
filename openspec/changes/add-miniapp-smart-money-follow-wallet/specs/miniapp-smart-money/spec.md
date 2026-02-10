## ADDED Requirements

### Requirement: Smart Money wallet LP follow (跟单) configuration
The MiniApp MUST allow users to configure LP copy-trading (跟单) for a specific Smart Money wallet.

The configuration MUST support:
- Enable/disable per target wallet
- `per_trade_amount_usdt` (single follow amount)
- `max_total_amount_usdt` (cap on total allocated amount across active follow positions)
- Random delay in seconds within a configurable range (<= 60s)

#### Scenario: User enables follow for a wallet
- **WHEN** the user enables follow for a target wallet
- **THEN** the backend stores the config and starts following only future add/remove events for that wallet

#### Scenario: User disables follow for a wallet
- **WHEN** the user disables follow for a target wallet
- **THEN** the backend stops scheduling new follow actions for that wallet

### Requirement: Smart Money follow config API
The backend MUST expose `GET /api/smart_money_follow_config` and `POST /api/smart_money_follow_config`.

The endpoints MUST:
- Require Telegram WebApp `initData` authentication and MiniApp permission checks
- Require Smart Money permission (or admin)
- Allow per-wallet configuration keyed by `wallet_address`
- Return JSON (`Content-Type: application/json`)

#### Scenario: Unauthorized caller
- **WHEN** `initData` is missing/invalid
- **THEN** the endpoint returns HTTP `401`

#### Scenario: Permission required
- **WHEN** the caller is authenticated but does not have Smart Money permission
- **THEN** the endpoint returns HTTP `403`

#### Scenario: Successful update
- **WHEN** the caller updates follow config with valid inputs
- **THEN** the endpoint returns HTTP `200` with the saved config

### Requirement: Follow worker executes delayed add/remove
The backend MUST implement a background worker that:
- Reads target wallet SmartLP add/remove events from ClickHouse
- Schedules follow actions with random delay (<= 60s)
- Enforces `max_total_amount_usdt` and `per_trade_amount_usdt`
- Applies actions only to tasks created by follow (no interference with manual/AutoLP tasks)
- Opens LP with the same tick range (`tick_lower`/`tick_upper`) as the target wallet event

#### Scenario: Target wallet adds liquidity
- **WHEN** the target wallet emits an `add` event for a pool
- **THEN** the worker creates a follow open job and enters the same pool (same tick range) after the configured random delay

#### Scenario: Follow open range matches target wallet
- **WHEN** a follow open is executed for a target wallet `add` event
- **THEN** the created position uses the exact `tick_lower` and `tick_upper` from that event (no fallback range)

#### Scenario: Target wallet removes liquidity
- **WHEN** the target wallet emits a `remove` event for a pool
- **THEN** the worker creates a follow close job and requests exit for the corresponding follow task after the configured random delay
