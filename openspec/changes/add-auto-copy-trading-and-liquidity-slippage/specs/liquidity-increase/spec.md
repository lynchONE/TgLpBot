## ADDED Requirements

### Requirement: 补充流动性支持本次滑点覆写
系统 SHALL 在补充流动性时允许用户提供可选滑点参数 `slippage_tolerance`。未提供该参数时，系统 MUST 使用当前仓位已保存的滑点作为本次补流动性滑点。

#### Scenario: 未修改滑点时沿用当前仓位滑点
- **GIVEN** 用户对已有仓位发起补充流动性
- **AND** 请求未包含 `slippage_tolerance`
- **WHEN** 后端构造补流动性报价、模拟和交易
- **THEN** 系统 MUST 使用该任务当前保存的 `slippage_tolerance`

#### Scenario: 用户修改本次补流动性滑点
- **GIVEN** 用户对已有仓位发起补充流动性
- **AND** 请求包含合法的 `slippage_tolerance`
- **WHEN** 后端构造补流动性报价、模拟和交易
- **THEN** 系统 MUST 使用请求中的滑点执行本次补流动性
- **AND** MUST NOT 因本次覆写自动修改该任务长期保存的滑点配置

#### Scenario: 补流动性滑点非法
- **WHEN** 用户提交的 `slippage_tolerance` 不满足系统滑点校验范围
- **THEN** 系统 MUST 拒绝本次补流动性请求
- **AND** MUST NOT 发起报价、授权或链上交易

### Requirement: 补充流动性界面默认展示当前仓位滑点
MiniApp 与 WebApp 的补充流动性入口 SHALL 展示当前仓位滑点作为默认值，并允许用户在提交前修改本次滑点。

#### Scenario: 打开补充流动性弹窗
- **WHEN** 用户打开某个仓位的补充流动性弹窗
- **THEN** 界面 MUST 将该仓位当前滑点填入滑点输入框
- **AND** 用户可以在不修改滑点的情况下直接提交

#### Scenario: 修改滑点后提交
- **WHEN** 用户在补充流动性弹窗中修改滑点并提交
- **THEN** 前端 MUST 将修改后的 `slippage_tolerance` 传给后端补流动性接口
