# Change: 聪明钱池子雷达支持流式候选展示

## Why
当前池子雷达的候选预览仍以一次 HTTP 请求返回，用户必须等扫描结束或 partial 返回后才能看到钱包。大时间窗或 RPC 较慢时，即使后端已经解析出符合条件的钱包，页面也无法做到“找到一个就展示一个”。

## What Changes
- 新增池子雷达候选钱包流式预览接口，使用 SSE 按事件推送扫描阶段、候选钱包、warning、summary 和 done。
- 后端扫描链路在每解析出一个符合条件的钱包时立即 emit 候选事件；同一钱包后续有更大金额或更新交易时发送 upsert。
- WebApp 和 MiniApp 的雷达弹窗优先使用流式接口，收到一个候选就插入/更新列表，并保留扫描进度与执行日志。
- Vercel API proxy 支持流式透传，不再对该 endpoint 读取完整响应后再返回。
- 保留现有批量候选接口作为兼容路径和不支持流式环境的降级路径，但降级必须在 UI 日志中可见。

## Impact
- Affected specs:
  - `smart-money-pool-radar`
- Affected code:
  - Backend: `backend/service/smart_money/token_liquidity.go`, `backend/service/web_server/smart_money.go`
  - WebApp: `webapp/src/smartMoneyApi.js`, `webapp/src/components/SmartMoneyDashboard.jsx`, `webapp/api/sm.js`
  - MiniApp: `miniapp/src/lib/smartMoneyApi.js`, `miniapp/src/components/SmartMoneyPage.jsx`, `miniapp/api/sm.js`
  - Tests: `backend/service/smart_money/*_test.go`, `backend/service/web_server/*_test.go`
- Compatibility:
  - 现有 `pool_liquidity_wallet_candidates` JSON 接口继续保留。
  - 新接口只改变预览交互，不改变导入接口和 `monitored_wallets` 写入规则。
