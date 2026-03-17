# Change: 重构聪明钱活跃仓位状态来源

## Why
当前聪明钱钱包列表与详情主要基于 `smart_lp_events` 明细事件推断，再在查询时补充链上 live resolve。这个方案会带来两个核心问题：
- 当 remove 事件漏采、事件 TTL 过短或历史不完整时，前端会出现“列表仍显示钱包，但点开详情发现没有活跃仓位”的空壳行。
- 为了纠正空壳行，查询路径不得不临时打链上 RPC 校验，容易在高并发或大窗口请求下压垮 RPC。

系统需要把“当前是否仍有活跃仓位”从查询时推断，前移为采集层维护的持久化状态。

## What Changes
- 新增 ClickHouse 活跃仓位状态表 `smart_lp_active_positions`，按仓位键持久化当前 `liquidity`、最近加仓时间、最近事件信息与活跃标记。
- SmartLP 采集 add/remove 事件时，同步增量更新 `smart_lp_active_positions`，使活跃状态在入库时就完成收敛。
- `GET /api/smart_money_pool_adds` 与 `GET /api/smart_money_wallet_positions` 改为优先读取 `smart_lp_active_positions`，不再依赖请求时链上 RPC 判断活跃与否。
- 保留后台低频 reconciliation / 修复任务，用于处理漏采、重组或历史状态漂移，但不进入用户请求主链路。
- 调整聪明钱查询口径：`smart_lp_events` 保留为事件明细与回放数据源，`smart_lp_active_positions` 作为当前活跃仓位真源。

## Impact
- 影响规格：
  - `miniapp-smart-money`
  - `analytics-performance`
- 影响代码：
  - `backend/base/clickhouse/*`
  - `backend/service/smart_lp/smart_lp_monitor.go`
  - `backend/service/web_server/smart_money_pool_adds.go`
  - `backend/service/web_server/smart_money_wallet_positions.go`
  - 可能新增 `backend/service/smart_lp/*active*` / `backend/service/web_server/*active*` 辅助逻辑
- 影响数据：
  - 新增 `smart_lp_active_positions` 表
  - 需要从最近窗口内 `smart_lp_events` 回放一次初始 active state
- 风险：
  - 若 remove 漏采，active state 可能长期漂移，因此必须有后台校准机制
