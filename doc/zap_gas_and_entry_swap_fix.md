# 非 USDT 池子开仓修复（2026-03-30）

## 1. Entry Swap Dust 余额绕过修复

**问题**：在 SIREN/WBNB 等非 USDT 池子开仓时，钱包中极少量的残余 WBNB（dust）会导致系统跳过前置兑换预览步骤，并用极小额度开仓，最终 `execution reverted`。

**修复文件**：
- `backend/service/liquidity/entry_swap_preview.go`
- `backend/service/liquidity/liquidity_enter.go`

**修复方案**：将跳过前置兑换的条件从 `use.Sign() > 0`（有任意余额即可）改为**需达到预算的 95% 以上**。

## 2. zapInV3 Gas 估算失败导致开仓中止

**问题**：`tuneZapTxGasLimit` 中 `EstimateGas` 模拟失败且未配置 `ZapGasLimitMin` 时，代码不设置 `auth.GasLimit`，导致后续 `zap.ZapInV3` 调用也失败，整个操作中止。

**修复文件**：
- `backend/service/liquidity/zap_gas_limit.go`

**修复方案**：当 EstimateGas 失败且无 minLimit 配置时，设置安全默认值 `8,000,000` gas 兜底。

## 3. OKX Swap Zap 内部 native value 路由校验

**问题**：`prepareOKXSwapParams` 缺少 OKX `tx.value` 非零校验。OKX 可能返回需要 native BNB 的 swap 路由，但 Zap 合约内部无法提供 `msg.value`，导致合约 revert。

**修复文件**：
- `backend/service/liquidity/liquidity_enter.go` (`prepareOKXSwapParams`)

**修复方案**：增加 `tx.value != 0` 检查，拒绝需要原生代币的路由。
