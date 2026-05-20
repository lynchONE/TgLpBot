## Context
已有数据模型包含 `user_accesses`、`auth_codes`、`announcements`，Bot 侧也已有授权码和公告流程。Web API 目前只有 `user_access` 的单字段更新，无法覆盖管理员日常运营。

## Goals / Non-Goals
- Goals: 在 `webapp` 与 `miniapp` 中提供完整的授权、授权码和公告管理闭环。
- Goals: 复用现有模型、服务与管理员鉴权，不引入新的权限系统。
- Goals: 保持所有管理员操作都需要 Telegram WebApp initData 验证，并确认调用者是管理员。
- Non-Goals: 不改变现有授权码兑换语义，不迁移旧数据，不增加新的角色体系。

## Decisions
- Decision: 新增独立的 `admin_access`、`admin_auth_codes`、`admin_announcements` endpoint，并继续支持 `/api/admin?endpoint=...` 代理模式。
- Decision: 用户授权编辑使用明确字段更新，不用空值或默认值覆盖未提交字段；需要清空到期时间时使用显式 `clear_active_to`。
- Decision: 停用授权使用 `revoked_at` / `revoked_by_user_id`，恢复授权清空停用字段，不删除授权记录。
- Decision: 公告发布先写入 `announcements`，再遍历用户发送 Telegram 消息并返回发送统计；单个用户发送失败不得回滚公告记录。

## Risks / Trade-offs
- 公告广播可能较慢：后端应限制请求体大小并返回成功/失败统计；如后续需要更高吞吐，再引入异步队列。
- 授权时间字段容易被误清空：前端必须区分“不修改”和“清空”，后端只接受显式清空标记。
- 管理员误操作会影响用户权限：UI 需要对停用授权、停用授权码和公告发布使用确认操作。

## Migration Plan
1. 复用现有 GORM AutoMigrate 模型，无需新增表。
2. 后端先补齐服务和 API。
3. 前端再接入管理页。
4. 构建和测试通过后上线。

## Open Questions
- 公告是否需要支持只发给已授权用户或全部历史用户？本次默认面向全部 Telegram 用户，后续可按运营需求扩展筛选。
