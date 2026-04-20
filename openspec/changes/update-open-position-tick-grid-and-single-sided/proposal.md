# Change: 更新开仓 Tick/格子编辑器并补充价格区间与单边快捷模式

## Why
- 现有变更已经覆盖按百分比、Tick、格子编辑区间，以及单边/双边结果回显，但仍缺少用户最直观的按价格输入区间能力，例如直接输入 `0.05 - 0.07`。
- 用户需要更明确的“当前将开单边仓还是双边仓”提示，以及一键把当前区间整体推到当前价下方或上方的快捷操作，降低单边仓位的操作门槛。
- 后端已经支持按规范化后的 `tick_lower` / `tick_upper` 预览和执行，再新增一个后端 `price` 输入模式收益不高，反而会扩张协议面。

## What Changes
- 保持后端开仓协议不变：后端继续支持 `percentage`、`tick`、`grid` 三种执行输入，不新增后端 `price` 模式。
- WebApp 与 MiniApp 新增前端“价格区间”编辑层，用户输入价格上下限后，由前端映射成 Tick 区间，并复用现有 `tick` 预览/提交链路。
- 在 Tick/格子/价格编辑层上补充 `单边下限` / `单边上限` 快捷按钮，将当前选中宽度整体移动到当前价下方或上方。
- 在提交前摘要中更显式地展示“当前将开单边仓 / 双边仓”、主偏向资产、规范化后的 Tick 区间和价格区间。

## Impact
- Affected specs:
  - `open-position-grid-range`
  - `open-position-visual-editor`
- Affected code:
  - WebApp: `webapp/src/components/OpenPositionModal.jsx`
  - MiniApp: `miniapp/src/App.jsx`
  - 共用可视化组件：流动性分布图相关组件
- Compatibility:
  - 现有按百分比、按 Tick、按格子的预览与开仓请求继续可用
  - 新增价格区间仅是前端编辑能力，不要求后端新增字段
