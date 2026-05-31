# Change: 增加 OKX 代币风控提示

## Why
用户在新加入池子流动性后遭遇 rugpull，当前池子榜单与开仓链路缺少对非主流稳定/原生资产的代币风险提示。上线后又发现池子列表刷新会频繁触发 OKX `advanced-info`，出现大量 429 并导致风控未知。

## What Changes
- 使用 OKX `market/token/advanced-info` 查询非 BNB/WBNB/USDT/USDC 及其他稳定币的代币风控信息。
- 新增代币风控数据库快照，按 `chain + token_address` 持久化风险等级、貔貅盘、低流动性、标签、持仓指标与查询错误。
- 池子列表和搜索结果只读取本地快照；缺失或过期的代币进入后台限速刷新队列，避免列表刷新重复打 OKX。
- 开仓 `prepare/preview/execute` 链路展示代币风控检查；缺失或过期时允许单币即时刷新并写回数据库；貔貅盘必须阻止真实开仓。
- WebApp 与 MiniApp 在池子列表和开单前展示统一风险提示。

## Impact
- Affected specs: `pool-token-risk`, `open-position-safety`
- Affected code: OKX market API client, token risk snapshot model/migration, pool catalog/search API, open position API, WebApp/MiniApp pool list and open-position UI
