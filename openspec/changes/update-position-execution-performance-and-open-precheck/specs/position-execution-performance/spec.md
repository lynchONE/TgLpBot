## ADDED Requirements

### Requirement: 开仓与撤仓执行 MUST 将非关键持久化移出关键路径
系统 MUST 将交易记录、交易历史、补账和非关键通知从开仓/撤仓关键路径中解耦，避免在链上结果已明确成功后继续阻塞接口返回。

#### Scenario: 开仓链上成功后异步补写记录
- **GIVEN** 开仓所需的链上交易已经确认成功
- **AND** 任务主状态已经更新为可运行
- **WHEN** 系统补写 trade record、transaction history 或通知
- **THEN** 系统 MUST 不因这些非关键写入阻塞 `open_position` 成功响应
- **AND** MUST 在后台补写或重试这些记录
- **AND** MUST 记录补写失败日志

#### Scenario: 撤仓链上成功后记录补写失败
- **GIVEN** 撤仓和扫尾换回稳定币的链上交易已经确认成功
- **WHEN** 交易历史或收益记录补写暂时失败
- **THEN** 系统 MUST 保持撤仓结果为成功
- **AND** MUST 将失败的记录写入后台重试链路
- **AND** MUST NOT 因补写失败把用户已完成的撤仓误报为失败

### Requirement: BSC 开仓与撤仓 MUST 先走快路径同步
系统 MUST 先采用短轮询快路径确认 receipt、allowance 和关键余额变化，仅在快路径未命中时再退回保守重试，而不是无条件执行固定等待。

#### Scenario: approve 后 allowance 首次读取滞后
- **GIVEN** approve 交易已经成功上链
- **AND** 首次 allowance 读取仍小于目标值
- **WHEN** 系统继续准备开仓
- **THEN** 系统 MUST 先执行短间隔轮询重查 allowance
- **AND** MUST NOT 无条件固定等待数秒
- **AND** 仅在短轮询窗口耗尽后 MAY 进入较长的保守重试

#### Scenario: 撤仓后的 receipt 与余额同步走快路径
- **GIVEN** 撤仓交易已广播且 BSC 链正常出块
- **WHEN** 系统等待 receipt 和目标 token 余额变化
- **THEN** 系统 MUST 先在短窗口内高频轮询
- **AND** MUST 在快路径失败时回退到现有保守同步策略
- **AND** MUST 保证最终仍以 receipt 或关键余额条件判定成功

### Requirement: 撤仓失败重试 MUST 采用梯度退避
系统 MUST 在可重试的撤仓失败场景中按固定梯度推进重试间隔，优先快速补救，再逐步拉长等待时间。

#### Scenario: 连续撤仓失败时按固定梯度推进
- **GIVEN** 某个撤仓任务发生了可重试失败
- **WHEN** 系统安排后续自动重试
- **THEN** 系统 MUST 按 `500ms -> 1s -> 2s -> 3s -> 5s -> 10s -> 30s` 的顺序推进重试间隔
- **AND** MUST 基于失败次数选择当前档位
- **AND** 在超过最后一档后 MUST 继续使用 `30s` 作为后续重试间隔

#### Scenario: 不可重试失败不进入梯度退避
- **GIVEN** 撤仓失败原因为参数错误、权限错误或其他明确不可重试问题
- **WHEN** 系统处理失败结果
- **THEN** 系统 MAY 直接终止任务或转人工处理
- **AND** MUST NOT 为这类失败强制进入梯度退避重试

### Requirement: 执行链路 MUST 复用已获得的链上结果与上下文
系统 MUST 复用 prepare、preview 或执行阶段已经获取的池子元数据、receipt 和余额快照，避免同一请求链路内重复发起等价 RPC。

#### Scenario: 撤仓统计复用已有 receipt
- **GIVEN** 撤仓流程已经获取某笔交易的 receipt
- **WHEN** 系统继续计算 gas、USDT delta 和记录补写内容
- **THEN** 系统 MUST 复用已有 receipt 结果
- **AND** MUST NOT 为同一用途重复查询同一笔 receipt

#### Scenario: 开仓快照在关键路径采集但异步持久化
- **GIVEN** 开仓流程需要记录 stable balance snapshot
- **WHEN** 链上关键步骤已经完成
- **THEN** 系统 MUST 在关键路径内采集必要快照
- **AND** MUST 允许将快照写入 trade record 的动作放入异步补写链路
