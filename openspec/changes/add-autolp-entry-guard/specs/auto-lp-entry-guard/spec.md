## ADDED Requirements

### Requirement: AutoLP Flash Pump/Dump 开仓门禁
当 AutoLP 评估到候选池准备自动开仓时，系统 SHALL 在开仓前执行 Flash Pump/Dump 门禁；若命中门禁条件，系统 SHALL 跳过或延迟本次自动开仓并给出原因。

#### Scenario: 候选池处于短时冲高而被延迟/跳过
- **WHEN** 某池子被选为候选池，且准备进入自动开仓流程
- **AND** 当前 spot 价格相对短窗 TWAP 偏离度命中阈值
- **THEN** 系统跳过或延迟自动开仓
- **AND** 系统记录/返回可读的跳过原因（用于通知或日志）

### Requirement: Spot vs TWAP 偏离度判断
系统 SHALL 支持使用链上 spot 与短窗 TWAP 的偏离度（仅向上偏离）来识别“短时冲高”风险，并基于配置阈值决定是否允许开仓。

#### Scenario: spot 明显高于 TWAP 时阻止开仓
- **WHEN** 候选池准备自动开仓
- **AND** `spotOverTwapPct >= spot_over_twap_max_pct`
- **THEN** 系统阻止当次自动开仓并说明原因

### Requirement: 冲顶回落后的长冷却（可配置）
当系统判定候选池发生“短时冲高后快速回落”时，系统 SHALL 对该池子设置更长的冷却时间，避免短时间内再次开仓。

#### Scenario: 冲顶回落触发长冷却
- **WHEN** 某池子在观察期内出现显著回撤并满足“冲顶回落”条件
- **THEN** 系统对该池子设置长冷却
- **AND** 在长冷却期间不对该池子自动开仓
