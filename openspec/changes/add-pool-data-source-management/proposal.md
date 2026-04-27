# Change: 增加池子同步数据源管理

## Why
当前 `pool_sync` 的上游地址主要来自 `.env` 中的 PoolM base URL。PoolM 异常、限流或字段变更时，需要改配置并重启服务，热门池子、智能狗池子监控和开仓流动性参考都会受影响。

需要把池子同步数据源改为数据库可管理，并允许管理员在 PoolM 与备用来源之间新增、切换和禁用，降低单一上游故障风险。

## What Changes
- 新增数据库驱动的池子数据源池，保存数据源名称、类型、base URL、请求参数模板、当前启用状态、健康状态和错误信息。
- `pool_sync` 从“固定 PoolM base URL”改为“读取当前启用数据源并通过适配器拉取”，支持至少两类来源：
  - PoolM `top-fees/5` 兼容来源。
  - 本地/备用 `market/pools` 兼容来源，例如 `/api/market/pools?timeframe=5m&limit=...&protocol=...&dex=...`。
- 管理员可以通过后台 API/UI 查看数据源列表、新增来源、切换当前来源、禁用/启用来源、触发连通性检查。
- 保持向后兼容：数据库没有配置数据源时，继续使用现有 `POOLS_SYNC_POOLM_BASE_URL` 或默认 PoolM 地址。
- 同步写入 `pools` 时记录实际来源，便于排查某条快照来自哪个数据源。

## Impact
- Affected specs: `pool-catalog`
- Affected code:
  - `backend/base/models/*`
  - `backend/base/database/*`
  - `backend/service/pool_sync/*`
  - `backend/service/web_server/*`
  - `miniapp/src/components/AdminPage.jsx`
  - `miniapp/src/lib/api.js`
  - `miniapp/api/admin.js`
  - 可选：`webapp/src/api.js` 与 webapp 管理页
- Backwards compatibility: additive；空 DB 数据源池时继续走现有 env/default PoolM 行为。
- Operational risk: 新来源字段可能与 PoolM 命名不同，适配层必须明确做字段归一化和缺字段校验。
