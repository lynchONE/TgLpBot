# Change: 补齐 Web 管理端授权与公告工作台

## Why
当前管理员页面只能查看在线用户、活跃任务和少量系统配置，无法在 `webapp` / `miniapp` 内完成授权发放、关闭授权、授权码生成、用户到期时间修改和公告发布。管理员仍需要依赖 Bot 侧入口，导致 Web 管理端无法覆盖日常运营闭环。

## What Changes
- 新增 Web 管理端授权工作台：支持搜索/分页查看用户授权、启用/停用授权、修改 MiniApp 权限、到期时间、钱包数、活跃任务数和备注。
- 新增授权码管理：支持生成授权码、查看授权码列表、启用/停用授权码、修改授权码有效期、兑换次数、额度和 MiniApp 权限。
- 新增公告管理：支持管理员在 Web 管理端发布公告、保存公告记录，并向所有 Telegram 用户广播发送结果。
- 在 `webapp` 与 `miniapp` 管理页中接入以上能力，保持管理员鉴权与现有 `/api/admin` 代理模式一致。

## Impact
- Affected specs: `admin-access-workbench`
- Affected code:
  - `backend/service/user/access.go`
  - `backend/service/web_server/admin_user_access.go`
  - `backend/service/web_server/server.go`
  - `backend/base/models/auth_code.go`
  - `backend/base/models/user_access.go`
  - `backend/base/models/announcement.go`
  - `webapp/src/api.js`
  - `webapp/src/components/AdminPanel.jsx`
  - `webapp/api/[...path].js`
  - `miniapp/src/lib/api.js`
  - `miniapp/src/components/AdminPage.jsx`
  - `miniapp/api/admin.js`
