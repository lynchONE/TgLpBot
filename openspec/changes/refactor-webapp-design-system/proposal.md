# Change: 重构 WebApp 设计系统与交互基座

## Why
当前 `webapp` 已是 React/Vite 项目，但样式主要依赖大量手写 CSS 和业务局部类名；`App.jsx`、`SmartMoneyDashboard.jsx` 等组件承载过多 UI 与状态逻辑，导致视觉统一、交互一致性和后续迭代成本都变高。

WebApp 的目标是桌面端高密度金融工作台，需要更现代、统一、易用且不过度花哨的界面基座，而不是简单替换为大型 UI 框架。

## What Changes
- 在 `webapp` 引入 TailwindCSS 作为样式 token 与 utility 基座，并继续保留 React/Vite 架构。
- 引入 Radix UI primitives 与 shadcn/ui 风格的本地组件封装，用于 Dialog、Popover、Select、Tabs、Tooltip、Switch、Slider 等交互基础件。
- 引入轻量动效能力，用于弹层、面板切换、加载态和行状态反馈；动效必须克制，并遵守 `prefers-reduced-motion`。
- 新增 `webapp/src/components/ui/` 组件层，统一按钮、图标按钮、徽章、面板、输入框、标签页、弹层、下拉、提示、骨架屏等基础 UI。
- 收敛现有暗色金融工作台风格：保留高对比、数据密度和明确状态色，减少散落的玻璃拟态、光晕、局部渐变和不一致圆角。
- 分阶段迁移高频区域：顶部栏与设置弹层、热门池子模块、K 线工具栏与筛选弹层，然后再扩展到仓位、聪明钱、资产与管理模块。
- 拆分超大组件中的 UI 层与业务状态层，避免继续把新界面逻辑堆入 `App.jsx` 或单个大组件。
- 不改变后端 API、交易执行逻辑、鉴权逻辑和资金相关默认行为。

## Impact
- Affected specs: `webapp-design-system`
- Affected code:
  - `webapp/package.json`
  - `webapp/postcss.config.*`
  - `webapp/tailwind.config.*`
  - `webapp/src/styles/*`
  - `webapp/src/components/ui/*`
  - `webapp/src/App.jsx`
  - `webapp/src/components/*`
- 新增前端依赖会影响 `webapp/package-lock.json` 与构建产物体积，需要通过 `npm run build` 验证。
- 本变更为 UI 架构与体验重构，不应修改资金交易、自动策略、钱包密钥、后端接口语义或权限判定。
