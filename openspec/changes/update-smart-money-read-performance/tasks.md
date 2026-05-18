## 1. Investigation
- [x] 1.1 为 `/api/sm/pools`、`/api/sm/pool_fee_heatmap`、`/api/sm/position_detail` 增加阶段耗时日志，区分 SQL、修复、RPC、元数据加载。
- [ ] 1.2 在线下或本地复现接口耗时，确认主要耗时点与 RPC/SQL 占比。

## 2. Backend
- [x] 2.1 新增聪明钱只读 RPC endpoint 枚举逻辑，从 RPC 池读取同链 HTTP 可用端点，不改变当前交易 RPC。
- [x] 2.2 新增只读 ethclient 缓存和有界并发执行器，支持端点轮询、请求超时和失败统计。
- [x] 2.3 改造聪明钱仓位详情读取，使 V3/V4 position、slot0、feeGrowth/tick 读取走只读执行器。
- [x] 2.4 改造收益火焰图刷新逻辑，接口不再同步等待全量手续费刷新，后台刷新任务去重并使用多 RPC 并发。
- [x] 2.5 将 `RepairPositions` 从池子/火焰图/详情请求阻塞前置路径移出，改为非阻塞触发。
- [x] 2.6 保留现有响应字段，不破坏 MiniApp/WebApp 现有调用。

## 3. Safety
- [x] 3.1 检查核心开仓、撤仓、再平衡、交易发送路径没有引用新增只读执行器。
- [x] 3.2 做针对性 diff 检查，确认没有改动交易执行、nonce、授权、OKX swap、开撤仓状态机逻辑。
- [x] 3.3 增加或更新单元测试覆盖 RPC 池多个可用 endpoint、禁用 endpoint、环境变量回退场景。
- [x] 3.4 运行 `go test ./...`。
