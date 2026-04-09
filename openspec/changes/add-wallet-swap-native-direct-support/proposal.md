# Change: 支持钱包一键兑换原生 BNB/ETH 直兑

## Why
- 当前 `webapp` 一键兑换会把原生 `BNB/ETH` 标记为不可兑换，但 OKX DEX 本身支持 EVM 原生币作为输入或输出。
- 这会让用户必须先手动换成 `WBNB/WETH`，与页面的“一键兑换”目标不一致，也增加了不必要的操作成本。

## What Changes
- 放开钱包余额预览中的原生币可兑换状态。
- 让钱包单币兑换的报价与执行链路支持 `0xeeee...` 原生币伪地址。
- 在 OKX 执行器中支持原生币 `tx.value`、跳过 ERC20 approve，并对原生币到账使用 native balance 口径校验。
- 前端常用代币区域增加原生 `BNB/ETH` 入口。

## Impact
- Affected specs: `wallet-swap`
- Affected code:
  - `backend/service/web_server/wallet_swap_api.go`
  - `backend/service/web_server/wallet_swap_single_api.go`
  - `backend/service/liquidity/okx_swap.go`
  - `backend/service/liquidity/liquidity_wallet_swap.go`
  - `webapp/src/components/SwapPanel.jsx`
