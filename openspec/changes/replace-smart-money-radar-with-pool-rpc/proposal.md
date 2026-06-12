# Change: 将聪明钱雷达改为按池子 RPC 筛选

## Why
当前聪明钱雷达依赖 Bitquery 按代币地址发现大额加池钱包，但 Bitquery API Key 获取存在阻碍，且按代币聚合会混入多个池子、多个费率档和无关交易对，证据口径不够精确。

项目已有 V3/V4 LP 事件解析、仓位元数据补全和 USD 金额计算能力。本次改为以池子为筛选入口，通过现有 RPC 池读取链上日志，并复用现有聪明钱 LP 解析链路完成候选钱包发现。

## What Changes
- **BREAKING**: 聪明钱雷达候选预览入口从 `token_address` 改为 `pool_address` / `pool_id`，不再兼容按代币地址直接筛选。
- 雷达数据源固定使用项目现有 RPC 池，不新增数据源环境变量，也不再要求 `BITQUERY_API_KEY`。
- 雷达预览按指定池子扫描 V3/V4 加 LP 事件，返回达到最低 USD 金额阈值的钱包候选。
- V3 与 V4 都必须支持：
  - V3 通过 PositionManager 的 `IncreaseLiquidity` 日志定位加仓事件，并通过 position metadata 过滤目标池子。
  - V4 通过 PoolManager 的 `ModifyLiquidity` 日志过滤目标 `poolId`，并复用 receipt transfer 解析补齐金额。
- 复用现有聪明钱 LP 事件解析、元数据补全、金额计算和候选聚合逻辑，但雷达不写入 `sm_lp_events`，只返回预览结果。
- 批量导入继续写入 `monitored_wallets`，来源保持 `token_liquidity_indexer` 或迁移为更准确的池子雷达来源值，并将来源上下文保存为目标池子标识。
- RPC 扫描窗口、chunk、日志上限和并发解析上限使用代码常量，不新增环境变量。

## Impact
- Affected specs:
  - `smart-money-pool-radar`
- Affected code:
  - Backend: `backend/service/smart_money/token_liquidity.go`, `backend/service/smart_money/watcher.go`, `backend/service/smart_money/position_metadata.go`, `backend/service/web_server/smart_money.go`, `backend/base/config/config.go`
  - MiniApp: `miniapp/src/lib/smartMoneyApi.js`, `miniapp/src/components/SmartMoneyPage.jsx`
  - WebApp: `webapp/src/smartMoneyApi.js`, `webapp/src/components/SmartMoneyDashboard.jsx`
  - Tests: `backend/service/smart_money/*_test.go`, `backend/service/web_server/*_test.go`
- Data model:
  - 预览结果不落库。
  - 导入仍复用 `monitored_wallets`，但 `source_contract` 保存池子地址或 V4 poolId，而不是 token address。
