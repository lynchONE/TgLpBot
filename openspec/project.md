# Project Context

## Purpose
TgLpBot is a Telegram bot + web dashboard for managing concentrated-liquidity (CLMM) positions on Binance Smart Chain (BSC), covering V3 pool address / V4 PoolId flows.

Primary goals:
- Let users create/import wallets and manage them securely (encrypted private key storage)
- Enter/exit V3/V4 LP positions from USDT, with swaps routed via the OKX DEX aggregator
- Provide realtime monitoring and optional strategy automation (AutoLP / SmartLP) with a Mini App UI for visibility

## Tech Stack
- Backend: Go (module `TgLpBot`, `backend/`), `net/http`, `github.com/go-telegram-bot-api/telegram-bot-api/v5`, `github.com/ethereum/go-ethereum`, `gorm.io/gorm` + MySQL, `github.com/go-redis/redis/v8`, optional `github.com/ClickHouse/clickhouse-go/v2`
- Frontend (Mini App): React 18 + Vite + TailwindCSS (`miniapp/`), `lightweight-charts`
- Smart contracts: Solidity (0.8.26) + Hardhat + Ethers v6 (`contracts/`), OpenZeppelin contracts, Uniswap v4 core/periphery
- Infra: Docker/Docker Compose (`backend/docker-compose.yml`); Mini App deploy target is Vercel (`miniapp/vercel.json`)

## Project Conventions

### Code Style
- Go: use `gofmt`; prefer explicit errors (`fmt.Errorf("...: %w", err)`); use contexts/timeouts for outbound I/O; avoid logging secrets (bot token, private keys, API keys).
- Project structure conventions:
  - `backend/base/`: infrastructure/shared utilities (config, db, blockchain bindings, security, concurrency)
  - `backend/service/`: business logic by feature (bot handlers, liquidity, pricing, strategy, web_server)
  - snake_case package dirs are used for feature groupings (e.g. `auto_lp`, `smart_lp`).
- Mini App: plain JS/JSX (no TypeScript currently); React function components + Tailwind utility classes; keep UI changes in `miniapp/src/`.
- Contracts: Hardhat JS scripts + Solidity; keep chain addresses out of git (use `contracts/.env`).

### Architecture Patterns
- Monorepo with three primary deliverables:
  - `backend/`: single Go process that starts the Telegram bot and an HTTP API server (default `:8080`).
  - `miniapp/`: Telegram Mini App UI that calls the backend API directly (or via Vercel function proxy).
  - `contracts/`: on-chain zap helper used by the backend to make swap+mint atomic.
- Data stores:
  - MySQL: source of truth for users, wallets, tasks/strategies, trades/transactions.
  - Redis: caching/coordination and realtime task state.
  - ClickHouse (optional): pool analytics/history; when unavailable, some `/api/*` endpoints degrade gracefully.
- AuthN/AuthZ:
  - Web API uses Telegram WebApp `initData` verification (HMAC) to authenticate the caller.
  - Admin access is derived from configured admin wallet address and/or access grants/codes.

### Testing Strategy
- Backend: `cd backend; go test ./...` (unit tests preferred; avoid real mainnet RPC in tests unless explicitly gated).
- Contracts: `cd contracts; npm test` / `npm run compile` (requires appropriate RPC keys when running against live networks).
- Mini App: no formal test harness; use `npm run build` + manual QA in Telegram WebApp.

### Git Workflow
- Prefer short-lived feature branches and PRs.
- Keep commits small and scoped; include clear commit messages (no strict convention enforced yet).
- Never commit secrets: `.env`, private keys, API keys, or production URLs that include tokens.

## Domain Context
- Chain: BSC (chainId 56). Primary user-facing asset for workflows is USDT; other tokens are swapped as needed.
- CLMM basics: V3/V4 positions are range-bound liquidity defined by ticks; PnL depends on price movement, fees earned, and gas.
- “Zap” flows: entering positions often requires swapping USDT into token0/token1 and minting in one transaction via a helper contract; dust should be returned to the wallet.
- External signals:
  - AutoLP: evaluates pools/candidates and may auto-open positions when enabled.
  - SmartLP: monitors external contract activity / LP events for strategy triggers.

## Important Constraints
- Financial risk: changes can affect real funds; avoid breaking changes and ensure safety defaults (no auto-trading unless explicitly enabled).
- Security: private keys must remain encrypted at rest (AES-256 via `ENCRYPTION_KEY`); never log or persist plaintext keys.
- RPC unreliability: public RPC endpoints can lag; code uses retries/timeouts; avoid assuming state is immediately consistent.
- Deployment safety: ClickHouse is optional and should not block boot unless explicitly required.

## External Dependencies
- Telegram Bot API + Telegram WebApp initData verification (Mini App auth).
- BSC RPC endpoint(s) (HTTP and optional WS).
- OKX DEX aggregator API (builds swap calldata and routes swaps).
- GeckoTerminal API (pool stats) and PoolM API (AutoLP scanning), when enabled.
- MySQL + Redis (required), ClickHouse (optional).
- Vercel (optional) for Mini App hosting and serverless proxy functions.
