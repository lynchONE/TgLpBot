## ADDED Requirements

### Requirement: Liveness Endpoint
The backend HTTP server MUST expose an unauthenticated liveness endpoint at `GET /healthz`.

#### Scenario: Liveness check succeeds
- **WHEN** the backend HTTP server is running
- **THEN** `GET /healthz` returns HTTP `200`

### Requirement: Readiness Endpoint
The backend HTTP server MUST expose an unauthenticated readiness endpoint at `GET /readyz`.

The readiness check MUST be based on required dependencies:
- Configuration is loaded
- MySQL is reachable
- Redis is reachable

The endpoint MUST return HTTP `200` only when required dependencies are ready; otherwise it MUST return HTTP `503`.

#### Scenario: Ready when MySQL and Redis are available
- **WHEN** configuration is loaded and MySQL and Redis are reachable
- **THEN** `GET /readyz` returns HTTP `200`

#### Scenario: Not ready when MySQL is unavailable
- **WHEN** MySQL is not reachable
- **THEN** `GET /readyz` returns HTTP `503`

#### Scenario: Not ready when Redis is unavailable
- **WHEN** Redis is not reachable
- **THEN** `GET /readyz` returns HTTP `503`

### Requirement: Health Payload Safety
The health endpoints MUST return `application/json` responses.

The responses MUST NOT include secrets (Telegram bot token, OKX API keys, encryption keys, private keys, or equivalent sensitive configuration).

#### Scenario: Responses are JSON
- **WHEN** `GET /healthz` or `GET /readyz` is called
- **THEN** the response includes `Content-Type: application/json`

#### Scenario: Responses do not leak secrets
- **WHEN** `GET /healthz` or `GET /readyz` is called
- **THEN** the response body does not include configured secrets
