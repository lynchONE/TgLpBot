## ADDED Requirements

### Requirement: 开仓风险检查 MUST 先于任何前置兑换或真实开仓
系统 MUST 在进入 entry swap 预览、entry swap 确认和真实开仓前，先执行统一的开仓风险检查，并优先拦截所有风险。

#### Scenario: 价格偏差超限时直接拦截
- **GIVEN** 用户发起开仓请求
- **AND** 当前池子价格与 OKX 价格偏差超过系统允许阈值
- **WHEN** 系统处理开仓预览或真实执行请求
- **THEN** 系统 MUST 直接返回结构化风险错误
- **AND** MUST NOT 继续 entry swap 预览
- **AND** MUST NOT 执行真实 entry swap
- **AND** MUST NOT 继续后续开仓

#### Scenario: 流动性不满足要求时直接拦截
- **GIVEN** 用户发起开仓请求
- **AND** 当前池子的链上 raw liquidity 或美元流动性不满足系统要求
- **WHEN** 系统处理开仓预览或真实执行请求
- **THEN** 系统 MUST 直接返回结构化风险错误或警告
- **AND** MUST NOT 在风险未解除前继续 entry swap 预览确认流程
- **AND** MUST NOT 执行真实 entry swap 或真实开仓

### Requirement: 开仓前置兑换 MUST 支持预览与二次确认
当本次开仓需要先把钱包资产兑换成 entry token 时，系统 MUST 在真实执行前提供无副作用的预览结果，并要求用户二次确认后才能执行真实兑换。

#### Scenario: `USDC/WBNB` 池子需要先做 `USDT -> USDC`
- **GIVEN** 用户选择开 `USDC/WBNB` 池子
- **AND** 当前钱包没有足够的 `USDC` 可直接开仓
- **WHEN** 页面请求开仓预览
- **AND** 本次请求已经通过统一开仓风险检查
- **THEN** 系统 MUST 返回“需要前置 entry swap”的结果
- **AND** MUST 返回本次 entry swap 的兑换方向、预计输入数量、预计到账数量和推荐滑点
- **AND** MUST NOT 在该预览阶段执行真实兑换或创建持久化任务

#### Scenario: 钱包已有足够 entry token 时无需额外确认
- **GIVEN** 用户选择的池子 entry token 为 `USDC`
- **AND** 当前钱包已有足够 `USDC` 可直接开仓
- **WHEN** 页面请求开仓预览
- **THEN** 系统 MUST 返回“无需前置 entry swap”
- **AND** 页面 MAY 继续现有开仓流程，而不额外要求用户确认兑换

### Requirement: 稳定币到稳定币的前置兑换 MUST 返回独立推荐滑点
对于稳定币到稳定币的前置 entry swap，系统 MUST 返回独立推荐滑点，且不得简单继承用户全局滑点。

#### Scenario: 全局滑点较高时稳定币前置兑换不沿用全局值
- **GIVEN** 用户全局滑点为 `1.0%`
- **AND** 本次开仓需要先做 `USDT -> USDC`
- **WHEN** 页面在未传自定义滑点的情况下请求预览
- **THEN** 系统 MUST 返回独立的 `recommended_slippage_tolerance`
- **AND** MUST NOT 仅因为全局滑点是 `1.0%` 就把本次稳定币前置兑换默认设为 `1.0%`

#### Scenario: 用户修改本次兑换滑点后刷新预估到账数量
- **GIVEN** 页面已经拿到一次前置兑换预览结果
- **WHEN** 用户修改本次兑换滑点
- **THEN** 页面 MUST 重新请求预览
- **AND** 系统 MUST 返回基于该滑点重新计算的预计到账数量
- **AND** 页面 MUST 在刷新后的预览结果基础上，才允许用户确认执行

### Requirement: 需要前置兑换时未确认不得执行真实开仓
当系统判断本次开仓需要前置 entry swap 时，执行接口 MUST 在收到显式确认前拒绝真实执行，并在确认后严格按“先兑换、后开仓”顺序处理。

#### Scenario: 未确认前置兑换时拒绝真实执行
- **GIVEN** 本次开仓需要先做 entry swap
- **AND** 当前请求已经通过统一开仓风险检查
- **WHEN** 客户端直接调用真实执行接口且未显式确认前置兑换
- **THEN** 系统 MUST 拒绝真实执行
- **AND** MUST NOT 发起真实兑换
- **AND** MUST NOT 继续后续开仓
- **AND** MUST NOT 创建持久化任务
- **AND** MUST 返回结构化的确认提示，供页面展示

#### Scenario: 用户确认后先兑换再开仓
- **GIVEN** 本次开仓需要先做 entry swap
- **AND** 用户已经基于当前预览结果显式确认前置兑换
- **WHEN** 客户端调用真实执行接口
- **AND** 该执行请求再次通过统一开仓风险检查
- **THEN** 系统 MUST 先执行 entry swap
- **AND** 仅在 entry swap 成功后，才继续后续私有 Zap / 开仓流程
- **AND** 若 entry swap 失败，系统 MUST 停止流程且不得继续开仓
