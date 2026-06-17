## ADDED Requirements

### Requirement: WebApp MUST provide a local design system layer
WebApp MUST provide a local reusable design system layer under `webapp/src/components/ui/` for shared controls, panels, feedback states, and overlays.

#### Scenario: 业务模块使用基础按钮和面板
- **WHEN** WebApp 业务模块需要按钮、图标按钮、徽章、面板、输入框或空状态
- **THEN** 模块 MUST 优先使用 `components/ui` 中的基础组件
- **AND** 新组件 MUST 使用统一 token，而不是在业务模块中重复定义局部视觉规则

#### Scenario: 业务模块需要弹层交互
- **WHEN** WebApp 业务模块需要 Dialog、Popover、Select、Tabs、Tooltip、Switch 或 Slider
- **THEN** 模块 MUST 使用本地封装的交互组件
- **AND** 组件 MUST 提供明确的 focus、disabled、loading 和关闭行为

### Requirement: WebApp styling MUST use shared theme tokens
WebApp styling MUST use shared theme tokens for color, spacing, radius, shadow, typography, and motion.

#### Scenario: 新增或迁移 UI 样式
- **WHEN** 新增或迁移 WebApp UI
- **THEN** 样式 MUST 复用 Tailwind theme 或 CSS variables 中的 token
- **AND** 不得新增与既有 token 冲突的散落颜色、圆角、阴影或 spacing 体系

#### Scenario: 用户切换主题强调色
- **WHEN** 用户切换现有强调色主题
- **THEN** 已迁移组件 MUST 跟随主题 token 更新强调色、边框、hover 和 active 状态

### Requirement: WebApp visual direction MUST remain a professional dark trading workbench
WebApp visual design MUST remain a professional dark trading workbench that prioritizes data density, clarity, and predictable operation over decorative effects.

#### Scenario: 用户查看工作台模块
- **WHEN** 用户查看热门池子、K 线、仓位、聪明钱或资产模块
- **THEN** 页面 MUST 保持高密度但可扫描的信息层级
- **AND** 状态色 MUST 清晰表达收益、风险、警告、禁用和加载状态

#### Scenario: 新增视觉装饰
- **WHEN** 新增背景、阴影、渐变、模糊或动效
- **THEN** 装饰 MUST 不遮挡内容、不降低对比度、不破坏数据可读性
- **AND** 不得把工作台改造成营销页或炫酷大屏风格

### Requirement: WebApp interaction components MUST be accessible and keyboard-operable
Interactive WebApp components MUST support keyboard operation, visible focus, accessible labels, and predictable dismissal.

#### Scenario: 用户使用键盘操作弹层
- **WHEN** 用户通过键盘打开 Dialog、Popover、Select、Tabs 或 Tooltip 相关控件
- **THEN** 焦点 MUST 移动到合理位置
- **AND** 用户 MUST 能通过键盘选择、确认、取消或关闭

#### Scenario: 用户启用减少动态效果偏好
- **WHEN** 浏览器设置 `prefers-reduced-motion: reduce`
- **THEN** WebApp MUST 降低或关闭非必要动画
- **AND** 关键状态变化仍 MUST 通过静态视觉反馈可见

### Requirement: WebApp design migration MUST preserve business behavior
The WebApp design migration MUST preserve existing business behavior, API semantics, local storage keys, authentication flows, and trading-related request parameters.

#### Scenario: 改造顶部栏和模块工具栏
- **WHEN** 顶部栏、设置弹层、模块开关、热门池子工具栏或 K 线工具栏完成迁移
- **THEN** 原有数据请求、刷新间隔、本地存储 key 和用户操作结果 MUST 保持兼容
- **AND** 迁移不得改变后端 API 参数语义

#### Scenario: 涉及资金操作的控件被迁移
- **WHEN** 开仓、撤仓、补流动性、Swap 或钱包相关控件被迁移
- **THEN** 控件 MUST 保持原有确认、禁用、错误展示和参数传递行为
- **AND** 不得引入自动执行、静默 fallback 或隐藏错误的默认行为

### Requirement: WebApp migration MUST be incremental and verifiable
WebApp design migration MUST be incremental and verifiable, with each migrated area buildable and reviewable.

#### Scenario: 完成一个迁移阶段
- **WHEN** 一个迁移阶段完成
- **THEN** `cd webapp; npm run build` MUST pass
- **AND** 变更 MUST 经过针对性 diff 检查，确认组件 API、调用方和样式入口匹配

#### Scenario: 迁移期间存在旧样式和新组件并存
- **WHEN** 某些模块尚未迁移
- **THEN** 旧样式 MAY 暂时保留
- **AND** 新迁移区域 MUST 不破坏未迁移模块的布局、颜色或交互
