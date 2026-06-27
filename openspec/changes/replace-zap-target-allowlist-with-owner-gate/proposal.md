# Change: Replace Zap Target Allowlist With Owner Gate

## Why
当前私有 Zap 已经做到每个钱包单独部署合约，但 Zap 执行外部兑换时仍依赖 OKX/Binance 的 swap target 和 approve target 白名单。为了减少新增兑换渠道时的配置成本，后续改为以“只有部署钱包能调用私有 Zap”为主要链上边界，不再维护聚合器目标地址白名单。

## What Changes
- Zap 合约资金入口新增 owner-only 调用限制，只有部署该 Zap 的钱包可以调用。
- Zap 执行外部 swap 时不再检查 `trustedSwapTargets` / `trustedApproveTargets`。
- 删除 Zap 的 swap/approve target 白名单配置入口；部署和更新脚本只配置 V3/V4 Position Manager 与 wrapped native。
- 后端移除 Binance target allowlist 配置项，私有 Zap 部署不再读取 OKX/Binance target 配置。
- 私有 Zap 版本提升，已有钱包下次使用时重新部署新版私有 Zap。

## Impact
- Affected specs: `zap-contracts`
- Affected code: `contracts/contracts/ZapSimple.sol`, `contracts/contracts/AtomicIncreaseZap.sol`, `contracts/scripts/*`, `backend/base/config/config.go`, `backend/service/liquidity/private_zap.go`
- Security: 外部聚合器返回的 `tx.to` / spender 将被 Zap 直接使用；链上限制从目标地址白名单变为调用者必须是私有 Zap owner。
