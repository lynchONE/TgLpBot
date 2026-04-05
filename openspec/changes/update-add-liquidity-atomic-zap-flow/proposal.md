# Change: 将补充流动性改为原子 Zap 流程

## Why
- 当前 `task_add_liquidity` 在很多池子上会先单独发送 `stable -> entry token` 换币交易，再执行 V3/V4 补仓。
- 一旦后续 `increaseLiquidity` 或 `modifyLiquidities` 在 `eth_estimateGas` 或链上执行阶段失败，前置 swap 已经落链，钱包会残留 entry token 或 dust，形成“先换币后失败”的不可回滚中间态。
- V4 补仓尤其容易因为链上精确结算和后端离线估算存在偏差而失败，这会直接把两段式链路的资金残留问题放大。

## What Changes
- 新增独立的 `AtomicIncreaseZap.sol`，专门处理“已有 `tokenId` 的补仓”场景，覆盖 V3 和 V4。
- 补仓执行从“先独立 swap、再 direct increase”改为“单笔 Zap 交易内完成全部步骤”：
  - 可选 `stable -> entry token` 前置换币
  - 可选 `token0 <-> token1` 配比换币
  - `increaseLiquidity` / `modifyLiquidities`
  - dust 退款
- 后端 `IncreaseLiquidityForTask` 改为优先构造并模拟新的原子补仓调用，不再先广播独立的 OKX swap 交易。
- 为原子补仓增加独立的私有合约绑定种类 `atomic_increase_zap`，不复用也不污染现有 `zap_simple` 绑定。
- 补仓成功后的任务金额、交易记录和返回结果改成按实际 `amountUsed` / `dust` / `gas` / 主交易哈希更新。

## Impact
- Affected specs:
  - `liquidity-increase`
  - `zap-contracts`
- Affected code:
  - `contracts/contracts/AtomicIncreaseZap.sol`
  - `backend/base/blockchain/atomic_increase_zap.go`
  - `backend/base/blockchain/atomic_increase_zap_private.go`
  - `backend/service/liquidity/liquidity_increase.go`
  - `backend/service/liquidity/liquidity_increase_atomic.go`
  - `backend/service/liquidity/private_atomic_increase_zap.go`
  - `backend/service/web_server/task_add_liquidity.go`
  - `backend/service/trade/trade_record.go`

## Rollout / Backwards Compatibility
- 新增 `ATOMIC_ADD_LIQUIDITY_ENABLED` 灰度开关：
  - 关闭时继续使用现有两段式补仓链路，作为紧急回退方案。
  - 开启时，且 `PRIVATE_ZAP_ENABLED=true` 时，`task_add_liquidity` 优先走原子补仓。
- 本次实现不要求提升 `PRIVATE_ZAP_VERSION`。
  - 原因是原子补仓使用新的私有绑定 kind `atomic_increase_zap`，与原有 `zap_simple` 绑定完全隔离。
- 现有开仓和撤仓逻辑不受影响，仍然沿用原有 Zap 合约和绑定。

## Open Questions
- 暂无。本次变更仅覆盖 `task_add_liquidity`；如果后续需要把开仓也迁移到相同的原子 funding 模型，应另行提案。
