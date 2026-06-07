# Change: 重构 MiniApp 模块边界

## Why
MiniApp 当前存在多个超大文件，尤其是 `App.jsx` 与 `SmartMoneyPage.jsx` 同时承担导航、轮询、业务状态、API 调用、业务计算和大段 JSX 渲染，导致功能互相耦合、修改回归风险高、代码审查困难。

## What Changes
- 将 `App.jsx` 中的纯函数、常量和模块权限判断拆到按职责命名的模块中。
- 将热门池筛选、开仓区间数学、API base 解析等无 UI 逻辑从组件文件中移出。
- 逐步将开仓向导、Smart Money 子页和 API facade 拆成 feature 目录，保留现有用户行为与接口契约。
- 拆分过程中不引入新的运行时依赖，不改变后端 API，不改变用户可见功能。

## Impact
- Affected specs:
  - `miniapp-maintainability`
- Affected code:
  - `miniapp/src/App.jsx`
  - `miniapp/src/lib/*`
  - `miniapp/src/features/*`
  - 后续可能涉及 `miniapp/src/components/SmartMoneyPage.jsx`
