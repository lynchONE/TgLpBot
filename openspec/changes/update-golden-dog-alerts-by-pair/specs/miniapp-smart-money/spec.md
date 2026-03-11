## MODIFIED Requirements

### Requirement: Smart Money "金狗通知" tab
MiniApp 的 Smart Money 模块 MUST 提供一个名为 `金狗通知` 的标签页。

该标签页 MUST 允许用户配置以下告警规则：
- `enabled` 开关
- `min_wallets` 阈值（最少活跃 LP 仓位数，沿用现有字段名）
- `window_minutes` 回看窗口
- `cooldown_minutes` 按交易对生效的冷却时间

该标签页 MUST 继续复用 `GlobalConfig` 中已有的 Bark 配置。

#### Scenario: 用户打开金狗通知标签页
- **WHEN** 用户打开 Smart Money 模块并切换到 `金狗通知`
- **THEN** 界面渲染告警配置表单

#### Scenario: 用户保存配置
- **WHEN** 用户更新阈值并保存
- **THEN** 后端持久化该配置，并在后续告警计算中使用

### Requirement: Golden-dog Bark notification
启用后，后端 MUST 在某个交易对于 `window_minutes` 窗口内达到 `min_wallets` 个活跃 LP 仓位时发送 Bark 通知。

该聚合 MUST 满足以下规则：
- 将同链、同一组 token 合约地址对应的多个池子合并为一个交易对
- 允许同一交易对跨不同 fee tier 与支持的 pool version 共同计数
- 活跃 LP 仓位在单个池子内 MUST 按仓位分别统计，不得按钱包地址去重
- 同一钱包如果同时出现在同交易对的多个池子中，必须按多个活跃仓位分别计数
- 交易对归并键 MUST 使用 token 合约地址，而不是 symbol

通知正文 MUST 包含：
- 交易对名称（例如 `TOKEN0/TOKEN1`）
- 活跃 LP 仓位数量
- 明确的关注提示语

#### Scenario: 同交易对跨池子共同触发告警
- **GIVEN** 金狗通知已启用，且 `min_wallets = 3`、`window_minutes = 10`
- **AND** 同一交易对在池子 A 中有 2 个活跃 LP 仓位，在池子 B 中有另外 1 个活跃 LP 仓位
- **WHEN** 后端执行本轮 GoldenDog 扫描
- **THEN** 后端按该交易对聚合后发送 1 条 Bark 通知

#### Scenario: 同一钱包在同一池子持有多个仓位
- **GIVEN** 金狗通知已启用，且某交易对下的同一个池子中同一钱包持有 2 个活跃 LP 仓位
- **WHEN** 后端按交易对聚合该交易对的活跃 LP 仓位数
- **THEN** 这 2 个仓位 MUST 分别计数

#### Scenario: 同一钱包跨不同池子重复参与
- **GIVEN** 金狗通知已启用，且某交易对下池子 A 与池子 B 都有同一个钱包持有活跃 LP 仓位
- **WHEN** 后端按交易对聚合该交易对的活跃 LP 仓位数
- **THEN** 该钱包在池子 A 与池子 B 中的活跃 LP 仓位 MUST 分别计数

#### Scenario: Bark 未配置
- **GIVEN** 金狗通知已启用
- **WHEN** 用户的 `GlobalConfig` 中未启用 Bark 或缺少可解密的 Bark key
- **THEN** 后端 MUST NOT 发送 Bark 通知

## RENAMED Requirements

- FROM: `### Requirement: Per-pool cooldown`
- TO: `### Requirement: Per-pair cooldown`

## MODIFIED Requirements

### Requirement: Per-pair cooldown
后端 MUST 对每个用户、每条链、每个交易对执行冷却去重，避免同一交易对的多个池子在冷却期内重复发送告警。

#### Scenario: 同交易对在冷却期内由另一个池子再次满足阈值
- **GIVEN** 某个交易对已经触发过一次金狗通知
- **WHEN** 同一交易对的另一个池子在 `cooldown_minutes` 内再次满足阈值
- **THEN** 后端 MUST NOT 再发送第二条 Bark 通知
