## ADDED Requirements

### Requirement: MiniApp 聪明钱仓位支持详情卡片与自动刷新
MiniApp SHALL 允许用户点开聪明钱仓位，并展示接近实时仓位卡片的详情视图。

详情视图 MUST：
- 使用与 MiniApp 实时 `PositionCard` 一致的主要信息结构
- 展示区间、当前 tick、是否在区间内、持仓金额、当前估值与手续费快照状态
- 按后端返回的 `poll_interval_sec` 自动刷新

#### Scenario: 用户点开聪明钱仓位
- **WHEN** 用户在 MiniApp 的聪明钱页面点击某一条仓位
- **THEN** 客户端 MUST 请求仓位详情接口
- **AND** MUST 以实时仓位卡片风格渲染详情

#### Scenario: 详情自动刷新
- **GIVEN** 用户已打开某个聪明钱仓位详情
- **WHEN** 轮询间隔到达
- **THEN** 客户端 MUST 再次请求详情接口
- **AND** 仅刷新当前打开的详情视图

### Requirement: MiniApp 仓位详情接口返回卡片化字段
MiniApp 使用的聪明钱仓位详情接口 MUST 返回可直接驱动卡片渲染的字段结构。

响应 MUST 至少包含：
- `position_ref`
- `title`
- `status_label`
- `current_tick`
- `tick_lower`
- `tick_upper`
- `in_range`
- `token_rows`
- `totals`
- `current_value_usd`
- `absolute_pnl_usd`
- `has_pnl`
- `fee_status`
- `fee_updated_at`
- `poll_interval_sec`

#### Scenario: 返回卡片详情数据
- **WHEN** 客户端请求某个聪明钱仓位详情
- **THEN** 接口 MUST 返回上述卡片化字段
- **AND** 客户端不需要再自行拼装持仓 token 行和总计区块
