## 1. 设计系统基础
- [x] 1.1 在 `webapp` 安装 TailwindCSS、PostCSS、Autoprefixer，并补齐配置文件。
- [x] 1.2 配置 Tailwind theme，使颜色、圆角、阴影、spacing 映射现有 CSS variables。
- [x] 1.3 引入必要的 Radix UI primitives 与轻量动效依赖，避免引入大型 UI 框架。
- [x] 1.4 新增 `webapp/src/components/ui/` 基础组件：Button、IconButton、Badge、Panel、Input、Tabs、Dialog、Popover、Select、Tooltip、Switch、Slider、Skeleton、EmptyState。
- [x] 1.5 保留旧样式入口并确保未迁移模块不受影响。

## 2. 高频区域迁移
- [x] 2.1 改造顶部栏、登录/用户区域、设置弹层、链切换和模块开关。
- [x] 2.2 改造热门池子模块的 toolbar、筛选弹层、状态操作和高度设置控件。
- [x] 2.3 改造 K 线工具栏、筛选/高度弹层、工具按钮、loading 和 error 状态的基础 UI 接入。
- [x] 2.4 将高频区域重复的按钮、输入框、弹层、滑杆样式替换为 `components/ui`。
- [x] 2.5 保持现有数据请求、鉴权、交易参数和本地存储 key 不变。

## 3. 组件结构收敛
- [x] 3.1 从 `App.jsx` 拆出 TopBar、登录验证码、游客热门池子和工作台配置展示组件。
- [x] 3.2 继续从 `App.jsx` 拆出 WorkbenchLayout、HotPoolsPanel、PositionsPanel 等大面板。
- [x] 3.3 从 `SmartMoneyDashboard.jsx` 拆出独立子面板和局部 hooks，避免继续扩大单文件。
- [x] 3.4 清理已迁移区域不再使用的 CSS 类，并保留仍被未迁移模块使用的样式。
- [x] 3.5 检查无障碍属性、键盘焦点、弹层关闭和 disabled/loading 状态。

## 4. 验证
- [x] 4.1 执行 `cd webapp; npm run build`。
- [x] 4.2 检查构建产物体积变化，确认没有引入明显不必要的大型依赖。
- [x] 4.3 进行桌面宽屏、普通桌面、窄屏视口的手动 UI 检查。
- [x] 4.4 做针对性 `git diff` 检查，确认没有改动资金交易、后端 API 语义、鉴权或钱包敏感逻辑。
