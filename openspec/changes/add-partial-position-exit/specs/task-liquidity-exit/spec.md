## ADDED Requirements
### Requirement: 用户 MUST 能按百分比撤出任务仓位
系统 MUST 允许用户对仍有链上 liquidity 的任务选择撤出百分比，百分比范围为大于 0 且小于等于 100。未显式指定百分比的撤仓请求 MUST 按 100% 处理，以保持现有停止任务行为不变。

#### Scenario: 未指定百分比时保持全撤
- **WHEN** 用户通过现有停止任务入口提交撤仓且请求中没有撤仓百分比
- **THEN** 系统按 100% 撤出该任务当前链上 liquidity
- **AND** 任务按现有停止任务流程进入停止或重试状态

#### Scenario: 指定部分百分比撤仓
- **WHEN** 用户对运行中的任务提交 25% 撤仓请求
- **THEN** 系统读取该任务链上当前 liquidity
- **AND** 按当前 liquidity 的 25% 构造本次 V3/V4 减少流动性交易
- **AND** 将本次撤出的非稳定币资产兑换为稳定币
- **AND** 交易成功后任务保留剩余 liquidity 并继续可管理

#### Scenario: 百分比非法时拒绝
- **WHEN** 用户提交 0、负数、大于 100 或无法解析的撤仓百分比
- **THEN** 系统 MUST 拒绝请求并返回明确错误
- **AND** 系统 MUST NOT 广播链上撤仓交易

### Requirement: 部分撤仓 MUST 支持 WebApp、MiniApp 与 Telegram 文字版
系统 MUST 在 WebApp、MiniApp 和 Telegram 文字版任务管理入口提供部分撤仓能力，并复用同一后端校验和链上执行逻辑。

#### Scenario: WebApp 或 MiniApp 部分撤仓
- **WHEN** 用户在 WebApp 或 MiniApp 的任务操作中选择部分撤仓并输入百分比
- **THEN** 前端 MUST 将任务 ID 与百分比提交到后端
- **AND** 后端 MUST 只撤出指定百分比对应的 liquidity
- **AND** 后端 MUST 将本次撤出的资产兑换为稳定币

#### Scenario: Telegram 文字版部分撤仓
- **WHEN** 用户在 Telegram 任务卡选择部分撤仓并输入百分比
- **THEN** Bot MUST 校验百分比并提交同一后端业务逻辑
- **AND** 后端 MUST 将本次撤出的资产兑换为稳定币
- **AND** 撤仓完成或失败后 MUST 通过消息反馈交易状态

### Requirement: 部分撤仓后任务状态 MUST 保留剩余仓位
当撤出百分比小于 100 且链上交易成功时，系统 MUST 更新任务剩余 `current_liquidity`，不得把任务标记为 `stopped`，不得清空 tokenId，也不得触发再入场流程。

#### Scenario: 部分撤仓成功后继续运行
- **WHEN** 任务当前 liquidity 为 100000，用户撤出 40% 且链上交易成功
- **THEN** 任务 `current_liquidity` MUST 更新为链上剩余 liquidity
- **AND** 任务状态 MUST 保持 `running` 或原有可管理状态
- **AND** 任务的 V3/V4 tokenId MUST 保留

#### Scenario: 100% 撤仓仍停止任务
- **WHEN** 用户选择撤出 100%
- **THEN** 系统 MUST 沿用现有全撤逻辑
- **AND** 撤仓完成后任务 MUST 按现有规则停止或进入后续再平衡/换仓流程
