## ADDED Requirements
### Requirement: AutoLP 跳过开仓通知
当 AutoLP 识别到候选池但因任意前置条件或过滤规则未开仓时，系统 SHALL 向用户通知并说明原因。

#### Scenario: 候选池因条件不满足被跳过
- **WHEN** 选中候选池
- **AND** 自动开仓流程因任意原因被跳过
- **THEN** 用户收到包含跳过原因的通知

### Requirement: AutoLP 跳过通知频控
系统 SHALL 对“用户+池子”的跳过通知进行频控，5 分钟内最多发送一次。

#### Scenario: 冷却窗口内重复跳过
- **WHEN** 同一用户与同一池子在 5 分钟内多次被跳过
- **THEN** 该窗口内仅发送一次通知
