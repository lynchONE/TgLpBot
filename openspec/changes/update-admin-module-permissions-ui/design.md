## Context
现有授权模型通过 `user_accesses.mini_app_enabled` 控制 MiniApp 总权限，通过 `max_wallets` 和 `max_active_tasks` 控制额度。Web/MiniApp 管理页已经有 `AdminAccessWorkbench`，但只暴露 MiniApp 总开关，无法表达“允许用户看热门池但不允许创建池子”这类运营需求。

现有前端模块注册散落在 `webapp/src/utils.js`、`webapp/src/App.jsx` 和 `miniapp/src/App.jsx`。本次需要先定义稳定的模块 key，再让后端保存和返回这些 key，前端用同一套 key 做入口过滤和授权编辑。

## Goals / Non-Goals
- Goals: 管理员能给用户和授权码勾选全部或部分功能模块。
- Goals: 模块授权在授权码兑换、用户授权更新、前端入口展示和后端关键访问校验中保持一致。
- Goals: 管理员页面简洁、美观、易扫读，降低停用授权、清空模块、发布公告等高影响操作的误触风险。
- Non-Goals: 不引入多角色 RBAC，不做组织/团队权限，不改变管理员身份来源。
- Non-Goals: 不把模块授权用于绕过现有业务安全校验；现有资金相关校验仍保持独立。

## Decisions
- Decision: 使用稳定字符串 key 表示可授权的顶层功能模块，例如 `hot_pools`、`positions`、`assets`、`smart_money`、`swap`、`create_pool`、`admin_panel`。钱包管理、交易记录、全局配置仍属于“我的/资产”内部功能，不作为单独授权入口展示。前后端共享同一语义，未知 key 在保存时 MUST 被拒绝。
- Decision: 用户授权与授权码分别持久化模块 key 清单。为避免 `mini_app_enabled=true` 但模块列表为空造成隐式全开放，后端 MUST 明确区分“未配置模块列表”和“配置为空列表”。新建授权默认不依赖静默 fallback；需要授权全部模块时写入完整模块列表。
- Decision: 管理员账号始终具有管理模块访问权，但普通用户只有在整体授权有效、MiniApp 开关开启且模块 key 被授权时才能访问对应模块。
- Decision: 前端管理页采用“左侧对象列表 + 右侧编辑详情”的桌面布局，移动端改为上下分段；模块权限用分组 checkbox 网格展示，提供“全选全部功能”“清空全部”“按组全选”。
- Decision: 更新 API 时使用显式字段，例如 `enabled_modules` / `module_permissions`。清空模块必须是管理员显式提交空数组，不能通过字段缺失触发。

## Risks / Trade-offs
- 旧数据没有模块列表：迁移或启动补齐必须明确策略，建议对已有 `mini_app_enabled=true` 的记录写入当前全部用户功能模块，避免上线后用户突然失去入口。
- 前端只隐藏入口不足以形成安全边界：后端关键 API 需要按模块增加校验，至少覆盖创建池子、开仓/加仓/任务动作、钱包管理、兑换、资产与聪明钱查询。
- 模块 key 散落会增加维护成本：应集中定义模块目录，前端从同一常量驱动 UI、入口过滤和授权表单。

## Migration Plan
1. 增加保存模块授权的字段或关联表，并为 `user_accesses` 与 `auth_codes` 建立明确的读写方法。
2. 对已有授权数据执行一次性迁移：`mini_app_enabled=true` 的用户和授权码写入当前全部非管理员用户功能模块；`mini_app_enabled=false` 的记录写入空模块列表。
3. 扩展管理员 API、DTO 和单测，确认字段缺失不会覆盖模块，空数组会清空模块。
4. 接入前端模块过滤和管理页重设计。

## Open Questions
- 是否允许普通用户获得 `admin_panel` 模块入口？建议不允许，管理员模块仍只由管理员身份决定，授权矩阵中仅展示为只读说明或完全隐藏。
- `webapp` 与 `miniapp` 的模块 key 是否需要完全一致？建议使用同一份后端模块目录；前端没有实现的模块可以不展示，但不应创造新的语义 key。
