# Change: 按交易对聚合金狗通知

## Why
- 同一交易对可能同时存在多个 V3/V4 或不同 fee tier 的池子；当前金狗通知按单个池子触发，会把同一波聪明钱热度拆成多条 Bark。
- 用户更关心某个交易对是否正在被多个监控钱包同时持有 LP 仓位，而不是先按池子地址再自己手动归并。

## What Changes
- 将 GoldenDog Bark 触发条件从“单池子达到阈值”改为“同一交易对达到阈值”。
- 交易对归并键使用同链 `token0/token1` 合约地址，而不是 symbol；这样跨 fee tier、跨协议版本时仍能稳定归并。
- 统计值按交易对下各池子的活跃 LP 仓位数累加；同一钱包如果同时持有同交易对的多个池子，或在同一池子内持有多个仓位，都按多个活跃仓位分别计数。
- 将通知冷却去重从 per-pool 改为 per-pair，避免同一交易对在多个池子上重复推送。
- 继续保留通知正文中的交易对名称与钱包数量描述，不改变用户配置项结构。

## Impact
- Affected specs: `miniapp-smart-money`
- Affected code:
  - `backend/service/smart_money_golden_dog/smart_money_golden_dog_service.go`
  - `backend/service/smart_money_golden_dog/smart_money_golden_dog_service_test.go`
