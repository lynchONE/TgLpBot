## Context
用户实际要找的“聪明钱”不是外部平台已经打好标签的钱包，而是一个可操作定义：某个钱包在指定代币相关 LP 池子里发生过达到阈值的大额加池，例如 500U 以上。

这个定义来自链上 LP 事件，但本功能不直接通过项目 RPC 扫描事件。首版主数据源使用 Bitquery：BSC Liquidity API / DEXPoolEvents 用于定位流动性事件，BSC Smart Contract Events API 用于读取 decoded event arguments，TransactionBalances 用于补充交易内地址余额和 USD 相关字段。DexScreener、GeckoTerminal、GMGN、DEXTools 可以辅助发现池子和价格，但如果它们没有 LP 加池钱包接口，就不能作为核心筛选源。

## Goals / Non-Goals
- Goals:
  - 用户可以基于指定代币地址筛选出该代币相关的大额加池钱包。
  - 核心筛选默认通过外部索引数据源完成，不压项目 RPC。
  - 用户可以在导入前看到候选钱包、实际加池池子、交易哈希、金额依据和数据来源，并只导入勾选的钱包。
  - MiniApp 和 WebApp 都提供该功能，并分别适配移动端和桌面端布局。
  - 导入后的钱包进入现有聪明钱监控列表，参与后续 LP 事件、池子、钱包详情和自动跟单链路。
- Non-Goals:
  - 不在首版自动判断“哪个代币值得筛选”。
  - 不绕过第三方 API 鉴权、不爬取受保护页面、不镜像外部平台私有评分体系。
  - 不使用 RPC 查询作为筛选数据来源。
  - 不因为筛选结果导入而自动开启任何真实资金跟单。

## Candidate Definition
候选钱包定义为：在指定链和时间窗口内，对包含目标代币的 DEX LP 池发生 `add liquidity` / `increase liquidity` / liquidity-changing 行为，且该次加池的 USD 金额大于等于用户设置阈值的钱包。

如果同一钱包多次满足条件，预览默认按钱包聚合，保留：
- 最大单次加池金额。
- 最近一次满足条件的交易时间和交易哈希。
- 相关池子数量和代表池子。
- 数据源名称和金额来源。

## Provider Strategy
### Primary Provider
- Bitquery。
- 使用 DEXPoolEvents 查询 BSC DEX 流动性事件、池子、token pair、交易哈希和时间。
- 使用 Smart Contract Events 查询 V2/V3/V4 加池事件的 decoded arguments，用于提取 owner/sender/recipient、amount0/amount1、tick 或 position 信息。
- 使用 TransactionBalances 或价格服务补充金额依据，得到可校验的 USD 金额。

### Auxiliary Providers
- DexScreener：用于按代币拿交易对/池子、价格、流动性概览。
- GeckoTerminal：用于按代币拿 top pools、pool trades、价格和池子信息。
- GMGN：用于代币展示、池子/市场状态、价格或额外交易者信息。
- DEXTools：如配置了可用 API，可用于池子/流动性辅助数据。

## Discovery Pipeline
1. 校验目标代币：
   - 校验链和代币地址。
   - 使用辅助数据源或项目内 token metadata 服务补充代币符号、名称和展示信息。
2. 查询候选事件：
   - 使用 DexScreener/GeckoTerminal/GMGN 或项目已有池子目录获取目标代币相关池子。
   - 使用 Bitquery DEXPoolEvents 按池子和时间窗口查询流动性事件。
   - 使用 Bitquery Events API 按事件签名和交易哈希补齐 decoded arguments，提取 LP 钱包和加池数量。
3. 计算或确认 USD 金额：
   - 优先使用索引源返回的 USD 金额。
   - 如索引源只有 token 数量，使用可验证的历史价格来源计算，并标记 `amount_source`。
   - 如果某条事件无法得到可信 USD 金额，排除该事件并记录原因。
4. 聚合候选钱包：
   - 按钱包聚合，排序默认按最大单次加池金额降序，其次按最近加池时间降序。
   - 标记是否已在 `monitored_wallets` 中。

## Decisions
- Decision: 后端提供独立的预览接口和导入接口。
  - Why: 预览让用户确认候选钱包，导入接口只处理显式选择的钱包。
- Decision: 核心筛选只来自第三方索引数据源，不使用 RPC 查询。
  - Why: 降低 RPC 压力，并让响应时间和成本可控。
- Decision: 导入使用 `monitored_wallets.Source = "token_liquidity_indexer"`，`SourceContract = token_address`。
  - Why: 复用现有钱包来源展示与筛选机制，同时保留这个钱包来自哪个代币筛选。
- Decision: 批量导入默认不写入 `smart_money_user_watch_wallets`。
  - Why: `monitored_wallets` 是全局聪明钱监控主表；用户特别关注列表是个人通知偏好，应由用户另行选择。

## API Sketch
- `GET /api/sm/token_liquidity_wallet_candidates`
  - Query: `chain`, `token_address`, `min_amount_usd`, `window_hours`, `limit`, optional `provider`
  - Response: `token`, `filters`, `sources`, `candidates[]`, `excluded_count`, `warnings[]`
  - Candidate fields: `wallet_address`, `max_amount_usd`, `last_amount_usd`, `tx_hash`, `tx_time`, `token_address`, `pool_address`, `pair`, `pool_count`, `amount_source`, `provider`, `already_monitored`
- `POST /api/sm/token_liquidity_wallet_import`
  - Body: `chain`, `token_address`, `wallets[]`, optional `label_prefix`
  - Response: `created`, `reactivated`, `skipped_existing`, `invalid`

## UI Approach
- MiniApp:
  - 在聪明钱钱包视图添加移动端入口。
  - 使用底部抽屉或全屏移动弹层，候选钱包用紧凑列表展示。
  - 每条候选显示金额、钱包短地址、池子短地址、时间、数据源和勾选状态。
- WebApp:
  - 在 SmartMoneyDashboard 钱包视图添加桌面端入口。
  - 使用宽屏模态框或右侧工作台面板，候选钱包用表格展示。
  - 支持表格批量勾选、按金额/时间排序、查看交易哈希、池子和数据源。

## Verified API Findings
- Bitquery 文档确认 BSC Liquidity API / DEXPoolEvents 可以返回池子、币对、流动性变化、交易哈希和区块时间；Smart Contract Events API 可以返回 decoded event arguments；TransactionBalances 可以按交易补充地址余额变化和 USD 相关字段。因此首版主索引源固定使用 Bitquery。
- DexScreener API 文档确认可按 token address 查询 pools/pairs，并返回价格、交易统计、成交量和 liquidity 概览；公开文档未看到返回 LP 加池钱包和加池金额的接口。
- GeckoTerminal API 文档确认可查询 top pools、pool trades、OHLCV、token price 和 pool/token 信息；公开文档未看到返回 LP 加池钱包和加池金额的接口。
- GMGN Agent API 文档确认可查 token 基础信息、实时价格、合约安全、流动性池状态、Top Holders/Traders、钱包交易和 PnL；公开文档未看到按代币返回 LP 加池钱包和加池金额的接口。
- DEXTools 公开 API 门户需要授权访问；公开介绍主要覆盖 token、pool、价格、liquidity、market trend 等指标，不能在未拿到授权文档前把它当作 LP 钱包筛选主源。

## Risks / Trade-offs
- 索引数据源需要额外 API Key，且可能有付费或限流。
  - Mitigation: 配置化 provider；未配置时返回明确错误。
- 不同 provider 对 LP 事件、金额和协议支持不同。
  - Mitigation: 响应返回 provider、amount_source 和 warnings；测试覆盖字段缺失和不支持协议。
- DexScreener/GeckoTerminal 公开 API 更偏池子/交易/行情，不一定提供 LP 加池钱包。
  - Mitigation: 只把它们作为辅助源，不作为唯一核心筛选依赖。

## Open Questions
- 首版支持的协议范围：仅 Pancake/Uniswap V3，还是同时覆盖当前项目已支持的 V4。
- 候选预览是否需要做成异步任务，还是先限制窗口并同步返回。
