 # TgLpBot Mini App (实时仓位)

## 开发

```bash
npm install
npm run dev
```

默认会请求 `http://localhost:8080/api/realtime_positions`，可通过环境变量覆盖：

```bash
VITE_API_BASE_URL=http://localhost:8080
```

## 部署

1) 部署到 Vercel 后，将 *Production 域名*（建议用 `xxx.vercel.app` 或自定义域名，避免使用带随机后缀的单次 Deployment URL）填到后端的 `TELEGRAM_WEBAPP_URL`（`backend/.env`）。

> 如果是把整个仓库导入 Vercel，请在 Project Settings 里把 Root Directory 设为 `miniapp/`。

2) 配置 Mini App 调用后端 API 的方式（二选一）：

**方式 A（推荐，后端有 HTTPS 域名）**：在 Vercel 环境变量设置

```bash
VITE_API_BASE_URL=https://<你的后端域名>
```

**方式 B（后端只有 HTTP/端口）**：使用 Vercel Function 代理（`miniapp/api/realtime_positions.js`）

```bash
BACKEND_API_BASE_URL=http://<你的服务器IP>:8080
```

> 方式 B 不要求后端支持 HTTPS；浏览器只访问 Vercel（HTTPS），由 Vercel 服务器转发到后端（HTTP）。
> 可用 `https://<你的Vercel域名>/api/config` 快速验证代理是否连通后端。

3) 在 @BotFather 为你的 Bot 配置 WebApp 允许域名（`/setdomain`），填你的 Vercel/自定义域名；否则 WebApp 按钮可能打不开。

常见报错排查：
- 页面提示 `DEPLOYMENT_NOT_FOUND`：`TELEGRAM_WEBAPP_URL` 指向了不存在/已删除的 Vercel 部署链接，请换成稳定的 Production 域名。
- 报错 `No more than 12 Serverless Functions`：Vercel Hobby 计划限制最多 12 个函数。当前项目已通过合并 API 来解决（共 9 个函数）。

## API 合并优化说明

为了兼容 Vercel Hobby 计划的 12 个 Serverless Functions 限制，本项目将多个相关的 API 合并：

| 合并后的端点 | 原端点 | 使用方式 |
|------------|--------|---------|
| `/api/task_action?action=xxx` | `/api/task_delete`, `/api/task_pause`, `/api/task_stop` | action 可选值: `delete`, `pause`, `stop` |
| `/api/admin?endpoint=xxx` | `/api/admin/autolp_disable`, `/api/admin/autolp_stats`, `/api/admin/realtime_positions`, `/api/admin/realtime_users` | endpoint 可选值: `autolp_disable`, `autolp_stats`, `realtime_positions`, `realtime_users` |

**当前 API 文件数量**: 9 个（在 12 个限制内）
