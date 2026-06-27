# Change: 开仓撤仓兑换支持可选 Provider 与 Binance Zap

## Why
当前钱包一键兑换已经接入 Binance 聚合报价，但开仓/撤仓业务中的兑换和 Zap 内部配比 swap 仍主要依赖 OKX。用户希望开仓和撤仓都能支持 OKX 与 Binance，默认自动择优，也能在开仓页面指定只走 OKX 或只走 Binance，并在执行时清楚提示本次走了哪个渠道、哪个路由。

该变更会影响真实资金路径、Zap 合约外部调用安全边界、WebApp 与 MiniApp 开仓交互，以及撤仓/部分撤仓结果展示，因此需要先明确规格和实施边界。

## What Changes
- Zap 合约必须支持 Binance swap target 与 approve target，同时继续保留严格 allowlist 校验，不允许任意外部调用。
- 开仓页面新增兑换渠道选择：
  - `自动择优`：默认，同时查询 OKX 与 Binance，在允许集合内选择最优可执行报价。
  - `仅 OKX`：只查询并执行 OKX，失败时不自动切到 Binance。
  - `仅 Binance`：只查询并执行 Binance，失败时不自动切到 OKX。
- 开仓相关所有兑换都遵循该策略：钱包侧 entry swap、Zap 内部配比 swap、后续补仓 Zap swap（如适用）。
- 撤仓、部分撤仓、清仓 dust 换回稳定币时，使用任务保存的兑换策略；执行结果必须提示实际 provider 与 route。
- WebApp 与 MiniApp 都要在开仓预览、确认、执行结果/进度反馈中展示实际或预计使用的 provider 与 route。
- 后端日志、交易结果和错误信息记录 provider、quoteId/routeId、route summary、expectedOut、actualOut、txHash。

## Impact
- Affected specs: `position-swap-routing`, `open-position-safety`, `position-execution-performance`, `zap-contracts`, `webapp-open-position`, `miniapp-open-position`
- Affected code:
  - `contracts/contracts/ZapSimple.sol`
  - `contracts/contracts/AtomicIncreaseZap.sol`
  - `contracts/scripts/*`
  - `backend/base/config/*`
  - `backend/base/models/*`
  - `backend/service/liquidity/entry_swap_preview.go`
  - `backend/service/liquidity/liquidity_enter.go`
  - `backend/service/liquidity/liquidity_increase_atomic.go`
  - `backend/service/liquidity/liquidity_exit.go`
  - `backend/service/liquidity/provider_swap.go`
  - `backend/service/exchange/binance_swap.go`
  - `backend/service/web_server/open_position*.go`
  - `backend/service/web_server/task_stop.go`
  - `backend/service/web_server/task_withdraw_liquidity.go`
  - `webapp/src/components/OpenPositionModal.jsx`
  - `webapp/src/api.js`
  - `miniapp/src/features/openPosition/*`
  - `miniapp/src/lib/api/openPosition.js`

