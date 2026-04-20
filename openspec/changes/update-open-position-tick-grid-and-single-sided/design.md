## Context
当前开仓链路在前端只暴露百分比区间输入，但后端实际会把百分比换算成 tick 再执行。用户给出的参考图体现的是更成熟的 CLMM 交互：区间选择不再是两个抽象数字，而是围绕当前价的离散格子、分布图、自动/手动切换、单边/双边结果反馈和右侧摘要面板。

本项目还有一个现实约束：当前开仓入口是从 USDT 一键开仓，不是直接让用户输入 token0/token1 两边数量。因此这里不能简单照搬竞品的“双输入金额表单”，而应该保留“开仓金额 = USDT”的主入口，同时在预览区明确展示最终会形成的资产配比与单边倾向。

## Goals / Non-Goals
- Goals:
  - 支持按百分比、按 tick、按格子三种方式定义开仓区间。
  - 让用户能围绕当前价直观看到区间所在格子、区间宽度、是否单边、预估资产配比。
  - WebApp 与 MiniApp 共用同一套区间数据协议和交互语义。
  - 兼容旧的百分比接口，允许渐进迁移。
- Non-Goals:
  - 本次不重做 Bot 文本开仓交互。
  - 本次不要求开仓入口改成用户手填 token0/token1 双币金额。
  - 本次不引入完整 K 线图编辑器；优先做轻量的离散格子区间编辑器。

## Decisions
- Decision: 新增统一的区间输入协议
  - 开仓请求新增 `range_input_mode`，取值为 `percentage`、`tick`、`grid`。
  - `percentage` 模式继续使用现有 `range_lower_pct` / `range_upper_pct`。
  - `tick` 模式使用 `tick_lower` / `tick_upper`。
  - `grid` 模式使用 `grid_left_count` / `grid_right_count`，表示相对当前 tick 左右各选多少个 spacing。
  - 后端始终返回规范化后的 `tick_lower` / `tick_upper`，作为最终执行基准。
  - Alternatives considered: 只保留 `tick` 一种新模式。
  - Rationale: 只给 tick 仍然不够产品化；格子模式更接近用户截图里的心智，百分比模式则保证兼容和新手可用。

- Decision: 预览接口返回区间编辑器所需的几何与分布数据
  - `prepare` 或 `preview` 需要返回 `current_tick`、`tick_spacing`、`price_lower`、`price_upper`、`normalized_grid_left_count`、`normalized_grid_right_count`、`position_shape`。
  - `position_shape` 至少区分 `single_token0`、`single_token1`、`dual_sided`。
  - 增加可视化分布数据，例如围绕当前价附近的离散 bins / bars，供前端画简化版流动性分布条。
  - Alternatives considered: 前端自行根据 tick spacing 伪造格子。
  - Rationale: 分布、规范化结果和单边形态都依赖后端真实池子状态，前端伪造会误导用户。

- Decision: 保留“USDT 开仓金额”主入口，用预估配比表达单边池结果
  - 界面仍然以 USDT 金额为主输入，不强制用户输入 token0/token1 两边金额。
  - 当区间完全位于当前价一侧时，预览必须明确展示“该仓位为单边池”以及“预计主要持有 Token0 / Token1”。
  - 当区间跨越当前价时，预览展示双边资产占比。
  - Alternatives considered: 直接把竞品的双币金额输入复制到开仓弹窗。
  - Rationale: 当前后端开仓模型并不是从双币余额直加流动性，直接复制会造成产品与执行链路不一致。

- Decision: 交互结构参考竞品，但按双端做响应式拆分
  - WebApp 采用桌面双栏结构：左侧是区间编辑器，右侧是钱包、金额、执行参数和摘要。
  - MiniApp 采用自上而下分步卡片：池子信息、区间模式、格子编辑器、金额与参数、摘要与确认。
  - `自动 / 手动` 切换保留：自动模式用于快捷区间和智能推荐；手动模式允许百分比、tick、格子精确编辑。
  - Alternatives considered: 两端共用完全相同的布局。
  - Rationale: 竞品是典型桌面模态布局，MiniApp 直接照搬会导致触控区域拥挤、滚动过长。

## Risks / Trade-offs
- 风险: 预览接口返回更多池子上下文，会增加一次预览请求的负担。
  - Mitigation: 将重数据放在 `prepare` 里缓存，`preview` 只返回与金额和区间变化强相关的数据。

- 风险: “格子模式”依赖当前 tick，市场快速波动时用户界面看到的中心格可能变化。
  - Mitigation: 预览返回规范化后的 tick 下上界和最新中心 tick，前端在提交前以预览回显为准。

- 风险: 用户可能把“单边池”理解成输入单边资产，而当前系统仍是 USDT 开仓。
  - Mitigation: 文案明确表述为“从 USDT 开仓，系统将自动换成单边所需资产”，并在摘要区展示预估结果。

## Migration Plan
1. 后端先支持新字段与增强预览，同时保留旧百分比字段。
2. WebApp 与 MiniApp 在后端能力可用时显示 tick/格子模式，否则退回百分比模式。
3. 完成双端切换后，再逐步清理只适用于旧百分比模型的冗余 UI。

## Open Questions
- 是否需要在后续任务中把“编辑任务区间”也升级到同样的 tick/格子模型，而不仅是新开仓。
- 是否需要给高级用户暴露“直接输入 tickLower/tickUpper”，还是只在 WebApp 中提供，高频用户再打开。
