## Context
池子雷达当前已经支持按 V3 pool address / V4 poolId 从 RPC 扫描 LP 事件，并在普通 JSON 接口中支持 partial 返回。但用户需要更直接的反馈：后端每找到一个符合条件的钱包，前端就立即展示一个。

SSE 适合这个场景，因为扫描是后端单向推送，前端只需要持续接收阶段和候选事件。项目当前代理层会 `await upstream.text()`，如果复用该路径会缓冲完整响应，必须为流式 endpoint 做透传。

## Goals / Non-Goals
- Goals:
  - 候选钱包一经后端确认符合条件就推送到前端。
  - 同一钱包重复命中时以前端 upsert 方式更新最高金额、最近交易和证据字段。
  - 扫描阶段、排除数量、warning、summary、done/error 都能在 UI 日志中呈现。
  - 用户关闭弹窗或重新扫描时能够中止后端扫描。
  - 不用空钱包、默认金额、默认价格等 fallback 伪造候选。
- Non-Goals:
  - 不改变导入接口。
  - 不引入新的第三方索引服务。
  - 不把雷达预览事件写入 `sm_lp_events`。
  - 不要求首版支持断线续传；断线后用户重新扫描。

## Decisions
- Decision: 使用 `GET /api/sm/pool_liquidity_wallet_candidates_stream` 作为 SSE endpoint。
  - Why: 与现有候选接口参数保持一致，前端可以复用参数构造；SSE 是浏览器原生可消费的单向事件流。
- Decision: 后端事件类型固定为 `stage`、`candidate`、`warning`、`summary`、`error`、`done`。
  - Why: 前端可以稳定区分进度、数据、可恢复提示和终态。
- Decision: `candidate` 事件使用钱包地址作为 upsert key。
  - Why: 现有聚合结果以钱包为主，同一钱包多次加池时应更新金额和最近交易，而不是重复插入多行。
- Decision: 流式扫描复用现有 RPC 解析、金额计算和 owner 归因逻辑。
  - Why: 避免出现 batch 与 stream 两套候选判定标准。
- Decision: 代理层对流式 endpoint 直接转发 `ReadableStream`/Node stream。
  - Why: 如果代理读取完整 body，SSE 会退化为普通长请求，无法做到实时插入。

## Event Shape
每个 SSE message 使用 JSON payload。

- `stage`: `{ "stage": "scanning_logs", "message": "...", "scanned_blocks": 1234 }`
- `candidate`: `{ "candidate": { ...TokenLiquidityCandidate }, "candidate_count": 3, "excluded_count": 18 }`
- `warning`: `{ "message": "...", "code": "partial_timeout" }`
- `summary`: `{ "candidate_count": 5, "excluded_count": 120, "partial": false, "warnings": [] }`
- `error`: `{ "message": "...", "recoverable": false }`
- `done`: `{ "partial": false }`

## Risks / Trade-offs
- 风险：部分 serverless 平台会缓冲响应。
  - Mitigation: Vercel proxy 对流式 endpoint 必须透传 body，并设置 `Cache-Control: no-store`、`Content-Type: text/event-stream`、`X-Accel-Buffering: no`。
- 风险：流式扫描仍受 RPC 单个 chunk 返回速度限制。
  - Mitigation: chunk 返回后立即解析并推送；如果 RPC 在首个 chunk 前卡住，UI 至少显示 stage 和 elapsed。
- 风险：用户在扫描未完成时导入部分候选，可能误以为结果完整。
  - Mitigation: 首版允许查看和勾选已到达候选，但导入按钮在扫描完成或用户主动停止后才启用。

## Migration Plan
1. 后端新增 stream scanner 回调能力，普通 JSON 接口继续使用聚合结果。
2. 新增 SSE handler，并接入同一权限、参数校验、成本限制。
3. WebApp/MiniApp API helper 增加 SSE 解析，弹窗优先使用流式预览。
4. Vercel proxy allowlist 增加 stream endpoint，并为该 endpoint 启用透传。
5. 保留现有 JSON 预览作为浏览器不支持流式读取时的显式降级。

## Open Questions
- 是否需要“停止扫描”按钮独立于关闭弹窗展示？建议首版增加，因为流式扫描会持续占用 RPC。
- 导入按钮是否允许扫描中使用？建议首版扫描中禁用，避免用户把部分结果误认为完整集合。
