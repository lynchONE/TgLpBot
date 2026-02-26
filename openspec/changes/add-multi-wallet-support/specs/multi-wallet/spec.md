## ADDED Requirements

### Requirement: Global multi-wallet mode
The system MUST support per-user wallet selection mode via `GlobalConfig.multi_wallet_enabled`.

When `multi_wallet_enabled=false`:
- Manual open/exit flows MUST use the user’s default wallet (`wallets.is_default=true`).
- The Bot and Mini App MUST NOT prompt the user to select a wallet.

When `multi_wallet_enabled=true`:
- Manual open flows MUST require the user to choose which wallet to use when the user has more than one wallet.
- The selected wallet MUST be persisted on the created task so that later exits/retries use the same wallet.

#### Scenario: Single-wallet mode uses default wallet
- **GIVEN** the user has imported at least one wallet and has a default wallet
- **AND** `multi_wallet_enabled=false`
- **WHEN** the user opens a new position
- **THEN** the system uses the default wallet without prompting for wallet selection.

#### Scenario: Multi-wallet mode prompts wallet selection
- **GIVEN** the user has 2 wallets (A and B)
- **AND** `multi_wallet_enabled=true`
- **WHEN** the user starts a manual open flow (Bot `/newposition` or Mini App open modal)
- **THEN** the UI prompts the user to pick wallet A or B before submitting the open.

### Requirement: Strategy tasks are bound to wallets
Each strategy task MUST record the wallet used to execute it:
- `strategy_tasks.wallet_id` (references `wallets.id`, no DB FK required)
- `strategy_tasks.wallet_address` (address snapshot for display/debug)

All task-related on-chain transactions MUST use the wallet bound to the task, including:
- entry (zap-in)
- exit (remove + swap-to-stable)
- dust swap
- rebalance re-entry
- exit retry / swap retry flows

#### Scenario: Exit uses the same wallet as entry
- **GIVEN** a task was created with `wallet_id=W1` and `wallet_address=addr1`
- **WHEN** the user requests stop/exit for that task
- **THEN** the backend signs and sends the exit transaction using wallet `W1` (addr1).

### Requirement: Open-position API supports wallet selection
`POST /api/trading?endpoint=open_position` MUST accept an optional `wallet_id`.

When `multi_wallet_enabled=true` AND the user has more than one wallet:
- `wallet_id` MUST be provided
- `wallet_id` MUST belong to the authenticated user

When `multi_wallet_enabled=false` OR the user has only one wallet:
- `wallet_id` MAY be omitted
- The backend MUST fallback to the default wallet

#### Scenario: Missing wallet_id returns 400 in multi-wallet mode
- **GIVEN** `multi_wallet_enabled=true`
- **AND** the user has 2 wallets
- **WHEN** the Mini App calls open-position API without `wallet_id`
- **THEN** the backend returns HTTP `400` (wallet_id required).

### Requirement: Realtime positions uses task wallet for balances
The realtime positions response MUST compute wallet token balances using the wallet bound to each task, not always the default wallet.

#### Scenario: Two tasks on two wallets show different wallet balances
- **GIVEN** the user has wallet A and wallet B
- **AND** the user has two tasks, one bound to wallet A and one bound to wallet B
- **WHEN** the Mini App loads realtime positions
- **THEN** each position’s wallet balances are computed against its bound wallet address.

