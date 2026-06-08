# Change: 按代币从索引数据源发现大额加池钱包并批量导入聪明钱

## Why
- 用户希望输入代币地址后，自动找出“在该代币相关池子里大额加池”的钱包，例如加池金额大于等于 500U，再批量导入项目的聪明钱监控列表。
- 直接用 RPC 做链上事件扫描会带来数据量和 RPC 压力，本功能不采用 RPC 查询路径。
- GMGN、DexScreener、GeckoTerminal、DEXTools 这类 DEX 终端通常更适合提供代币、池子、交易、价格和流动性概览；其中不一定有“按代币返回 LP 加池钱包”的直接接口。
- 因此该能力首版使用 Bitquery 作为主索引数据源；其他 DEX 终端作为辅助数据源。

## What Changes
- 新增按代币发现大额加池钱包的后端能力：
  - 输入链、代币地址、最低加池 USD 金额、时间窗口、返回数量。
  - 系统调用外部索引数据源查询该代币相关 DEX 流动性事件。
  - 系统筛选达到阈值的钱包，并保留交易哈希、实际池子、金额来源和数据源。
- 数据源策略：
  - 首选：Bitquery，组合 BSC DEXPoolEvents、Smart Contract Events 和 TransactionBalances 查询 LP 加池事件、事件参数和金额依据。
  - 辅助：GMGN、DexScreener、GeckoTerminal、DEXTools 等用于代币基础信息、池子发现、价格和流动性展示。
  - 禁止：不使用 RPC 查询作为该功能的数据来源。
- 新增筛选结果预览，展示钱包地址、加池金额、加池时间、交易哈希、实际加池池子、交易对信息、数据来源和是否已在聪明钱监控列表中。
- 新增批量导入接口，把用户确认的钱包写入 `monitored_wallets`，来源标记为代币加池筛选，并保留代币地址作为来源上下文。
- MiniApp 和 WebApp 都新增该功能，但分别按移动端和桌面端交互实现：
  - MiniApp：移动端弹层/底部抽屉，输入、预览、勾选和导入适配窄屏。
  - WebApp：桌面端面板/模态框，支持更宽的候选表格、筛选摘要和批量操作。
- 外部 API Key 通过环境变量或系统配置读取，不写入代码，不在日志中输出明文。
- 外部接口失败、索引源未配置、USD 金额缺失等情况必须返回明确错误或排除原因，不使用默认金额或空钱包列表掩盖真实失败。

## Impact
- Affected specs:
  - `smart-money-wallet-import`
- Affected code:
  - Backend: `backend/service/web_server/smart_money.go`, `backend/service/smart_money/*`, `backend/base/models/smart_money.go`, `backend/base/config/config.go`
  - MiniApp: `miniapp/src/lib/smartMoneyApi.js`, `miniapp/src/components/SmartMoneyPage.jsx`, `miniapp/src/features/smartMoney/shared/*`
  - WebApp: `webapp/src/smartMoneyApi.js`, `webapp/src/components/SmartMoneyDashboard.jsx`, `webapp/src/styles.css`
  - Config/Deploy: `docker-compose.env.example`, README 或部署说明中的外部数据源 API Key 配置
- Data model:
  - 复用 `monitored_wallets`，新增来源值 `token_liquidity_indexer`。
  - `SourceContract` 保存触发筛选的代币地址；候选结果和可选导入批次明细中保留实际池子地址、交易哈希和索引数据源。
