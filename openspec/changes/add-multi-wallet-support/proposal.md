# Change: Add multi-wallet support (Bot + Mini App)

## Why
- 当前开单/撤仓等链上操作只使用“默认钱包”，用户虽然可以导入多个钱包，但无法在开单时选择使用哪个钱包。
- 一旦用户切换默认钱包，历史任务在撤仓/再平衡时可能会用错钱包（nonce/owner 不匹配），带来资金与稳定性风险。
- Mini App 一键开单也需要支持钱包选择，并展示每个钱包的余额，减少开错钱包的概率。

## What Changes
- 在 `GlobalConfig` 增加 `multi_wallet_enabled`（单钱包/多钱包模式开关）。
- 在 `strategy_tasks` 增加 `wallet_id` + `wallet_address`，用于将每个任务绑定到创建时选择的钱包。
- Bot：
  - `/config` 增加“多钱包模式”开关。
  - 多钱包模式下，`/newposition` 开仓流程增加“选择钱包”步骤，并展示每个钱包在所选链上的余额信息。
- Mini App：
  - 多钱包模式下，一键开单弹窗增加钱包选择器，展示余额，并在请求中传 `wallet_id`。
  - 新增后端接口用于获取“用户钱包列表 + 指定链余额”。
- 后端链上执行：
  - Enter/Exit/SwapDust/再入场/退出重试等所有与任务相关的链上交易，必须使用任务绑定的钱包，而不是默认钱包。
  - 交易串行锁（nonce 保护）从“按用户默认钱包”调整为“按任务钱包”。

## Impact
- DB schema（GORM AutoMigrate）：
  - `global_configs`：新增 `multi_wallet_enabled`
  - `strategy_tasks`：新增 `wallet_id`、`wallet_address`
- API：
  - `POST /api/trading?endpoint=open_position`：新增可选字段 `wallet_id`
  - `POST /api/settings?endpoint=wallets`：新增获取钱包列表与余额
- UI/UX：
  - Bot 开仓新增一步（仅多钱包模式）
  - Mini App 开单弹窗新增钱包选择器（仅多钱包模式）

## Backward Compatibility
- `multi_wallet_enabled` 默认 `false`，不改变现有单钱包用户体验：继续使用默认钱包，不提示选择。
- 旧任务 `wallet_id=0` 时保持兼容：仍按默认钱包执行（后续新任务会写入 wallet_id）。

## Risks / Notes
- 多钱包余额展示会增加 RPC 调用次数：采用 best-effort（失败显示 N/A）并限制并发/超时，避免影响主流程稳定性。
- 如果用户删除了任务绑定的钱包，任务后续撤仓会失败：需要在错误提示中引导用户处理（例如恢复钱包或停止任务）。

