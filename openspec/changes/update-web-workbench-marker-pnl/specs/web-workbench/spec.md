## ADDED Requirements

### Requirement: Web workbench 撤仓气泡展示估算盈亏
Web workbench SHALL 在聪明钱 K 线覆盖层的减仓事件详情中展示该次撤仓对应的估算成本和估算已实现盈亏。

#### Scenario: 减仓事件具备可回放的建仓历史
- **WHEN** 用户打开某个减仓气泡详情，且后端能够根据同钱包同仓位历史事件回放出该次撤仓的成本
- **THEN** 详情面板展示“估算成本”和“估算盈亏”
- **AND** 盈亏金额按正负样式区分
- **AND** 文案明确该值为估算结果

#### Scenario: 减仓事件缺少完整建仓历史
- **WHEN** 用户打开某个减仓气泡详情，但后端无法为该次撤仓回放出完整成本
- **THEN** 详情面板不显示误导性的盈亏数值
- **AND** 仍可显示该事件的基础金额、时间和交易链接

### Requirement: Web workbench 撤仓气泡展示撤仓时图表价格
Web workbench SHALL 在减仓气泡详情中展示与该事件时间最近的当前图表 K 线价格。

#### Scenario: 当前图表存在匹配 candle
- **WHEN** 用户打开减仓气泡详情，且当前图表可找到与事件时间最近的 candle
- **THEN** 详情面板展示该价格
- **AND** 价格标签应表明其来自当前图表时间点

#### Scenario: 当前图表没有匹配 candle
- **WHEN** 用户打开减仓气泡详情，但当前图表没有可用 candle
- **THEN** 详情面板不展示价格字段
- **AND** 其他事件详情仍正常展示
