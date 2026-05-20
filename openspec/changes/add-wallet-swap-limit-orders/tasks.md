## 1. Implementation
- [x] 1.1 新增 `WalletSwapLimitOrder` 模型、状态常量和 AutoMigrate。
- [x] 1.2 后端：新增限价单创建、列表、详情、取消 API，并接入 `swap` 模块权限。
- [x] 1.3 后端：新增限价单 service/repository，校验钱包归属、token 地址、数量、目标价格/目标到账金额、滑点和 provider。
- [x] 1.4 后端：新增限价单 worker，轮询 `open` 订单、获取报价、满足条件后原子切换 `triggering` 并执行兑换。
- [x] 1.5 后端：执行结果写入订单状态与 `transactions`，失败写入明确错误原因。
- [x] 1.6 前端：改造 `webapp` 一键兑换页面，新增市价/限价模式、目标价格/目标到账金额输入、限价单创建与订单列表。
- [x] 1.7 验证：补充后端单测并执行 `cd backend && go test ./...` 与 `cd webapp && npm run build`。
