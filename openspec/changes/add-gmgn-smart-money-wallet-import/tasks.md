## 1. Implementation
- [x] 1.1 首版主索引数据源采用 Bitquery，确认 BSC DEXPoolEvents、Events、TransactionBalances 的鉴权、字段、分页、限流和费用。
- [x] 1.2 增加 Bitquery 配置项，例如 `SMART_MONEY_LIQUIDITY_INDEX_PROVIDER=bitquery`、`BITQUERY_API_KEY`，并确保日志脱敏。
- [x] 1.3 后端实现 Bitquery provider：按链、代币、窗口、金额阈值查询 LP 加池候选事件。
- [x] 1.4 首版不接入辅助池子/价格源；响应 `sources` 明确只列 Bitquery，避免把 GMGN、DexScreener、GeckoTerminal 的池子展示数据误当作钱包证据。
- [x] 1.5 后端实现金额校验和候选聚合，缺少可信 USD 金额的事件必须排除并记录原因。
- [x] 1.6 后端新增候选钱包预览接口，校验链、代币地址、金额阈值、窗口和数量限制，并标记 `already_monitored`。
- [x] 1.7 后端新增批量导入接口，复用 `monitored_wallets` upsert，来源写入 `token_liquidity_indexer`，代币地址写入 `source_contract`。
- [x] 1.8 MiniApp 新增 API wrapper 和移动端“索引源筛选导入”底部抽屉。
- [x] 1.9 WebApp 新增 API wrapper 和桌面端“索引源筛选导入”模态框或工作台面板。
- [x] 1.10 更新 MiniApp/WebApp 钱包来源展示，把新增来源显示为代币加池筛选，并展示来源代币短地址。

## 2. Validation
- [x] 2.1 后端单元测试覆盖参数校验、provider 未配置、provider 字段缺失、USD 金额缺失和重复候选聚合；导入 upsert 走服务包编译验证。
- [x] 2.2 MiniApp 执行 `npm run build`。
- [x] 2.3 WebApp 执行 `npm run build`。
- [x] 2.4 后端执行相关包测试，例如 `cd backend; go test ./service/smart_money ./service/web_server`。
- [x] 2.5 修改完成后执行针对性 diff 检查，确认接口名、字段名和 MiniApp/WebApp 调用点一致。
