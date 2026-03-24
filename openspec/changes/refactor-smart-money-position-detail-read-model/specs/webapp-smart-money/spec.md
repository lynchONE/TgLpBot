## ADDED Requirements

### Requirement: WebApp 聪明钱仓位支持详情卡片与自动刷新
WebApp SHALL 允许用户在聪明钱面板中点开单个仓位，并展示自动刷新的仓位详情卡片。

详情卡片 MUST：
- 主要展示结构对齐 WebApp 现有仓位卡片
- 展示交易对、状态、区间、当前估值、PnL、token 明细与手续费快照状态
- 按后端返回的 `poll_interval_sec` 自动刷新

#### Scenario: 用户点开 WebApp 聪明钱仓位
- **WHEN** 用户在 WebApp 聪明钱列表中点击某条仓位
- **THEN** 系统 MUST 打开该仓位的详情卡片
- **AND** MUST 用自动轮询方式刷新该卡片内容

#### Scenario: 手续费快照降级展示
- **GIVEN** 某仓位的手续费快照状态为 `stale`、`error` 或 `unavailable`
- **WHEN** WebApp 渲染该仓位详情
- **THEN** 页面 MUST 明确展示手续费状态
- **AND** MUST NOT 误导用户认为手续费数据是实时精确值
