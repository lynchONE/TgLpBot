# Change: 重构聪明钱仓位详情读模型

## Why
- 现有几份聪明钱仓位相关提案依赖 ClickHouse，或者把“是否活跃 / 仓位详情”放在请求时链上 resolve，但项目当前已经移除了 ClickHouse，这些方案不再适配当前代码库。
- 用户需要在 WebApp 和 MiniApp 中点开聪明钱仓位后，像查看自己的实时仓位一样看到详情卡片，并能持续刷新。
- 如果每次刷新都逐条从链上读取仓位、slot0、手续费，会在高频轮询下浪费 RPC，放大接口延迟与不稳定性。

## What Changes
- Backend / 数据层：
  - 新增 MySQL 持久化读模型 `sm_lp_active_positions`，在聪明钱 add/remove 事件入库时同步维护当前活跃仓位。
  - 在 active position 中持久化仓位详情查询所需的核心字段：钱包、协议、池子、tokenId、tick 区间、token 元数据、tickSpacing、当前 liquidity、开仓金额快照、净投入快照、最近手续费快照与状态等。
  - 新增统一的 `position_ref` 概念，列表接口返回引用，详情接口按引用读取活跃仓位读模型。
  - 查询阶段以 `sm_lp_active_positions` 为入口，直接拿到池子、token、tick 区间、manager 引用等元数据；链上只读取真正需要实时性的字段，例如 current tick / sqrtPrice、必要的 live position / claimable fee 数据，避免每次先通过 RPC 反查池子和 token 相关信息。
  - 对高成本动态字段增加缓存 / 限流 / 降级策略；当链上读取失败时，可回退到最近一次成功快照并显式返回状态。
- WebApp：
  - 聪明钱列表中的仓位行支持点开详情。
  - 详情展示方式对齐 WebApp 现有仓位卡片，并按后端返回的 `poll_interval_sec` 自动刷新。
- MiniApp：
  - 聪明钱列表中的仓位行支持点开详情。
  - 详情展示方式对齐 MiniApp 实时 `PositionCard`，并按后端返回的 `poll_interval_sec` 自动刷新。
- 兼容与迁移：
  - 保留 `sm_lp_events` 作为事件明细与回放来源，`sm_lp_positions` 作为历史开平仓记录，`sm_lp_active_positions` 作为当前仓位详情真源。
  - 为历史数据提供回放初始化 / 补算逻辑，使新表能从现有 MySQL 事件数据中恢复。

## Impact
- Affected specs:
  - `miniapp-smart-money`
  - `webapp-smart-money`
  - `analytics-performance`
- Affected code:
  - `backend/base/models/*`
  - `backend/base/database/mysql.go`
  - `backend/service/smart_money/*`
  - `backend/service/web_server/smart_money*.go`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `miniapp/src/lib/smartMoneyApi.js`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - 可能新增共用的仓位详情渲染组件
- Notes:
  - 本提案替代旧的 ClickHouse 方案，后续实现与验收以“ MySQL 持久化元数据 + 最小化链上实时读取”的混合模型为准。
