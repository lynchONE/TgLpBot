# Change: 增加自动跟单与补流动性滑点配置

## Why
用户在补充流动性时需要按本次操作调整滑点，而不是只能沿用仓位已有滑点；同时，聪明钱模块需要从“观察与提醒”扩展为可配置的自动跟单能力，减少用户手动跟随开仓和撤仓的延迟。

## What Changes
- 补充流动性请求增加可选滑点参数：
  - 默认使用当前仓位 `strategy_tasks.slippage_tolerance`。
  - 用户可在本次补流动性弹窗中修改滑点，并且本次执行按修改后的滑点构造报价、模拟与交易。
  - 滑点输入必须复用现有开仓滑点校验规则，非法值直接拒绝。
- 聪明钱模块新增“自动跟单”页签：
  - 支持配置要跟单的钱包。
  - 支持设置跟单开仓金额，金额模式包括固定金额和按被跟单开仓金额比例。
  - 支持设置立即跟单或固定延时跟单。
  - 支持配置撤仓是否跟单；开启后按被跟单钱包的撤仓事件对对应跟单仓位执行撤仓。
  - 支持启用/停用单条跟单配置。
- 后端新增自动跟单配置、任务映射与执行调度能力：
  - 自动跟单默认关闭，只有用户显式保存并启用后才执行。
  - 跟单开仓只消费启用时间之后的新事件，不回放历史事件。
  - 撤仓跟单只影响由该配置创建的跟单仓位，不影响用户手动仓位或 AutoLP 仓位。

## Impact
- Affected specs:
  - `liquidity-increase`
  - `miniapp-smart-money`
- Affected code:
  - `backend/base/models/*`
  - `backend/service/smart_money/*`
  - `backend/service/strategy/*`
  - `backend/service/liquidity/*`
  - `backend/service/web_server/task_add_liquidity.go`
  - `backend/service/web_server/smart_money*.go`
  - `miniapp/src/lib/smartMoneyApi.js`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `miniapp/api/sm*.js`

## Compatibility
- 补充流动性请求不传滑点时保持现有行为。
- 自动跟单是新增页签与新增配置，不改变现有池子视图、钱包视图、合约视图、监控通知和聪明钱资产页签。
- 已存在的 `add-miniapp-smart-money-follow-wallet` 提案偏向钱包弹窗跟单、随机延时与固定金额；本变更以“自动跟单页签 + 固定/比例金额 + 立即/固定延时 + 撤仓可配置”为准，后续实现应避免沿用旧提案中 ClickHouse 或随机延时的限制。
