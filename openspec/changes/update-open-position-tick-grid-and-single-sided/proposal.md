# Change: 开仓支持按 Tick/格子选区间并支持单边池交互

## Why
- 当前开仓流程只支持输入上下百分比区间，虽然底层最终也会换算到 tick，但前端无法直接按 tick 或格子精确选区间，精度和可控性都不够。
- 用户希望像主流 CLMM 产品一样，直接围绕当前价选择离散格子区间，并且能一眼看出区间落点、单边/双边形态和资金分布，而不是只看两个百分比输入框。
- 当前系统从 USDT 一键开仓，缺少“单边池”这一用户心智的明确反馈。用户需要知道自己当前选中的区间是否会形成单边仓位，以及预估会偏向哪一侧资产。
- WebApp 与 MiniApp 现在的开仓交互割裂，缺少统一的产品结构，不利于后续继续扩展按 tick、按格子、自动区间等高级交互。

## What Changes
- 扩展开仓接口与预览接口，支持三种区间输入方式：百分比、直接 tick、围绕当前价的格子数量。
- 扩展开仓预览返回值，增加当前 tick、tick spacing、规范化后的 tick 下上界、格子数量、价格边界、单边/双边形态、预估资产占比和可视化所需的区间分布数据。
- 重做 WebApp 与 MiniApp 的开仓交互，参考用户提供的竞品结构，采用“左侧区间编辑 / 右侧资金与执行参数”的信息架构，并在移动端折叠为分步卡片式布局。
- 在开仓界面中明确支持“单边池”心智：当区间完全位于当前价一侧时，界面与预览都要明确展示这是单边仓位，并展示预估偏向的资产侧。
- 保留现有百分比模式兼容性，旧客户端继续传 `range_lower_pct` / `range_upper_pct` 时不受影响。

## Impact
- Affected specs:
  - `open-position-grid-range`
  - `open-position-visual-editor`
- Affected code:
  - Backend: `backend/service/web_server/open_position.go`, `backend/service/web_server/open_position_prepare.go`, `backend/service/liquidity/*`, `backend/service/pricing/*`
  - WebApp: `webapp/src/components/OpenPositionModal.jsx`, `webapp/src/api.js`, 相关样式与可视化组件
  - MiniApp: `miniapp/src/App.jsx`, `miniapp/src/lib/api.js`, 相关卡片与可视化组件
- Compatibility:
  - 现有百分比开仓请求继续可用
  - 新交互优先消费增强后的 prepare/preview 数据；若后端未升级，前端不得误导用户进入 tick/格子模式
