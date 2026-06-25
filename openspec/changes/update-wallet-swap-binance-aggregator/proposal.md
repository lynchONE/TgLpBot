# Change: 钱包兑换改用 Binance 聚合报价

## Why
- 当前钱包兑换能力曾规划/接入 0x 与 LI.FI 多 provider 报价，增加了执行目标校验、手续费解释和前端选择复杂度。
- 现在需要去掉 0x 与 LI.FI 报价兑换渠道，改为接入 Binance Web3 Wallet Trading API 的聚合报价与构造交易能力。
- Binance 聚合报价会返回多个路由，MiniApp 与 WebApp 都需要展示这些路由，并按用户选定的路由执行兑换。

## What Changes
- 后端移除钱包单币兑换链路中的 0x 与 LI.FI 报价/执行渠道，不再向客户端返回这两个 provider。
- 后端新增 Binance Trading API 适配器：
  - `Get Aggregated Quote` 用于获取聚合报价和多路由结果。
  - `Build Swap Transaction` 用于基于选定路由构造可执行兑换交易。
- 钱包单币兑换报价响应改为展示 Binance 聚合方案中的多个 route，而不是多个外部 provider。
- 钱包单币兑换执行接口支持提交选定 Binance route，并在执行前重新构造交易数据。
- MiniApp 与 WebApp 同步展示 Binance 多路由报价、路由明细、最少到账、Gas 和执行结果。

## Impact
- Affected specs: `wallet-swap`
- Affected code:
  - `backend/base/config/config.go`
  - `backend/service/exchange/`
  - `backend/service/liquidity/provider_swap.go`
  - `backend/service/web_server/wallet_swap_single_api.go`
  - `backend/service/web_server/wallet_swap_single_provider_helpers.go`
  - `miniapp/src/lib/api/walletSwap.js`
  - `miniapp/src/components/SwapModule.jsx`
  - `miniapp/src/components/swap/SwapQuoteDetails.jsx`
  - `webapp/src/api.js`
  - `webapp/src/components/SwapPanel.jsx`
