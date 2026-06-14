## 1. Backend
- [x] 1.1 抽出池子雷达扫描回调能力，复用 V3/V4 解析、金额计算、owner 归因和排除规则。
- [x] 1.2 新增 SSE endpoint `/api/sm/pool_liquidity_wallet_candidates_stream`，复用现有参数校验、权限和扫描限制。
- [x] 1.3 发送 `stage`、`candidate`、`warning`、`summary`、`error`、`done` 事件，并在客户端断开时取消扫描。
- [x] 1.4 对候选钱包做服务端 upsert 聚合，保证同一钱包更新而不是重复推送无意义行。
- [x] 1.5 为流式事件、取消、超时 partial 和错误传播补充后端测试。

## 2. Proxy
- [x] 2.1 WebApp proxy allowlist 增加 stream endpoint，并对 SSE 响应启用透传。
- [x] 2.2 MiniApp proxy allowlist 增加 stream endpoint，并对 SSE 响应启用透传。
- [x] 2.3 验证代理不会 `await upstream.text()` 缓冲流式响应。

## 3. Frontend
- [x] 3.1 WebApp API helper 增加流式候选扫描函数，支持 AbortController 和 SSE JSON 解析。
- [x] 3.2 MiniApp API helper 增加流式候选扫描函数，支持 AbortController 和 SSE JSON 解析。
- [x] 3.3 WebApp 雷达弹窗收到 `candidate` 事件时立即插入/更新候选表，并更新执行日志。
- [x] 3.4 MiniApp 雷达面板收到 `candidate` 事件时立即插入/更新候选列表，并更新执行日志。
- [x] 3.5 扫描中显示停止入口；扫描完成或停止后才允许导入当前候选。

## 4. Validation
- [x] 4.1 `cd backend; go test ./service/pricing ./service/web_server ./service/smart_money`
- [x] 4.2 `cd webapp; npm run build`
- [x] 4.3 `cd miniapp; npm run build`
- [x] 4.4 针对性检查 diff，确认没有把流式失败伪装成成功。
