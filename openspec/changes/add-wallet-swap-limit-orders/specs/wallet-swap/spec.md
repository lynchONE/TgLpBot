## ADDED Requirements
### Requirement: 钱包一键兑换 MUST 支持创建限价单
系统 MUST 允许用户为钱包一键兑换创建限价单，保存链、钱包、卖出代币、买入代币、卖出数量、目标价格或目标到账金额、滑点和 provider 偏好。

#### Scenario: 用户创建目标到账金额限价单
- **WHEN** 用户选择钱包、卖出代币、买入代币、卖出数量、目标到账金额并提交限价单
- **THEN** 系统 MUST 校验该钱包属于当前用户
- **AND** 系统 MUST 保存状态为 `open` 的限价单
- **AND** 系统 MUST 返回订单 ID 和当前订单状态

#### Scenario: 用户创建目标价格限价单
- **WHEN** 用户输入目标价格并提交限价单
- **THEN** 系统 MUST 将目标价格换算为本次卖出数量对应的目标到账金额
- **AND** 系统 MUST 同时保存目标价格和目标到账金额供后续展示与触发判断

### Requirement: 钱包限价单 MUST 在报价达到条件后自动兑换
系统 MUST 由后台 worker 定期检查 `open` 限价单，并在可用报价的净到账金额达到订单目标到账金额后自动执行兑换。

#### Scenario: 报价达到目标后执行兑换
- **GIVEN** 一个状态为 `open` 的限价单
- **WHEN** worker 获取到可用报价且 `net_to_amount >= target_to_amount`
- **THEN** 系统 MUST 将订单原子切换为 `triggering`
- **AND** 系统 MUST 按订单 provider 偏好重新获取可执行交易数据并提交链上交易
- **AND** 成交后系统 MUST 将订单更新为 `filled` 并保存交易哈希、实际到账金额和成交时间

#### Scenario: 报价未达到目标
- **GIVEN** 一个状态为 `open` 的限价单
- **WHEN** worker 获取到可用报价但 `net_to_amount < target_to_amount`
- **THEN** 系统 MUST 保持订单为 `open`
- **AND** 系统 MUST 记录最近检查时间、最近报价 provider 和最近报价到账金额

### Requirement: 钱包限价单 MUST 支持取消和状态查询
系统 MUST 允许用户查询自己的限价单，并取消尚未成交或尚未进入执行中的订单。

#### Scenario: 用户取消未触发订单
- **GIVEN** 一个属于当前用户且状态为 `open` 的限价单
- **WHEN** 用户提交取消请求
- **THEN** 系统 MUST 将订单状态更新为 `cancelled`
- **AND** 后台 worker MUST NOT 再执行该订单

#### Scenario: 用户查看限价单列表
- **WHEN** 用户打开一键兑换限价单列表
- **THEN** 系统 MUST 返回该用户订单的状态、代币、数量、目标条件、最近检查结果、失败原因和成交交易哈希

### Requirement: 钱包限价单 MUST 显式处理失败
系统 MUST 在订单触发后执行失败时把订单更新为 `failed`，并保存可展示的失败原因；系统 MUST NOT 用默认成功或安全默认值掩盖失败。

#### Scenario: 触发后兑换失败
- **GIVEN** 一个限价单已切换为 `triggering`
- **WHEN** 重新取价、私钥读取、approve、发交易或等待交易确认失败
- **THEN** 系统 MUST 将订单状态更新为 `failed`
- **AND** 系统 MUST 保存失败原因和失败时间
- **AND** 系统 MUST NOT 记录成功交易哈希
