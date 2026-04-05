## 1. Implementation
- [x] 1.1 Contracts: 新增 `AtomicIncreaseZap.sol`，实现 `zapIncreaseV3` / `zapIncreaseV4`、补仓事件、可选 `entrySwap` + `rebalanceSwap`、dust refund、ownership / NFT approval 校验。
- [x] 1.2 Contracts: 重新编译新的补仓合约，并生成后端嵌入所需的 ABI 与 bytecode 产物。
- [x] 1.3 Config / rollout: 新增 `ATOMIC_ADD_LIQUIDITY_ENABLED` 灰度开关，并为原子补仓建立独立的私有绑定 kind `atomic_increase_zap`。
- [x] 1.4 Blockchain bindings: 新增 `atomic_increase_zap` 的 Go 绑定、部署入口和 `eth_call` 模拟封装。
- [x] 1.5 Liquidity service: 重构 `IncreaseLiquidityForTask`，改为构造原子 funding/swap/increase 参数并发送单笔 Zap 交易，不再先发独立 OKX swap tx。
- [x] 1.6 Validation / hints: 补充 NFT 授权、链上模拟失败和 V4 revert selector 的可读错误与预检查。
- [x] 1.7 Accounting: 调整 `task_add_liquidity` 与交易流水写入逻辑，按原子补仓实际 `amountUsed` / `dust` / `gas` 更新任务和本金记录，并返回主交易哈希。
- [x] 1.8 Verification: 完成 `cd contracts && npm run compile`、`cd backend && go test ./service/liquidity -run TestDoesNotExist` 等编译验证。
