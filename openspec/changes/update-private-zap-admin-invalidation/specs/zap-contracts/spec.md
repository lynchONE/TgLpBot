## MODIFIED Requirements

### Requirement: Private Zap binding invalidation
The system MUST allow administrators to invalidate existing private Zap bindings by chain from the Mini App admin interface.

After invalidation, the backend MUST treat the affected chain's existing private Zap bindings as unavailable for future Zap interactions until a fresh contract is deployed and rebound.

#### Scenario: Admin invalidates BSC private Zap bindings
- **GIVEN** chain `bsc` has existing `zap_simple` bindings in MySQL and Redis
- **WHEN** an administrator triggers private Zap invalidation for `bsc`
- **THEN** the backend clears the cached private Zap bindings for `bsc`
- **AND** marks the stored binding addresses for `bsc` as unavailable
- **AND** the next open on `bsc` deploys and binds a fresh private Zap before sending the entry transaction

### Requirement: Private Zap binding resolution
The system MUST resolve a wallet's private Zap binding by checking Redis first, then MySQL, and only deploying a new contract when no valid bound address exists.

The runtime MUST NOT require a configured private Zap version number to decide whether an existing binding is reusable.

#### Scenario: Redis miss falls back to DB
- **GIVEN** wallet `W` has a valid private Zap binding in MySQL on `base`
- **AND** Redis has no corresponding cache key
- **WHEN** the backend resolves private Zap address for wallet `W`
- **THEN** the backend reads the bound address from MySQL
- **AND** returns that address
- **AND** repopulates Redis

#### Scenario: No valid bound address triggers deployment
- **GIVEN** wallet `W` has no valid private Zap address in Redis or MySQL on `bsc`
- **WHEN** the user opens a new position on `bsc`
- **THEN** the backend deploys a new private Zap contract
- **AND** configures trusted addresses
- **AND** persists the bound address
- **AND** writes the bound address into Redis
