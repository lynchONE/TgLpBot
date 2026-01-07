# Change: Add backend health and readiness endpoints

## Why
Deployments and monitoring need a simple liveness/readiness signal for Docker/Kubernetes health checks and uptime monitoring. Today the backend only exposes feature APIs and a Telegram bot, making it harder to detect dependency outages (MySQL/Redis) and to gate traffic during startup.

## What Changes
- Add `GET /healthz` (liveness) and `GET /readyz` (readiness) to the backend HTTP server.
- `GET /healthz` returns `200` whenever the HTTP server is running.
- `GET /readyz` checks required dependencies (config loaded, MySQL, Redis) and returns `200` only when ready; otherwise `503`.
- Responses are JSON and must not leak secrets (tokens, API keys, encryption keys, private keys).

## Impact
- Affected specs: `specs/service-health/spec.md` (new capability)
- Affected code: `backend/service/web_server/*`, `backend/base/database/*`, optional checks for `backend/base/clickhouse/*` and `backend/base/blockchain/*`
- Backwards compatibility: additive (no breaking changes)
