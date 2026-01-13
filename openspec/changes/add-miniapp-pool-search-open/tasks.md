## 1. Implementation
- [x] 1.1 Backend：新增 `GET /api/search_pools`（initData 认证），支持按池子ID/代币名称搜索；按 TVL 倒序，最多返回 10 条
- [x] 1.2 MiniApp Proxy：扩展 `miniapp/api/pools.js`，新增 `endpoint=search_pools` → `/api/search_pools` 转发
- [x] 1.3 MiniApp API：在 `miniapp/src/lib/api.js` 增加 `fetchSearchPools`
- [x] 1.4 MiniApp UI：在「热门池子」页增加搜索入口与弹窗；选择结果后复用现有“一键开仓”弹窗开单
- [x] 1.5 验证：`cd backend; go test ./...` 与 `cd miniapp; npm run build`
