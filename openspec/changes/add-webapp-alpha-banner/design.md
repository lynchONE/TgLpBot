## Context
Alpha 信息来自两个外部公开 JSON 数据源：
- `https://alpha123.uk/api/data?fresh=1`：主数据源，当前需要使用 `airdrops`。
- `https://alpha123.uk/stability/stability_feed_v3.json`：稳定度看板，字段为压缩格式，例如 `n`、`p`、`st`、`md`、`spr`。

webapp 首页顶部空间有限，现有顶部栏左侧是品牌，右侧是登录/用户操作。新增信息应放在顶部栏中间，并在移动端或窄屏下自动换行或压缩。

## Goals / Non-Goals
- Goals:
  - 在登录态和访客态首页都能展示 Alpha 摘要。
  - `airdrops` 项目展示 Token、项目名、数量、积分、日期时间。
  - 稳定度看板只展示摘要和少量异常项目，避免占满顶部。
  - 外部接口异常时不阻断首页主功能，并明确显示 Alpha 加载失败状态。
- Non-Goals:
  - 不在本次实现完整稳定度详情页。
  - 不把 Alpha 数据写入后端数据库。
  - 不改变现有热门池子、K 线、仓位、聪明钱模块的数据刷新逻辑。

## Decisions
- Decision: 新增独立 `AlphaTicker` 前端组件，挂载到 `TopBar` 中间区域。
  - Why: 顶部栏已有清晰的左中右布局边界，独立组件可以降低对现有登录和设置逻辑的影响。
- Decision: Alpha 主数据与稳定度数据并行读取，并在组件内部做轻量归一化。
  - Why: 两个接口互不依赖，任一接口失败不应拖慢另一部分展示。
- Decision: 稳定度只展示 `st` 非绿色或前 3 个条目，并将 `red:unstable`、`red:no_trade` 等状态映射为短标签。
  - Why: 顶部空间有限，展示异常优先级比完整表格更符合首页提示场景。
- Decision: 若浏览器直连遇到跨域或防盗链限制，使用 webapp 的 Vercel API 代理转发两个 Alpha 源。
  - Why: 现有 `webapp/api` 已有代理结构，放在同一前端项目内可以避免改动后端主服务。
- Decision: Docker/nginx 静态部署时，由 Go 后端提供同名 `/api/alpha` 代理。
  - Why: nginx 当前会把 `/api/*` 反代到后端，Vercel 函数不会参与该部署路径。

## Risks / Trade-offs
- 外部接口不可用或响应结构变化会导致顶部信息缺失。
  - Mitigation: 两个 Alpha 源独立请求，部分失败时仍返回可用数据并显示更新失败标记，不影响首页其它模块。
- 顶部空间不足时信息可能挤压登录操作。
  - Mitigation: 桌面端居中展示，窄屏下信息条换行到下一行并限制高度。

## Open Questions
- 是否需要点击 Alpha 条跳转到外部 Alpha 页面或展开详情面板？本次默认不做跳转，只展示摘要。
