## ADDED Requirements

### Requirement: Smart Money "金狗通知" tab
The MiniApp Smart Money module MUST provide a tab named `金狗通知`.

The tab MUST allow the user to configure an alert rule based on:
- `enabled` toggle
- `min_wallets` threshold (distinct wallets)
- `window_minutes` lookback window
- `cooldown_minutes` per-pool cooldown

The tab MUST reuse the user's existing Bark configuration stored in `GlobalConfig`.

#### Scenario: User opens 金狗通知 tab
- **WHEN** the user opens the Smart Money module and switches to `金狗通知`
- **THEN** the UI renders the alert configuration form

#### Scenario: User saves config
- **WHEN** the user updates the threshold and saves
- **THEN** the backend persists the config and the worker uses it for future alerts

### Requirement: Golden-dog Bark notification
When enabled, the backend MUST send a Bark notification if a pool has at least `min_wallets` distinct wallets adding liquidity within `window_minutes`.

The notification body MUST include:
- Pool pair name (e.g. `TOKEN0/TOKEN1`)
- Wallet count
- A call-to-action message: `建议立即关注`

#### Scenario: Pool triggers alert
- **GIVEN** golden-dog alerts are enabled with `min_wallets = 3` and `window_minutes = 10`
- **WHEN** 3 distinct wallets emit `action='add'` events for the same pool within the last 10 minutes
- **THEN** the backend sends a Bark push: `该交易对已出现 3 个钱包加 LP，建议立即关注`

#### Scenario: Bark not configured
- **GIVEN** golden-dog alerts are enabled
- **WHEN** the user's `GlobalConfig` has Bark disabled or missing Bark key
- **THEN** the backend MUST NOT send Bark notifications

### Requirement: Per-pool cooldown
The backend MUST enforce a per-user per-pool cooldown to prevent repeated alerts.

#### Scenario: Same pool re-triggers within cooldown
- **GIVEN** a pool has already triggered an alert
- **WHEN** the pool meets the threshold again within `cooldown_minutes`
- **THEN** the backend MUST NOT send a second Bark notification

### Requirement: Golden-dog config API
The backend MUST expose `GET/POST /api/smart_money_golden_dog_config`.

The endpoints MUST:
- Require Telegram WebApp `initData` authentication and MiniApp permission checks
- Require Smart Money permission (or admin)
- Store configuration per `user_id` and `chain`
- Return JSON (`Content-Type: application/json`)

#### Scenario: Unauthorized caller
- **WHEN** `initData` is missing/invalid
- **THEN** the endpoint returns HTTP `401`

#### Scenario: Successful update
- **WHEN** the caller updates golden-dog config with valid inputs
- **THEN** the endpoint returns HTTP `200` with the saved config

