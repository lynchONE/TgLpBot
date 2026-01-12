## 1. Implementation
- [ ] 1.1 定义 Flash Pump/Dump Guard 配置结构（系统配置/环境变量回退）
- [ ] 1.2 V3：增加链上 TWAP 读取（observe）并计算 spotOverTwapPct
- [ ] 1.3 AutoLP 开仓前增加门禁：命中则延迟确认或跳过，并输出可读原因
- [ ] 1.4 实现“冲顶回落”判定与长冷却（池子级别）
- [ ] 1.5 V4/Fallback：无 TWAP 时用两次扫描脉冲判定（可选）
- [ ] 1.6 验证：运行 `cd backend; go test ./...`
