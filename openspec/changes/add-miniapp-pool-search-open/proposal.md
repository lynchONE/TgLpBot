# Change: MiniApp 支持搜索池子并一键开仓

## Why
- 当前 MiniApp 主要依赖「热门池子」列表进行一键开仓，但用户经常需要开非榜单池子，缺少“按池子ID/代币名称快速定位并开单”的入口。

## What Changes
- MiniApp「热门池子」页新增“搜索池子”入口（弹窗/抽屉）：
  - 支持按 **池子ID**（V3 pool address 或 V4 poolId）搜索
  - 支持按 **代币名称/符号** 搜索（第三方数据源）
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
1. ✅ 数据源口径：使用 DexScreener API 进行池子/代币搜索，并用其返回的 `liquidity.usd` 作为 TVL。
2. ✅ 手续费口径：手续费金额（`total_fees`）为 best-effort（用 `volume.h24 * feeTier` 估算）；若无法解析则返回 0 且前端不展示。
