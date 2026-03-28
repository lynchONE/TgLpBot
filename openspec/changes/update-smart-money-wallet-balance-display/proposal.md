# Change: 聪明钱钱包视图展示钱包余额

## Why
- 当前聪明钱的钱包列表与钱包详情只展示标签、持仓数、活跃池数等信息，没有直接展示钱包总余额，用户进入钱包视图后无法快速判断该钱包当前资金体量。
- WebApp 和 MiniApp 都已经具备聪明钱钱包列表与详情页，适合统一补充“钱包余额”字段，减少来回切换页面或依赖其他模块查看资产的成本。

## What Changes
- 后端聪明钱钱包列表与钱包详情接口增加钱包余额字段，口径复用现有聪明钱资产服务中的钱包总资产 `TotalUSD`。
- WebApp 聪明钱钱包列表与钱包详情页增加钱包余额展示。
- MiniApp 聪明钱钱包列表与钱包详情页增加钱包余额展示。

## Impact
- Affected specs: `smart-money-wallet-view`
- Affected code:
  - `backend/service/assets`
  - `backend/service/web_server/smart_money.go`
  - `backend/service/smart_money/repository.go`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `miniapp/src/components/SmartMoneyPage.jsx`
