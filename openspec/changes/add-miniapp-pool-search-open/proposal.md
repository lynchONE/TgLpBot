# Change: MiniApp 支持搜索池子并一键开仓

## Why
- 当前 MiniApp 主要依赖「热门池子」列表进行一键开仓，但用户经常需要开非榜单池子，缺少“按池子ID/代币名称快速定位并开单”的入口。

## What Changes
- MiniApp「热门池子」页新增“搜索池子”入口（弹窗/抽屉）：
  - 支持按 **池子ID**（V3 pool address 或 V4 poolId）搜索
  - 支持按 **代币名称/符号** 搜索（匹配交易对字符串）
  - 搜索结果按 **TVL（current_pool_value）倒序** 排序
  - 最多展示 **10 条** 结果
- 后端新增 API：`GET /api/search_pools`（需 Telegram WebApp `initData` 认证）：
  - `q` 支持池子ID与代币名称/符号
  - 返回与热门池子卡片兼容的字段（`protocol_version`, `pool_address`, `trading_pair`, `current_pool_value` 等）
- 开仓流程复用现有“一键开仓”弹窗与 `POST /api/open_position`，不新增交易逻辑。

## Impact
- Affected specs (new):
  - `specs/miniapp-pool-search-open/spec.md`
  - `specs/pool-search-api/spec.md`
- Affected code (implementation stage):
  - Backend: `backend/service/web_server/server.go`, `backend/service/web_server/search_pools.go`（新增）
  - MiniApp: `miniapp/api/pools.js`, `miniapp/src/lib/api.js`, `miniapp/src/App.jsx`
- Backwards compatibility: 仅新增 API 与前端功能，不影响旧客户端。

## Open Questions (need your confirmation)
1. ✅ 代币名称匹配口径：本次先按 `trading_pair`（例如 `WBNB/USDT`）做不区分大小写的子串匹配。
2. ✅ 数据源口径：按 `poolm_top_fees_realtime` 的 `timeframe_minutes=5` 快照进行搜索与排序（TVL）。

