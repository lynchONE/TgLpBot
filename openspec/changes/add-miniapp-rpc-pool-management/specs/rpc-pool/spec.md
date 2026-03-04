## ADDED Requirements

### Requirement: Admin-managed RPC endpoint pool
The system MUST support a DB-backed RPC endpoint pool for EVM chains.

Each RPC endpoint record MUST include:
- `chain` (at least `bsc` and `base`)
- `transport` (`http` or `ws`)
- `url`
- availability status (at minimum: available vs unavailable)

#### Scenario: Admin adds a new HTTP RPC endpoint for BSC
- **GIVEN** an admin user is authenticated via Telegram WebApp `initData`
- **WHEN** the admin adds an `http` RPC endpoint for `bsc`
- **THEN** the endpoint is persisted and appears in the admin RPC pool list

### Requirement: Current endpoint selection per chain and transport
For each `(chain, transport)` pair, the system MUST have a notion of a “current” RPC endpoint.

The admin UI MUST show the current endpoint and the status of all other endpoints in the pool.

#### Scenario: Admin switches the current endpoint
- **GIVEN** there are multiple available RPC endpoints for the same `(chain, transport)`
- **WHEN** the admin switches the current endpoint to another available endpoint
- **THEN** subsequent RPC client selections use the newly selected endpoint

### Requirement: Monthly quota exhaustion disables an endpoint until next month
When the system detects that an RPC endpoint has exhausted its monthly quota, it MUST mark the endpoint as unavailable until the next month.

The system MUST NOT automatically select an unavailable endpoint as current.

#### Scenario: Quota exhausted triggers disable-until-next-month
- **GIVEN** the current RPC endpoint returns an error indicating monthly quota exhaustion
- **WHEN** the system attempts an RPC call through that endpoint
- **THEN** the endpoint is marked unavailable until the next month and removed from current selection

### Requirement: Automatic failover to another endpoint in the pool
When the current RPC endpoint becomes unavailable, the system MUST automatically switch to another available endpoint in the same pool for that `(chain, transport)` pair.

#### Scenario: Current endpoint becomes unavailable and system fails over
- **GIVEN** there is at least one other available endpoint in the pool for the same `(chain, transport)`
- **WHEN** the current endpoint becomes unavailable
- **THEN** the system selects another available endpoint as current without requiring a backend restart

### Requirement: Backward-compatible fallback to `.env`
When the RPC endpoint pool has no entries for a given `(chain, transport)`, the system MUST fall back to the corresponding `.env` configuration so existing deployments keep working.

#### Scenario: Empty pool uses `.env` configuration
- **GIVEN** the RPC endpoint pool is empty
- **WHEN** the backend starts and needs an RPC endpoint for `bsc` over `http`
- **THEN** it uses the configured `.env` RPC URL for that chain/transport

