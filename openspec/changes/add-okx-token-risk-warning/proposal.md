# Change: 增加 OKX 代币风控提示

## Why
用户在新加入池子流动性后遭遇 rugpull，当前池子榜单与开仓链路缺少对非主流稳定/原生资产的代币风险提示。

## What Changes
- 使用 OKX `market/token/advanced-info` 查询非 BNB/WBNB/USDT/USDC 及其他稳定币的代币风控信息。
- 在池子列表与搜索结果返回代币风险等级、貔貅盘、低流动性等风险标签。
- 在开仓 prepare/preview/execute 链路展示代币风控检查；貔貅盘必须阻止真实开仓。
- WebApp 与 MiniApp 在池子列表和开单前展示统一风险提示。

## Impact
- Affected specs: `pool-token-risk`, `open-position-safety`
- Affected code: OKX market API client, pool catalog/search API, open position API, WebApp/MiniApp pool list and open-position UI
