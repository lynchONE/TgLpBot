## ADDED Requirements
### Requirement: 开仓 MUST 支持兑换渠道策略选择
系统 MUST 在 WebApp 与 MiniApp 的开仓页面提供兑换渠道策略选择。策略 MUST 支持 `best`、`okx`、`binance`，默认值 MUST 为 `best`。

#### Scenario: 用户使用默认自动择优
- **GIVEN** 用户打开 WebApp 或 MiniApp 开仓页面
- **WHEN** 用户未手动修改兑换渠道策略
- **THEN** 页面 MUST 默认选择 `best`
- **AND** 后端 MUST 在开仓预览和执行中允许 OKX 与 Binance 同时参与报价与择优

#### Scenario: 用户选择仅 OKX
- **GIVEN** 用户在开仓页面选择 `仅 OKX`
- **WHEN** 页面请求开仓预览或提交开仓
- **THEN** 请求 MUST 携带 `swap_provider_policy=okx`
- **AND** 后端 MUST 只查询和执行 OKX
- **AND** 若 OKX 不可用，MUST 返回明确错误且 MUST NOT 自动切换到 Binance

#### Scenario: 用户选择仅 Binance
- **GIVEN** 用户在开仓页面选择 `仅 Binance`
- **WHEN** 页面请求开仓预览或提交开仓
- **THEN** 请求 MUST 携带 `swap_provider_policy=binance`
- **AND** 后端 MUST 只查询和执行 Binance
- **AND** 若 Binance 不可用，MUST 返回明确错误且 MUST NOT 自动切换到 OKX

### Requirement: Provider policy MUST 随任务保存并用于撤仓
系统 MUST 将开仓时选择的兑换渠道策略保存到任务上。撤仓、部分撤仓和清仓 dust 换回稳定币时，系统 MUST 默认使用该任务保存的策略。

#### Scenario: 使用开仓时保存的策略撤仓
- **GIVEN** 某任务创建时保存了 `swap_provider_policy=binance`
- **WHEN** 用户对该任务执行撤仓、部分撤仓或清仓 dust
- **THEN** 后端 MUST 只使用 Binance 查询和执行清仓兑换
- **AND** MUST NOT 自动切换到 OKX

#### Scenario: 旧任务没有保存策略
- **GIVEN** 某历史任务没有 `swap_provider_policy`
- **WHEN** 用户执行撤仓、部分撤仓或清仓 dust
- **THEN** 后端 MUST 按显式默认策略 `best` 处理
- **AND** MUST 在日志中能看出这是历史任务默认策略

### Requirement: 开仓撤仓兑换 MUST 支持 OKX 与 Binance 择优
在 `best` 策略下，系统 MUST 在开仓 entry swap、Zap 内部配比 swap、撤仓/清仓换回稳定币的兑换中同时获取 OKX 与 Binance 的可执行报价，并选择预计到账最高的可执行结果。

#### Scenario: Binance 报价优于 OKX
- **GIVEN** 本次开仓或撤仓兑换需要执行 token-to-token swap
- **AND** provider policy 为 `best`
- **AND** OKX 与 Binance 都返回可执行报价
- **AND** Binance 的预计到账 raw amount 大于 OKX
- **WHEN** 系统执行该兑换
- **THEN** 系统 MUST 使用 Binance 构造并发送兑换交易或 Zap swap calldata
- **AND** MUST 记录最终 provider 为 `binance`

#### Scenario: OKX 报价优于 Binance
- **GIVEN** 本次开仓或撤仓兑换需要执行 token-to-token swap
- **AND** provider policy 为 `best`
- **AND** OKX 与 Binance 都返回可执行报价
- **AND** OKX 的预计到账 raw amount 大于或等于 Binance
- **WHEN** 系统执行该兑换
- **THEN** 系统 MUST 使用 OKX 构造并发送兑换交易或 Zap swap calldata
- **AND** MUST 记录最终 provider 为 `okx`

#### Scenario: 双 provider 都不可用
- **GIVEN** 本次兑换需要执行 token-to-token swap
- **AND** provider policy 为 `best`
- **AND** OKX 与 Binance 都报价失败或不可执行
- **WHEN** 系统尝试执行该兑换
- **THEN** 系统 MUST 返回明确错误
- **AND** MUST NOT 返回伪造成功、零值到账或静默跳过该兑换

### Requirement: Zap 合约 MUST 支持 Binance 受信任路由
Zap 合约 MUST 支持通过受 owner 管理的 allowlist 信任 OKX 与 Binance 的 swap target 和 approve target。Zap 内部 swap MUST 只调用受信任 target，并只授权给受信任 approve target。

#### Scenario: Zap 执行 Binance 内部 swap
- **GIVEN** owner 已将 Binance swap target 与 approve target 加入 Zap allowlist
- **AND** 后端基于 Binance 报价构造了 `SwapParams`
- **WHEN** Zap 执行开仓或补仓中的内部配比 swap
- **THEN** 合约 MUST 允许该 Binance target 与 approve target
- **AND** MUST 校验 `tx.value` 为零
- **AND** MUST 继续执行 minOut 与余额 delta 保护

#### Scenario: Zap 拒绝不可信 target
- **GIVEN** 某 `SwapParams.target` 未在 Zap allowlist 中
- **WHEN** 后端或攻击者尝试通过 Zap 执行该 swap
- **THEN** 合约 MUST revert
- **AND** MUST NOT approve 或 call 该 target

#### Scenario: Zap 拒绝不可信 approve target
- **GIVEN** 某 `SwapParams.approveTarget` 未在 Zap allowlist 中
- **WHEN** 后端或攻击者尝试通过 Zap 执行该 swap
- **THEN** 合约 MUST revert
- **AND** MUST NOT 授权该 approve target 花费用户资金

### Requirement: 开仓预览和执行反馈 MUST 展示 provider 与 route
WebApp 与 MiniApp MUST 在开仓预览、开仓确认、开仓执行结果、撤仓执行结果中展示预计或实际使用的 provider 与 route。

#### Scenario: 开仓预览展示预计 provider 和 route
- **GIVEN** 本次开仓需要 entry swap 或 Zap 内部配比 swap
- **WHEN** 页面展示开仓预览
- **THEN** 页面 MUST 展示当前预计使用的 provider
- **AND** 页面 MUST 展示后端返回的 route summary、vendor 或 route 标识
- **AND** 页面 MUST 提示真实执行前会重新报价，最终以实际执行结果为准

#### Scenario: 开仓成功后展示实际 provider 和 route
- **GIVEN** 用户提交开仓且链上执行成功
- **WHEN** 页面展示开仓成功结果或进度详情
- **THEN** 页面 MUST 展示实际执行的 provider
- **AND** 页面 MUST 展示实际 route summary、vendor 或 route 标识
- **AND** 若本次有多段 swap，MUST 分段展示每段 swap 的 provider 与 route

#### Scenario: 撤仓成功后展示实际 provider 和 route
- **GIVEN** 用户提交撤仓或部分撤仓且清仓兑换成功
- **WHEN** 页面展示撤仓结果或任务进度
- **THEN** 页面 MUST 展示每笔清仓兑换实际使用的 provider
- **AND** 页面 MUST 展示每笔清仓兑换的 route summary、vendor 或 route 标识

