## 1. Implementation
- [x] 1.1 OpenSpec：补充 `smart-money-wallet-view` 规格变更，明确钱包列表为轻量列表、余额只在详情页展示。
- [x] 1.2 Backend：调整 `GET /api/sm/wallets`，移除列表结果中的钱包余额 enrichment。
- [x] 1.3 WebApp：移除聪明钱钱包列表中的余额列，保留点击进入详情页的交互。
- [x] 1.4 MiniApp：移除聪明钱钱包列表中的余额展示，保留点击进入详情页的交互。
- [x] 1.5 Verification：执行相关构建或测试，确认钱包列表首屏仅请求列表数据，详情页点击后再加载详细信息。
