## Context
`webapp` 当前已经使用 React 18 + Vite，并且已有一套 CSS token、`PanelShell`、`CustomSelect`、弹层、按钮、面板等自研基础能力。问题不在于缺少 React 框架，而在于样式和交互基座没有形成可复用边界：

- 大量业务组件直接绑定局部 CSS 类，按钮、标签、弹层、输入框、列表行和 tabs 的实现重复。
- `webapp/src/styles/` 中存在多份业务样式，视觉语言接近但不完全一致。
- `App.jsx` 与 `SmartMoneyDashboard.jsx` 过大，新增 UI 容易继续堆叠。
- 当前视觉方向是暗色金融工作台，具备基础辨识度，但玻璃、光晕、圆角、渐变和 hover 反馈不够统一。

本次重构应保持桌面交易工作台心智：安静、专业、高信息密度、状态明确、操作可预期。

## Goals / Non-Goals
- Goals: 建立可复用的 WebApp UI 基座，并让高频区域先获得统一现代化体验。
- Goals: 使用 TailwindCSS 管理 spacing、颜色、圆角、阴影和状态 token，减少继续新增散落 CSS。
- Goals: 使用 Radix UI primitives 提升弹层、下拉、标签页、开关、滑杆和 tooltip 的键盘可用性与状态一致性。
- Goals: 采用 shadcn/ui 的本地组件思想，组件代码保留在仓库内，方便按项目风格裁剪。
- Goals: 用轻量动效改善交互反馈，但不得把工作台做成营销页或炫酷大屏。
- Goals: 按模块渐进迁移，避免一次性重写导致交易相关流程回归。
- Non-Goals: 不迁移到 Next.js、Ant Design、MUI 或其他大型全家桶 UI 框架。
- Non-Goals: 不改后端 API、数据库、鉴权、交易执行、钱包管理和策略执行逻辑。
- Non-Goals: 不重写 `miniapp`。
- Non-Goals: 不为了视觉效果牺牲数据密度、加载性能或移动端可读性。

## Decisions
- Decision: 保留 React/Vite，新增 TailwindCSS、PostCSS、Autoprefixer。
  - Alternatives considered: 全量换 Next.js 或大型 UI 框架。拒绝原因是当前项目并不需要 SSR/路由框架，也不能接受大规模业务迁移风险。
- Decision: 引入 Radix UI primitives，封装成本地 `components/ui`。
  - Alternatives considered: 继续维护纯手写弹层/下拉。拒绝原因是键盘交互、焦点管理和可访问性容易重复出错。
- Decision: 使用 shadcn/ui 的代码组织思想，而不是把 shadcn 当运行时组件库。
  - Alternatives considered: 直接使用第三方成品主题。拒绝原因是项目需要暗色金融工作台风格，并且现有组件需要逐步兼容。
- Decision: 用 CSS variables 作为主题源，Tailwind theme 映射到这些变量。
  - Alternatives considered: 完全用 Tailwind 默认色板。拒绝原因是会削弱现有品牌和业务状态色。
- Decision: UI 组件先服务 WebApp，不抽成跨 `miniapp` 包。
  - Alternatives considered: 立即做 monorepo 共享组件包。拒绝原因是会扩大改造面，且 miniapp 目前已有 Tailwind 栈。
- Decision: 先迁移高频工作台区域，再拆分长尾模块。
  - Alternatives considered: 全站一次性重写。拒绝原因是现有交易、聪明钱和管理模块复杂，风险高。

## Component Model
新增 `webapp/src/components/ui/`：

- `Button` / `IconButton`
- `Badge` / `StatusBadge`
- `Panel` / `PanelHeader` / `MetricTile`
- `Input` / `Field` / `NumberInput`
- `Tabs`
- `Dialog` / `ConfirmDialog` 适配层
- `Popover`
- `Select`
- `Tooltip`
- `Switch`
- `Slider`
- `Skeleton`
- `EmptyState`

组件要求：

- 封装基础交互与样式，不包含业务 API 调用。
- 支持 `className` 扩展，但默认样式必须来自统一 token。
- 可用 lucide-react 图标。
- 交互组件必须提供明确 focus 状态、disabled 状态、loading 状态和 aria 信息。
- 业务组件逐步从局部 CSS 类迁移到这些组件，不一次性删除所有旧 CSS。

## Visual Direction
风格定义为“专业深色交易工作台”：

- 背景以深色中性面为主，避免页面被单一绿色或黄色支配。
- 主强调色保留绿色，黄色作为可选主题或警示辅助，不做大面积主色。
- 面板半径以 8-12px 为主，复杂弹窗可更大但需克制。
- 保留高密度数据表、紧凑工具栏、清晰分隔线和数字对齐。
- 减少纯装饰光晕与大面积玻璃模糊，保留必要层级感。
- 动效聚焦状态变化：弹层进入、筛选打开、行 hover、加载骨架、操作反馈。

## Migration Plan
1. 基座阶段：
   - 安装 TailwindCSS / PostCSS / Autoprefixer。
   - 建立 Tailwind config，并映射现有 CSS variables。
   - 新增 `components/ui` 基础组件。
   - 保留现有 `styles.css` import，避免破坏未迁移模块。
2. 高频区域阶段：
   - 改造顶部栏、用户区域、设置弹层、链切换、模块开关。
   - 改造热门池子 toolbar、筛选 popover、排序 tabs、池子行状态。
   - 改造 K 线工具栏、筛选/高度弹层、图标工具按钮。
3. 面板扩展阶段：
   - 迁移仓位、Swap、钱包、交易历史、聪明钱核心列表。
   - 把重复样式沉淀到 UI 组件或 feature 层组件。
4. 结构收敛阶段：
   - 从 `App.jsx` 拆出 `WorkbenchLayout`、`TopBar`、`HotPoolsPanel`、`PositionsPanel` 等。
   - 从 `SmartMoneyDashboard.jsx` 拆出独立子面板和 hooks。
   - 清理不再使用的 CSS 类，并做针对性 diff 检查。

## Risks / Trade-offs
- 风险: 新增依赖导致 bundle 增长。
  - Mitigation: 仅安装按需使用的 Radix 包，避免引入大型 UI 全家桶，并通过 `npm run build` 查看构建体积。
- 风险: Tailwind 与现有 CSS 并存期间样式冲突。
  - Mitigation: 使用 CSS variables 做统一源，迁移期间保留旧类名，优先迁移独立区域。
- 风险: 一次性改动太多导致交易工作流回归。
  - Mitigation: 分阶段迁移，首阶段只改 UI 基座和展示交互，不改 API 请求语义与交易参数。
- 风险: shadcn 默认视觉不适合项目。
  - Mitigation: 只借鉴组件结构，本地组件以项目 token 重写视觉。
- 风险: 手写 CSS 被 Tailwind 与组件层双轨长期拖累。
  - Mitigation: 每迁移一个区域，都删除或收敛对应旧样式，并做 diff 检查。

## Validation
- 每个阶段必须运行 `cd webapp; npm run build`。
- 每次修改后必须做针对性 `git diff` 检查，确认调用方、组件 API 和样式入口匹配。
- UI 改造完成后需要手动检查桌面宽屏、普通桌面和窄屏视口。
- 弹层、下拉、tabs、tooltip、dialog 必须检查键盘焦点和关闭行为。

## Open Questions
- 是否允许在第一阶段同时压缩或替换 `webapp/src/icon/avatar_*.png` 大图资源？这属于性能优化，可单独拆变更。
- 是否需要把 `miniapp` 的 Tailwind token 与 `webapp` 新 token 对齐？当前建议暂不纳入第一阶段。
