## 1. Implementation
- [x] 1.1 更新 GoldenDog 规格说明，将通知与冷却维度从池子调整为交易对
- [x] 1.2 后端按池子查询活跃仓位后，基于 token 地址归并为交易对并累计各池子的活跃仓位数
- [x] 1.3 将 GoldenDog 去重状态改为按交易对写入，兼容现有状态表
- [x] 1.4 补充单元测试，并执行 `gofmt`、`go test`、`go build` 验证
