## Context
- 现有 `webapp/src/components/KlineChart.jsx` 已经通过 `lightweight-charts` + DOM overlay 渲染蜡烛图、成交量、聪明钱头像 marker、区间虚线和 tooltip。
- 用户新增的两项需求都属于图表前端交互层：
  - 绘图测量工具
  - marker 钱包筛选
- 后端现有 `smart_money_pool_markers` 返回的数据已经包含 `wallet_address`、`wallet_label`、`estimated_usd` 等字段，足以支撑前端过滤，不需要新增接口。

## Goals / Non-Goals
- Goals:
  - 在不引入新后端接口的前提下，为 Web Workbench K 线增加可用的价格涨跌幅测量能力。
  - 让用户能在图表左上角快速筛选需要展示的钱包气泡。
  - 不破坏现有 marker tooltip、选中聪明钱高亮、特别关注虚线等功能。
- Non-Goals:
  - 不实现多条测量线持久化、序列化保存、跨池子恢复。
  - 不实现专业绘图套件中的无限对象管理、锁定、吸附、样式编辑。
  - 不把钱包筛选下沉到后端查询层。

## Decisions
- Decision: 测量工具完全在 `KlineChart.jsx` 内部完成。
  - 通过已有图表坐标换算能力，把鼠标落点映射到时间与价格。
  - 工具栏状态由 `App.jsx` 持有并传入 `KlineChart`，图形草绘和渲染由 `KlineChart.jsx` 负责。
- Decision: V1 只维护一个当前测量对象。
  - 直线工具：第一次点击记录起点，第二次点击记录终点并生成测量结果。
  - 矩形工具：按下开始、拖拽更新、抬起完成，并显示矩形与涨跌幅标签。
  - 切换工具或点击清除时直接替换当前对象，避免复杂的对象列表管理。
- Decision: 钱包筛选状态在 `App.jsx` 持有。
  - 从当前 `klineMarkers` 派生候选钱包列表和金额范围。
  - 先在 `App.jsx` 过滤 marker，再把过滤后的 marker 传给 `KlineChart`。
  - 这样可复用现有 marker 聚合、tooltip、抽屉逻辑，避免在图表组件内再复制一套过滤规则。
- Decision: 左上角下拉只筛选“聪明钱气泡”，不影响蜡烛、成交量、GMGN 跳转和池子选择。

## Risks / Trade-offs
- 绘图层需要占用指针事件，若处理不当会影响现有 marker hover 与 tooltip 点击。
  - Mitigation: 仅在工具激活时启用绘图捕获；普通浏览模式保持现有交互优先级。
- marker 候选钱包来自当前数据窗口，筛选项会随时间窗变化。
  - Mitigation: 在下拉面板中明确“基于当前已加载气泡”的范围，并在候选项变化时自动剔除失效勾选。
- 金额阈值与钱包勾选组合后可能返回空结果。
  - Mitigation: 在图表摘要或筛选面板中提示当前过滤为空，并保留一键重置。

## Migration Plan
1. 在 `App.jsx` 增加 K 线工具状态和钱包筛选状态。
2. 在 `App.jsx` 基于 `klineMarkers` 派生候选钱包和过滤结果。
3. 扩展 `KlineChart.jsx`，加入工具栏 props、绘图层、坐标转换与测量标签。
4. 调整 `styles.css`，补充绘图层、工具按钮、筛选下拉样式。
5. 运行 `webapp` 构建验证。

## Open Questions
- 直线测量标签是否同时显示绝对价格差与百分比；当前提案先以百分比为必选，绝对价差可作为附加信息。
- 矩形测量的涨跌幅是否以拖拽起点/终点价格计算；当前提案按矩形上下边价格计算。
