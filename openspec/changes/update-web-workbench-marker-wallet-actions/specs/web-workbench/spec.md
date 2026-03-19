## MODIFIED Requirements

### Requirement: Web workbench 支持聪明钱 K 线覆盖层
Web workbench SHALL 为选中池子提供可选的聪明钱覆盖层，使用户可以在同一张图上查看监控钱包活动，并在 marker tooltip 中直接维护钱包标签。

#### Scenario: 用户在 marker tooltip 中修改已有钱包标签
- **WHEN** 用户点击某个聪明钱 marker 打开 tooltip，并编辑一个已存在 Smart Money 钱包记录的标签
- **THEN** 前端允许用户在 tooltip 内保存新标签
- **AND** 保存成功后，当前 tooltip 与后续 marker 展示更新后的钱包标签

#### Scenario: 用户为未命名钱包保存标签
- **WHEN** 用户点击某个尚未保存标签的钱包 marker，并在 tooltip 中输入并保存标签
- **THEN** 后端按该钱包地址持久化标签
- **AND** 该次保存不会隐式把该钱包加入活跃监控

#### Scenario: tooltip 标签编辑失败
- **WHEN** 用户在 tooltip 中保存钱包标签时请求失败
- **THEN** tooltip 保持打开
- **AND** 前端展示错误提示且不丢失用户当前输入

### Requirement: Web workbench 特别关注钱包线条保持最新
Web workbench SHALL 在当前池子 K 线中仅显示每个特别关注钱包最近一次开仓事件对应的蓝色提示线，避免历史线条堆叠干扰读图。

#### Scenario: 特别关注钱包在窗口内有多次开仓
- **WHEN** 同一特别关注钱包在当前池子和当前 marker 数据窗口内存在多条 `add` 事件
- **THEN** 图表仅渲染该钱包最近一次 `add` 事件对应的蓝色开仓线
- **AND** 更早的蓝色开仓线不再显示

#### Scenario: 取消特别关注后隐藏蓝线
- **WHEN** 用户取消某钱包的特别关注
- **THEN** 该钱包在图上的蓝色开仓线立即隐藏
