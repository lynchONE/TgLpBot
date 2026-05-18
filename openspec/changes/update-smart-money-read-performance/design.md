## Context
当前慢查询集中在聪明钱展示链路：

- 火焰图请求会调用 `ListActivePositionsForFeeHeatmap` 后同步执行 `refreshSmartMoneyHeatmapFees`，再对需要刷新的每个活跃仓位调用 `Realtime.GetSmartMoneyPositionDetail`。
- 详情接口调用 `Realtime.GetSmartMoneyPositionDetail`，V3 路径会读 position manager、block number、slot0、feeGrowthGlobal、tickLower/tickUpper feeGrowthOutside；V4 路径会读 position info、slot0、feeGrowthGlobal、tickLower/tickUpper feeGrowthOutside。
- 活跃池子列表调用 `repairSmartMoneyPositions`，而 `RepairPositions` 会周期性执行近期 open position state repair，并通过 `loadCurrentLiquiditySnapshot` 读取链上 liquidity。
- 现有 RPC 池支持多个端点，但 `blockchain.GetEVMClient` 对每条链保留一个当前 client。这个语义适合交易执行，不适合高并发展示读取。

## Goals / Non-Goals
- Goals:
  - 降低活跃池子、收益火焰图和仓位详情的用户感知响应时间。
  - 将聪明钱只读查询分散到多个可用 RPC，降低单一 RPC 限速影响。
  - 为线上定位输出阶段耗时日志。
  - 保证开仓、撤仓、再平衡、交易发送链路不受影响。
- Non-Goals:
  - 不改变核心交易执行 RPC 选择策略。
  - 不改变资金操作、nonce、gas、授权、OKX swap、开撤仓状态机。
  - 不引入新的外部 RPC 管理系统。

## Decisions
- Decision: 新增聪明钱只读 RPC 执行器，而不是改造全局 blockchain client。
  - 原因：全局 client 被交易和余额同步等核心路径复用，直接做多 RPC 轮询会改变资金执行行为。
  - 执行器只接受读请求，内部从 RPC 池列出可用 HTTP endpoint，按 endpoint 创建/复用 ethclient，并提供有界并发执行。

- Decision: 火焰图接口返回快照优先，刷新异步化。
  - 原因：火焰图是列表视图，实时性要求低于响应速度。过期手续费快照可以后台刷新，响应保留 `fee_updated_at`、`sample_status`、缺失计数。
  - 接口不得等待全部过期仓位刷新完成；可在后台触发刷新并去重，避免轮询时重复刷同一批。

- Decision: 详情接口保留实时读取，但使用只读多 RPC 并发和短超时。
  - 原因：用户点开详情预期看到最新仓位状态，不能只返回旧快照。
  - 同一仓位内部优先使用同一个 RPC endpoint 和 snapshot block 完成相关读取；当该 endpoint 限速/失败时，可切换到其他只读 endpoint 重试。

- Decision: `RepairPositions` 不再作为池子/详情/火焰图请求的阻塞前置步骤。
  - 原因：状态修复属于后台校准，不能阻塞用户列表查询。
  - 请求链路最多触发非阻塞后台修复信号，并由互斥/频控保证不会堆积。

- Decision: 耗时日志默认仅在请求超过阈值或配置开启时输出。
  - 原因：避免高频轮询产生过多日志，同时保留线上定位能力。

## Risks / Trade-offs
- 多 RPC 并发会增加总 RPC 调用量：通过全局并发上限、每 endpoint 并发上限、请求超时和后台刷新去重控制。
- 多 endpoint 区块高度不一致：详情读取在单仓位内尽量固定 endpoint/snapshot block；火焰图使用快照时间表达数据新鲜度。
- 异步刷新会让火焰图短时间显示旧数据：这是列表性能换实时性的取舍，响应字段必须保留更新时间和样本状态。

## Migration Plan
- 新增只读执行器和配置默认值，默认启用但只作用于聪明钱展示链路。
- 将火焰图同步刷新改为后台刷新，保留现有 DB 字段 `fee_usd`、`fee_status`、`fee_updated_at`。
- 将请求前置 `repairSmartMoneyPositions` 调整为非阻塞触发或后台任务。
- 通过针对性测试覆盖：不触碰开仓/撤仓包；详情和火焰图在无 RPC 池、单 RPC、多 RPC 下均可返回。

## Open Questions
- 线上 RPC 池中是否所有 endpoint 都允许高频 `eth_call`？如果部分 endpoint 质量较差，是否需要在 UI 中标记“仅交易/仅查询”用途。
- 火焰图手续费快照的可接受最大陈旧时间是 15 秒、30 秒还是 60 秒。
