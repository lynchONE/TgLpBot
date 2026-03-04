# Change: Add Mini App RPC endpoint pool management

## Why
RPC endpoints are currently configured via `.env`. When an endpoint runs out of quota or becomes unstable, we must edit `.env` and restart the backend process, which is slow and operationally risky.

We need an admin UI in the Mini App to:
- Add multiple RPC endpoints
- See which endpoint is currently used and the status of all endpoints
- Automatically fail over when the current endpoint becomes unavailable
- Allow admins to manually switch the active endpoint

This must work for:
- Chains: `bsc`, `base`
- Transports: `http` and `websocket`

## What Changes
- Add a DB-backed RPC endpoint pool (per `chain` + `transport`) with availability/status tracking.
- Add admin-only backend APIs to:
  - List endpoints + current selection + status
  - Add endpoints
  - Mark endpoints unavailable (e.g., monthly quota exhausted)
  - Switch the active endpoint (manual override)
- Add backend auto-failover:
  - If the active endpoint becomes unavailable, switch to another available endpoint in the pool.
  - If an endpoint is detected as “monthly quota exhausted”, mark it unavailable until next month.
- Extend the Mini App admin page with an “RPC” management tab to view status, add endpoints, and switch.
- Keep backward compatibility: when the DB pool is empty, continue using `.env` RPC settings.

## Impact
- Affected specs: `specs/rpc-pool/spec.md` (new capability)
- Affected code (expected):
  - Backend: `backend/base/models/*`, `backend/base/blockchain/*`, `backend/service/web_server/*`
  - Mini App: `miniapp/src/components/*`, `miniapp/src/lib/api.js`
  - Vercel proxy: `miniapp/api/admin.js`
- Security: RPC URLs may contain API keys. APIs and logs must avoid leaking sensitive data; UI should mask secrets by default.
- Backwards compatibility: additive, with safe fallback to current `.env` behavior.

