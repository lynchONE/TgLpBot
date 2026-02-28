# MiniApp 小程序导航栏与图标重构

**更新日期**: 2026-02-28

## 1. 任务背景
在收到反馈后，确认当前 MiniApp 的导航栏（顶部与底部 Tab）显得比较落后，并且使用的图标体验不佳，缺乏符合现代 Web3 审美的质感与交互体验。

## 2. 改进点介绍

### 2.1 引入业界顶级的图标库：Lucide React
原系统使用了多达十几段生硬的 SVG path 代码维护图标。我们将其清空，并运行了 `npm install lucide-react`，从全球最受迎的开源图标库中挑选出符合业务含义的语义图标：
- **机器人/自动执行**: 改用 `Bot` 与 `Cpu`
- **仓位/数据**: 改用 `BarChart2` (更具专业感)
- **热门筛选**: 改用 `Flame` 与 `Filter`
- **管理看板**: 改用清晰的 `Settings`

### 2.2 底部浮动胶囊导航 (Modern Floating Pill Bottom Nav)
将紧贴底边的直角边框导航全部重构为了漂浮在内容上方的"胶囊容器"：
- 加上了 `backdrop-blur-xl` 的亚克力视觉虚化效果，使后置滚动的 K 线和数字能够若隐若现。
- 图标激活时，增加了弹性的 `scale-110` 过度动画。
- 加入了强烈的深浅色模式适配处理 (`dark:bg-[#1f222a]/90`)，使夜间模式更加深邃。

### 2.3 顶部切换栏重绘 (Top Tab Switchers)
顶部的 `POSITION_TASK_TABS` 与 `HOT_POOL_SORT_TABS` 从一个简单的细框变为了带有内发光投影细节 (`shadow-inner ring-1 ring-zinc-200/50`) 与悬浮块 (`relative bg-white shadow-sm ring-1 ring-black/5`) 的动态切换器。这能在手机触摸端提供更加精确和舒适的点击面。

## 3. 测试与验证
通过 `npm run build` 验证全部代码编译通过，没有任何构建警告与错误。相关的代码重构只修改了 UI 层展示逻辑 (`miniapp/src/App.jsx`)，不影响背后的实际业务开仓与刷新流程。
