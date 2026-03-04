# Design: RPC endpoint pool (bsc/base, http/ws)

## Goals
- No-restart RPC rotation: admins can add/switch endpoints at runtime.
- Visibility: admin UI shows the current endpoint and health/status of all endpoints.
- Safety: if an endpoint is quota-exhausted for the month, avoid using it until the next month.
- Backward compatibility: if the DB pool is empty, keep using `.env` RPC settings.

## Data Model
Table: `rpc_endpoints`

Minimal fields:
- `id`
- `chain`: `bsc` | `base`
- `transport`: `http` | `ws`
- `url`
- `is_current`: whether this endpoint is currently selected for its `chain+transport`
- `disabled_until` (nullable): if set and `now < disabled_until`, the endpoint is unavailable
- `disabled_reason` (string): e.g. `quota_exhausted`, `manual`, `health_fail`
- Health metadata:
  - `last_checked_at`, `last_success_at`
  - `last_latency_ms`
  - `last_error` (truncated)

Notes:
- Enforce uniqueness at least on `(chain, transport, url)` to avoid duplicates.
- Enforce “single current per chain+transport” in code via transaction updates.

## Status / Availability
Computed status for UI and selection:
- `unavailable`: `disabled_until != nil && now.Before(*disabled_until)`
- `available`: otherwise (with optional `last_error`/`last_checked_at` for operational context)

## Monthly quota exhaustion
Heuristic classification based on RPC error strings/codes (provider-dependent).

When detected:
- Set `disabled_reason=quota_exhausted`
- Set `disabled_until = first day of next month 00:00` in `timeutil.Location()` (Asia/Shanghai)
- If the endpoint was current, immediately switch to another available endpoint

Manual override:
- Admin switch MUST NOT allow switching to an endpoint that is currently unavailable.

## Failover / Switching
Selection algorithm (per `chain+transport`):
1. Prefer `is_current=true` endpoint if it is available.
2. Else pick the first available endpoint in deterministic order (e.g. lowest `id`), set it as current.
3. If none available, fall back to `.env` URL (if configured); otherwise return a clear error.

Health check:
- Background ticker (e.g. 30–60s) runs a lightweight probe (e.g. `eth_blockNumber`) with a short timeout.
- On repeated failures for the current endpoint, mark temporarily disabled (short TTL) and switch.

Client caching:
- Maintain a per `chain+transport` cached dialed client where appropriate.
- On switch, close old client and dial the new one lazily on next use.

## Backend APIs (admin-only)
Proposed endpoint: `POST /api/admin/rpc_pool` (and compat route `endpoint=rpc_pool`)

Operations:
- `list`: return pool state grouped by chain+transport, including current endpoint and per-endpoint status
- `add`: create an endpoint
- `switch`: set an endpoint as current for its chain+transport
- `disable`: mark endpoint unavailable until a given time (used for `quota_exhausted`)

All operations require Telegram WebApp `initData` verification and admin authorization.

## Mini App UI
Admin tab “RPC”:
- Shows current endpoint for `bsc/base` × `http/ws`
- Shows all endpoints with status badges:
  - current / available / unavailable (and `disabled_until`)
  - last check + last error snippet
- Actions:
  - Add new endpoint (chain + transport + url)
  - Switch current endpoint (only to available endpoints)
  - (Optional) Mark as quota-exhausted (manual disable-until-next-month)

Sensitive data:
- Mask URLs by default in UI (e.g., keep domain + last 4 chars), with an explicit “copy full” action.

