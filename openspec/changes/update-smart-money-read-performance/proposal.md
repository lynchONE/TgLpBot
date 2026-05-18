# Change: 优化聪明钱只读视图查询性能

## Why
聪明钱「池子视图」中的活跃池子、收益火焰图以及仓位详情当前存在明显慢查询。初步定位显示：

- `GET /api/sm/pool_fee_heatmap` 在请求链路中同步刷新所有过期活跃仓位的手续费快照，每个仓位会触发多次链上 RPC 读取，接口必须等待刷新完成后才返回。
- `GET /api/sm/position_detail` 查看详情时会实时读取 V3/V4 仓位、slot0、feeGrowth/tick 信息，当前主要使用全局当前 RPC，遇到单点限速或延迟会直接拖慢详情加载。
- `GET /api/sm/pools` 活跃池子列表本身主要是 SQL 聚合，但请求开始时会调用 `RepairPositions`，该函数会在请求链路中做近期仓位状态修复并读取链上 liquidity，导致活跃池子列表也可能被 RPC 拖慢。

这些慢点都属于聪明钱展示/分析只读链路，不应影响核心开仓、撤仓、再平衡等资金执行逻辑。

## What Changes
- 新增聪明钱只读 RPC 并发执行能力：从现有 RPC 池读取同链 HTTP 可用端点，为只读查询创建独立客户端集合，并按仓位/任务分散到多个 RPC 并发执行。
- 优化收益火焰图手续费刷新：接口优先返回已有快照，过期快照刷新改为独立后台任务或有严格超时的异步刷新，不再让用户请求等待全量 RPC 刷新完成。
- 优化仓位详情加载：详情中的 V3/V4 position、slot0、feeGrowth/tick 等只读 RPC 调用使用聪明钱专用并发读执行器，并限制单请求并发度和超时。
- 将 `RepairPositions` 从高频用户请求主链路移出或加轻量触发保护，避免活跃池子列表被链上校准拖慢。
- 增加低成本耗时日志/指标，拆分输出 SQL 聚合、RPC 刷新、详情链上读取、元数据加载等阶段耗时，便于线上确认瓶颈。
- 明确隔离边界：不修改核心开仓、撤仓、交易发送、nonce、gas、授权、余额同步等执行路径；不改变全局 `blockchain.GetEVMClient` 的交易使用语义。

## Impact
- Affected specs: `miniapp-smart-money`, `analytics-performance`, `rpc-pool`
- Affected code:
  - `backend/service/web_server/smart_money.go`
  - `backend/service/realtime/smart_money_detail.go`
  - `backend/service/realtime/realtime_positions.go`
  - `backend/service/smart_money/position_metadata.go`
  - `backend/service/smart_money/repository.go`
  - `backend/base/rpcpool/*`
  - 可能新增 `backend/service/smart_money/read_rpc_*`
- Risk:
  - 并发 RPC 读取可能放大总请求量，需要默认并发上限、单请求超时和端点级限流。
  - 多 RPC 返回的区块高度可能不同，详情和手续费计算需要使用单仓位内一致的 snapshot block 或接受只读展示级别的轻微延迟。
  - 只读缓存/异步刷新可能让火焰图短时间展示旧快照，响应中需要保留更新时间/样本状态。
