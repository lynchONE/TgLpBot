## 1. Implementation
- [x] 1.1 Config: Add multi-chain `ChainConfig` keyed by chain slug; keep legacy BSC env backward-compatible; Base stable uses USDT.
- [x] 1.2 DB: Add `chain` column/index to `strategy_tasks`/`trade_records`/`transactions` (and `positions` if used); ensure CRUD uses `chain` (default `bsc`).
- [x] 1.3 Blockchain: Implement `InitBlockchains()` to init and maintain `map[chain]*ethclient.Client` + `map[chain]*big.Int`; remove single-chain assumptions.
- [x] 1.4 Executor: Introduce `ChainExecutor` interface + EVM implementation; dispatch by `task.Chain` (reserve for Solana executor later).
- [x] 1.5 API: Add / pass-through `chain` for endpoints related to open/close/query tasks and tx links; default to `bsc` when omitted.
- [x] 1.6 PoolService: Identify V3 exchange by `factory()` using per-chain deployments; add default V3 PositionManager fallback.
- [x] 1.7 Liquidity Enter: Convert `AmountUSDT` (float) to stable token minimal units using decimals (no fixed `1e18`); scale records to internal `USD(1e18)`.
- [x] 1.8 Liquidity Exit: Parse OKX quote/swap `toTokenAmount` by stable decimals; chain-scoped client for dust/leftover calc and wallet sync.
- [x] 1.9 OKX Swap: Use per-chain `chainId` + allowlist (router/tokenApprove); keep Permit2 spender branch consistent.
- [x] 1.10 Gas/PnL: Replace `GetBNBPriceUSDT` usage with `GetNativePriceUSD(chain)` (BSC=BNB, Base=ETH); compute gas->USD by chain.
- [x] 1.11 Bot/MiniApp: Add chain selection to task creation entry; render explorer links via chain explorer template.
- [x] 1.12 Contracts: Add Base network(s) to Hardhat; de-BSC-ify deploy/verify scripts; document Base deploy steps.
- [x] 1.13 Verification: `cd backend; go test ./...` + `cd contracts; npm run compile` + `cd miniapp; npm run build` + `openspec validate add-base-chain-support --strict`.

