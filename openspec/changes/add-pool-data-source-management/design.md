## Context
当前 `pool_sync` 通过 `PoolMClient` 使用固定 base URL 抓取 `top-fees/5`，配置来自 `.env`。用户希望增加备用数据源，避免 PoolM 单点异常影响热门池子列表、智能狗池子监控以及开仓流动性参考。

已有 RPC 池管理能力提供了可复用模式：DB 存储候选项、管理员 API 增删切换、空 DB 时回退 env 配置。池子数据源可以采用类似模型，但需要额外处理不同上游的响应字段命名和 v4 poolId 语义。

## Goals / Non-Goals
- Goals:
  - 支持管理员无需重启服务即可新增、切换和禁用池子同步数据源。
  - 支持 PoolM 与备用 `market/pools` 两类来源。
  - 保留现有 env/default PoolM 兜底行为。
  - 同步链路统一输出现有 `PoolMTopFeesResponse`/`PoolMFeePool` 等价的内部快照。
- Non-Goals:
  - 不在本次改造 `/api/search_pools` 的 DexScreener 搜索链路。
  - 不把多个来源的数据做合并去重；同一时刻只使用一个当前来源，失败时最多切到一个备用来源。
  - 不把趋势图作为必填能力；`metricTrends`、`liquidityTicks` 可以为空。

## Decisions
- Decision: 新增 `pool_data_sources` 表管理上游来源。
  - 字段建议：`id`、`name`、`source_type`、`base_url`、`path_template`、`query_template_json`、`chain`、`timeframe_minutes`、`protocols_json`、`dexes_json`、`is_current`、`is_enabled`、`last_checked_at`、`last_success_at`、`last_latency_ms`、`last_error`、`created_at`、`updated_at`。
  - `source_type` 初始支持 `poolm_top_fees` 与 `market_pools`。
  - 通过代码事务保证同一 `chain + timeframe_minutes` 下只有一个 `is_current=true`。

- Decision: `pool_sync` 通过数据源管理器解析当前来源，再交给适配器拉取。
  - `poolm_top_fees` 适配器继续请求 `/api/pools/top-fees/{minutes}?chain=...&dex=...`。
  - `market_pools` 适配器请求 `/api/market/pools`，并按配置填入 `timeframe=5m`、`limit`、`protocol`、`dex` 等参数。
  - 两类适配器最终都输出统一内部快照，后续 `buildRows` 不关心上游类型。

- Decision: 字段解析同时兼容 snake_case 与 camelCase。
  - PoolM 当前代码使用 `pool_address`、`fee_rate`、`current_pool_value` 等 snake_case。
  - 备用接口样本使用 `poolAddress`、`feeRate`、`currentPoolValue` 等 camelCase。
  - 适配层负责归一化，避免业务层散落两套字段名。

- Decision: v4 备用来源使用 `poolId` 作为池子主标识。
  - 当 `protocolVersion=v4` 且 `poolAddress` 为空时，系统 SHALL 使用 `poolId` 作为 `models.Pool.ID`、`Address` 和读接口返回的 `pool_address`。
  - `poolManager` 需要单独保存或保留在 `source_payload_json`，避免后续 v4 链上查询丢上下文。

- Decision: 数据源失败处理采用显式切换优先，自动回退谨慎启用。
  - 默认：当前来源失败只记录错误并保留现有数据，不清空 `pools`。
  - 可选配置：允许同步任务在当前来源失败时尝试下一个 enabled 来源；成功后是否自动设为 current 由配置控制。
  - 管理员手动切换仍是主路径，避免短暂波动导致来源频繁切换。

## Admin API
新增管理员接口，沿用现有 `/api/admin` 兼容路由模式：
- `POST /api/admin/pool_data_sources`
- `POST /api/admin?endpoint=pool_data_sources`

请求字段建议：
- `action`: `list` | `add` | `update` | `switch` | `enable` | `disable` | `delete` | `check`
- `source_id`
- `name`
- `source_type`
- `base_url`
- `path_template`
- `query_template`
- `chain`
- `timeframe_minutes`
- `protocols`
- `dexes`

所有操作必须校验 Telegram WebApp `initData` 且要求管理员权限。

## MiniApp UI
在管理员页新增“池子源”页签：
- 展示当前来源、env 兜底来源、所有 DB 来源。
- 展示来源类型、base URL、链、窗口、协议、DEX、最后成功时间、最后错误。
- 支持新增来源、手动切换、启用/禁用、删除、连通性检查。
- URL 可按 RPC 池类似方式做脱敏展示；新增时由管理员提交完整 URL。

## Risks / Trade-offs
- 风险：备用来源缺少 `activeLiquidityUSD` 或 `currentPoolValue` 时，部分 v4 池子会被列表过滤或无法通过“活跃费率”筛选。
  - Mitigation: 管理页连通性检查返回字段覆盖情况，提示关键字段缺失；同步时保留缺失字段为 0/null，不伪造数据。
- 风险：两个来源对 DEX 名称和协议参数不一致。
  - Mitigation: 数据源记录保存独立的协议/DEX 参数模板，不复用 PoolM 的 `pcsv3/univ3/univ4` 假设。
- 风险：自动切换可能掩盖上游异常。
  - Mitigation: 自动回退默认只用于本次同步尝试，是否持久切换 current 由显式配置或管理员操作决定。

## Migration Plan
1. 新增 `pool_data_sources` 模型和 AutoMigrate。
2. 启动时如果 DB 没有来源，运行时继续用 env/default PoolM，不强制写入默认行。
3. 管理员可手动新增当前 PoolM 来源和备用 `market_pools` 来源。
4. 切换数据源后下一轮 `pool_sync` 使用新来源；已有 `pools` 数据按正常 upsert 覆盖。

## Open Questions
- 是否需要把“当前来源失败时自动持久切换到备用来源”作为默认行为，还是只记录错误并等待管理员手动切换？
- 备用 `market/pools` 的 `limit` 默认值是否应该固定为当前同步上限，还是允许管理员配置？
