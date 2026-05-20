## 1. Backend
- [x] 1.1 扩展用户授权服务，支持管理员分页/搜索用户授权、创建或更新授权、停用/恢复授权，并校验时间范围与额度参数。
- [x] 1.2 新增授权码管理 Web API，支持列表、创建、更新、启用、停用，并返回兑换状态与有效期信息。
- [x] 1.3 新增公告管理 Web API，支持公告列表、发布公告、记录发送成功/失败统计，并复用现有 Telegram 发送能力。
- [x] 1.4 将新接口接入 `/api/admin` endpoint 分发和 `/api/admin/*` 兼容路由，保持 Telegram WebApp initData + 管理员校验。
- [x] 1.5 补充后端单测，覆盖非管理员拒绝、参数校验、授权更新、授权码状态切换和公告发布记录。

## 2. Frontend
- [x] 2.1 在 `webapp/src/api.js` 与 `miniapp/src/lib/api.js` 增加管理员授权、授权码、公告 API 客户端。
- [x] 2.2 更新 `webapp` 管理员页面，增加“用户授权”“授权码”“公告”入口与表单操作。
- [x] 2.3 更新 `miniapp` 管理页，增加同等功能并适配移动端布局。
- [x] 2.4 更新 Vercel 代理端点白名单，确保 `webapp` 与 `miniapp` 都能访问新增管理员 API。

## 3. Verification
- [x] 3.1 运行 `cd backend; go test ./...`。
- [x] 3.2 运行 `cd webapp; npm run build`。
- [x] 3.3 运行 `cd miniapp; npm run build`。
- [x] 3.4 做针对性 diff 检查，确认接口字段、调用方和 UI 状态一致。
