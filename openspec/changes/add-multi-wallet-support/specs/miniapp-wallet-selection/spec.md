## ADDED Requirements

### Requirement: Wallet list endpoint for Mini App
The backend MUST expose `POST /api/settings?endpoint=wallets` for the Mini App to fetch the user’s wallets.

The endpoint MUST:
- Authenticate the caller via Telegram WebApp `initData`
- Enforce Mini App permission checks
- Return the caller’s wallets only (no cross-user access)
- Support an optional `chain` parameter to return best-effort balances for that chain

#### Scenario: Unauthorized caller
- **WHEN** `initData` is missing/invalid
- **THEN** the endpoint returns HTTP `401`.

#### Scenario: Permission required
- **WHEN** the caller is authenticated but does not have Mini App permission
- **THEN** the endpoint returns HTTP `403`.

#### Scenario: Successful wallets fetch with balances
- **GIVEN** the user has imported at least one wallet
- **WHEN** the Mini App calls the endpoint with `chain=bsc`
- **THEN** the response includes wallet list with `id/name/address/is_default` and best-effort `stable/native` balances.

### Requirement: Mini App open-position supports wallet selection in multi-wallet mode
The Mini App MUST support wallet selection during open-position when `GlobalConfig.multi_wallet_enabled=true` and the user has more than one wallet.

When `GlobalConfig.multi_wallet_enabled=true` AND the user has more than one wallet:
- The Mini App open-position modal MUST show a wallet picker
- The picker MUST display each wallet’s balance information (at least the chain stable token balance)
- The open-position request MUST include the selected `wallet_id`

When `GlobalConfig.multi_wallet_enabled=false`:
- The Mini App MUST keep the existing behavior (no wallet picker, default wallet is used).

#### Scenario: Multi-wallet open sends wallet_id
- **GIVEN** `multi_wallet_enabled=true` and the user has 2 wallets
- **WHEN** the user submits an open-position from the Mini App after selecting wallet B
- **THEN** the Mini App sends `wallet_id` for wallet B in the open-position request.
