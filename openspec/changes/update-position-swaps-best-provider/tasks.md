## 1. Contracts
- [x] 1.1 `ZapSimple`: 将单一 OKX router/approve 校验改为 owner 管理的 trusted swap target / approve target allowlist。
- [x] 1.2 `AtomicIncreaseZap`: 同步支持 trusted swap target / approve target allowlist。
- [x] 1.3 部署与更新脚本: 支持初始化或更新 OKX 与 Binance trusted target/approveTarget。
- [ ] 1.4 合约测试: 覆盖 OKX 路由、Binance 路由、不可信 target、不可信 approveTarget、native value 路由拒绝、minOut/余额 delta 保护。

## 2. Backend
- [x] 2.1 数据模型/API: 新增并校验 `swap_provider_policy`, 支持 `best`、`okx`、`binance`, 任务持久化该策略。
- [x] 2.2 后端: 新增开仓/撤仓 swap 报价归一化结构，复用 OKX 与 Binance adapter 获取候选报价。
- [x] 2.3 后端: 实现 provider policy 过滤与最优选择逻辑；单 provider 失败时不自动切换。
- [x] 2.4 后端: 将 `PreviewEntrySwap` 的 OKX 单报价改为按 provider policy 返回最优 provider、route 和预计到账。
- [x] 2.5 后端: 将开仓 entry swap 执行从 `swapExactInViaOKX` 改为执行前重新报价并执行策略允许的实际 provider。
- [x] 2.6 后端: 将 Zap 内部配比 swap 参数构造从 OKX 专用改为按 provider policy 选择 OKX/Binance，并通过 Zap allowlist 限制可执行目标。
- [x] 2.7 后端: 将撤仓、部分撤仓和清仓 dust 路径里的 token->稳定币 swap 改为按任务 provider policy 重新报价并执行实际 provider。
- [x] 2.8 后端: 补充日志和执行结果字段，记录 provider、quoteId、routeSummary、expectedOut、actualOut、txHash。
- [ ] 2.9 Tests: 增加 provider policy 单元测试覆盖 best/okx/binance、OKX 更优、Binance 更优、单 provider 不可用、双 provider 不可用、执行前 provider 变化。

## 3. WebApp
- [x] 3.1 `OpenPositionModal` 增加兑换渠道选择控件，默认 `自动择优`, 可选 `仅 OKX`、`仅 Binance`。
- [x] 3.2 WebApp 开仓预览卡展示预计 provider 与 route。
- [x] 3.3 WebApp 开仓确认与执行结果展示实际 provider 与 route。
- [x] 3.4 WebApp 撤仓/部分撤仓结果在交易历史里展示每笔清仓兑换实际 provider 与 route。

## 4. MiniApp
- [x] 4.1 MiniApp 开仓流程增加兑换渠道选择控件，默认 `自动择优`, 可选 `仅 OKX`、`仅 Binance`。
- [x] 4.2 MiniApp entry swap 预览与确认面板展示预计 provider 与 route。
- [x] 4.3 MiniApp 开仓执行结果和撤仓/部分撤仓结果展示实际 provider 与 route。

## 5. Verification
- [x] 5.1 运行 `cd contracts; npm run compile`。
- [x] 5.2 运行 `cd backend; go test ./...`。
- [x] 5.3 运行 `cd webapp; npm run build`。
- [x] 5.4 运行 `cd miniapp; npm run build`。
- [x] 5.5 做针对性 diff 检查，确认没有遗漏 API 字段、调用点或回归。
