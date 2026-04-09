# Change: 简化聪明钱钱包列表并将余额下沉到详情页

## Why
- 当前聪明钱“钱包视图”列表页为了展示 `wallet_balance_usd`，会在列表接口里逐个钱包补实时余额，这会放大首屏延迟，也让钱包列表承担了不必要的链上读取成本。
- 钱包列表页的核心目标是帮助用户快速找到目标钱包；余额、仓位细节等更适合在点击钱包后进入详情页再按需加载。

## What Changes
- 调整聪明钱钱包列表口径：列表页只展示钱包基础信息，不再展示钱包余额。
- 调整聪明钱钱包列表接口：`GET /api/sm/wallets` 不再为列表结果逐条补 `wallet_balance_usd`。
- 保留单钱包详情能力：用户点击钱包后进入详情页，再加载钱包余额、持仓与其他详细信息。
- WebApp 与 MiniApp 的聪明钱钱包列表统一移除余额展示，保持交互一致。

## Impact
- Affected specs: `smart-money-wallet-view`
- Affected code:
  - `backend/service/web_server/smart_money.go`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `webapp/src/smartMoneyApi.js`
  - `miniapp/src/lib/smartMoneyApi.js`
- Related changes:
  - `update-smart-money-wallet-balance-display`
