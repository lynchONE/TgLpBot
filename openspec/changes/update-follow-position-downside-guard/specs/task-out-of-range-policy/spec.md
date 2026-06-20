## ADDED Requirements
### Requirement: 跟单仓位下破区间必须保底撤出
系统 MUST 对 `is_follow=true` 的自动跟单仓位执行下破保底策略：当当前价格低于该仓位区间下沿时，系统 MUST 立即撤出该跟单仓位并停止任务。该保底策略 MUST 不依赖目标钱包撤仓事件是否被捕获。

#### Scenario: 跟单仓位价格下破区间
- **GIVEN** 一个 `is_follow=true` 且仍有流动性的跟单任务
- **WHEN** 策略轮询发现当前价格低于该任务区间下沿
- **THEN** 系统 MUST 提交撤出流动性并兑换为 USDT 的执行
- **AND** MUST 在撤出完成后停止该任务

#### Scenario: 跟单仓位价格上破区间
- **GIVEN** 一个 `is_follow=true` 且仍有流动性的跟单任务
- **WHEN** 策略轮询发现当前价格高于该任务区间上沿
- **THEN** 系统 MUST NOT 因上破区间自动撤出该跟单仓位
- **AND** MUST NOT 因上破区间自动再平衡该跟单仓位

#### Scenario: 普通仓位出区间
- **GIVEN** 一个 `is_follow=false` 的普通任务
- **WHEN** 策略轮询发现该任务出区间
- **THEN** 系统 MUST 继续按该任务已有区间策略处理
- **AND** MUST NOT 套用跟单仓位的下破保底规则
