## 1. Implementation
- [x] 1.1 更新 ZapSimple，资金入口只允许 owner 调用，并移除 swap/approve target 执行校验。
- [x] 1.2 更新 AtomicIncreaseZap，资金入口只允许 owner 调用，并移除 swap/approve target 执行校验。
- [x] 1.3 提升私有 Zap 绑定版本，触发钱包重新部署新版合约。
- [x] 1.4 更新/补充合约测试，覆盖非 owner 不能调用、任意 target 不再被 allowlist 阻断。
- [x] 1.5 清理部署脚本和后端配置中的 Binance target allowlist，以及 Zap 配置流程里的 OKX target 读取。
- [x] 1.6 运行针对性编译/测试并检查 diff。
