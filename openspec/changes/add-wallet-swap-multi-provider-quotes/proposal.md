# Change: 钱包一键兑换支持多提供商报价与执行

## Why
- 当前 `webapp` 的一键兑换仅接入 OKX，用户无法在同一页面比较 0x、LI.FI 与 OKX 的报价与 Gas。
- 当前页面只展示单一 provider 的结果，未按不同 provider 的手续费规则给出可直接比较的净到手金额。
- 当前单币兑换执行接口没有显式 provider 选择能力，前端无法把“看到的报价”和“最终执行的 provider”绑定起来。

## What Changes
- 后端为钱包单币兑换新增多提供商统一报价聚合，支持同时返回 OKX、0x、LI.FI 的报价结果。
- 统一报价响应补充 `provider`、净到手金额、最小到账、手续费明细/规则、Gas、可执行状态，并仅对允许展示路径的 provider 展示交易路径。
- 单币兑换执行接口支持显式指定 provider，并按所选 provider 重新获取可执行交易数据后发起交易。
- `webapp` 的 `SwapPanel` 改为展示多个 provider 报价卡片，支持按净到手排序并选择指定 provider 执行；页面仅展示 OKX 路径，不展示 0x 与 LI.FI 的兑换路径。

## Impact
- Affected specs: `wallet-swap`
- Affected code:
  - `backend/base/config/config.go`
  - `backend/service/exchange/`
  - `backend/service/liquidity/`
  - `backend/service/web_server/wallet_swap_single_api.go`
  - `webapp/src/api.js`
  - `webapp/src/components/SwapPanel.jsx`
