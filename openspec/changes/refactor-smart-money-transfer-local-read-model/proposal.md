# 变更：将聪明钱转账检测改为本地持久化读模型

## Why
- 当前资产管理里的聪明钱钱包详情为了生成“今天”的转账标识与盈亏日历，会在请求时按“今日 0 点到现在”实时扫链补转账数据；在 Base 这类快链上，这条路径很容易触发 RPC 的区块跨度限制，结果也会受当前 RPC 完整性影响。
- 聪明钱本质上是持续监控链路。普通转账如果要影响盈利日历和排行榜口径，应该和 LP 事件一样在监控时增量解析并入库，而不是在读取时反查整日区块。
- 当前本地只有 `sm_wallet_daily_snapshots` 的日汇总字段，没有可复用的聪明钱包转账明细表，导致“今天”的数据无法稳定依赖本地聚合计算。

## What Changes
- 后端：
  - 新增 `sm_wallet_transfer_events` 明细表，持久化聪明钱包普通转账事件，记录链、钱包、方向、资产类型、token 地址、符号、decimals、数量、估算 USD、tx hash、block、时间等信息。
  - 扩展 Smart Money watcher，在现有增量区块轮询中同步解析已激活钱包的普通转账并入库；ERC20 转账基于同一小窗口内的 `Transfer(address,address,uint256)` 日志解析，原生币转账基于区块交易 `value` 解析。
  - 延续现有盈利口径，排除 LP add/remove 同 tx 造成的内部流转，避免把 LP 操作误记为普通转账。
  - 聪明钱每日快照、排行榜快照、钱包详情今日数据与盈亏日历全部改为从本地转账事件聚合读取，删除请求时按整日扫链补 today 转账的路径。
- 前端：
  - 继续复用现有转账标识与金额展示字段，不要求新增交互。
  - 钱包详情不再因为实时扫链失败而返回 `today transfer detection incomplete` 这类“当日转账检测不完整”告警。

## Impact
- Affected specs:
  - `admin-smart-money-analytics`
  - `analytics-performance`
  - `smart-money-monitoring`
- Affected code:
  - `backend/base/models/asset_management.go`
  - `backend/base/database/mysql.go`
  - `backend/service/smart_money/watcher.go`
  - `backend/service/smart_money/repository.go`
  - `backend/service/assets/smart_money.go`
  - `backend/service/assets/smart_money_test.go`

## Validation Notes
- 当前环境缺少 `openspec` CLI，无法执行 `openspec validate refactor-smart-money-transfer-local-read-model --strict`；已通过 Go 单元测试覆盖本次后端实现改动。
