## Context
MiniApp 基于 React 18 + Vite + TailwindCSS。当前文件体积热点为：

- `miniapp/src/App.jsx`：约 5.9k 行，包含全局壳、实时仓位、热门池、开仓向导、任务操作和部分管理逻辑。
- `miniapp/src/components/SmartMoneyPage.jsx`：约 5.1k 行，包含 Smart Money 多个页面、设置、弹窗和通用小组件。
- `miniapp/src/lib/api.js`：导出数量较多，多个业务域的 API 混在一个文件。

## Goals / Non-Goals
- Goals:
  - 降低单文件体积和组件作用域复杂度。
  - 让无 UI 业务计算可以独立阅读、复用和测试。
  - 保持当前 MiniApp 行为、路由、接口调用和视觉表现不变。
  - 让后续功能迭代优先落在 feature 目录，而不是继续堆到 `App.jsx`。
- Non-Goals:
  - 不在本次重构中改后端接口。
  - 不引入 TypeScript 或新的状态管理库。
  - 不把所有大文件一次性拆完，避免超大 diff 带来回归风险。

## Decisions
- Decision: 先拆纯函数和常量，再拆组件。
  - 纯函数和常量移动的行为风险最低，适合作为第一批改动。
  - 组件拆分按功能域进行，优先拆出开仓向导和 Smart Money 子页。

- Decision: 使用 `miniapp/src/features/<domain>/` 承载业务域代码。
  - `features/openPosition/` 放开仓区间、DCA、预览和开仓 Sheet。
  - `features/hotPools/` 放热门池筛选和排序逻辑。
  - `features/appShell/` 放模块访问、导航和应用壳相关逻辑。
  - `lib/` 保留通用工具和 API 基础设施。

- Decision: API facade 分阶段拆分。
  - 第一阶段保留 `lib/api.js` 的现有导出，避免一次性修改所有调用方。
  - 后续按 `tasks`、`openPosition`、`admin`、`assets`、`walletSwap` 等域拆出内部模块，再由 `api.js` 转发。

## Risks / Trade-offs
- 大量移动代码容易产生 import 漏改。
  - Mitigation: 每批改动后运行 `npm run build`，并做针对性 diff 检查。
- 开仓向导状态较多，直接拆组件可能导致闭包依赖遗漏。
  - Mitigation: 先抽纯计算，再抽 hook，最后抽 UI Sheet。
- Smart Money 文件存在多套页面和本地小组件。
  - Mitigation: 先按页面边界移动，不重写内部逻辑。

## Migration Plan
1. 新增 OpenSpec 变更记录。
2. 从 `App.jsx` 移出 API base、模块权限、热门池筛选、开仓数学和格式化工具。
3. 将开仓向导状态与提交流程拆成 hook，再拆出 Sheet 组件。
4. 将 `SmartMoneyPage.jsx` 按页面拆成 feature 子目录。
5. 将 `lib/api.js` 分域拆分并保留兼容导出。
6. 每阶段执行 `cd miniapp && npm run build` 并检查 diff。

## Open Questions
- API facade 是否需要在同一轮完成完整分域，还是等后续功能改动顺手迁移调用方。
- Smart Money 子页拆分时是否同时修复文件中已有的乱码文案，还是单独开一个文案修复变更。
