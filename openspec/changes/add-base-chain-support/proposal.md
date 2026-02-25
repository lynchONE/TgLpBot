# Change: 单实例多链（BSC + Base），并为后续 Solana 预留扩展点

## Why
- 现有交易闭环（Zap 合约 + 开仓/撤仓 + PnL/Gas 估算）默认只跑在 BSC（chainId=56），代码中存在大量 BSC 特有硬编码：RPC/chainId、稳定币精度（当前按 18 处理）、WBNB 作为 gas 资产、Pancake 价格池/Factory、BscScan 链接等。直接切到 Base（chainId=8453）会出现开仓失败或金额/收益计算错误。
- 你希望「单实例多链」：同一个后端进程同时管理 BSC + Base 的任务；这要求 **链信息成为一等公民**（DB、API、执行器、展示都要带 chain），不能再用全局单链 client。
- 后续可能扩展 Solana（非 EVM）：本次实现需要避免把“链=EVM”写死在业务层，至少要把链配置与执行入口做接口化/可插拔。

## What Changes
- 引入 `ChainConfig`（链配置）概念，并一次性支持 **多链并行**（单实例多链）。
  - 配置化：每条链独立配置 RPC/chainId、稳定币（本次 Base 也按 USDT）、wrapped native、Zap/NPM/PoolManager/StateView、OKX allowlist、Explorer 模板等。
  - 运行时：初始化并维护 `map[chain]*EVMClient`（BSC + Base），按任务链路选择正确 client/chainId。
  - 预留：`ChainKind`（如 `evm` / `solana`）与 `ChainExecutor` 接口，业务入口通过 executor 分发（本次只实现 EVM executor）。
- 数据模型：为核心交易闭环表新增 `chain` 维度（默认 `bsc` 保持兼容）。
  - `strategy_tasks` / `trade_records` / `transactions` /（如仍使用）`positions` 等记录需要可区分链。
  - 任务创建/展示/执行/查询时必须携带 `chain`，避免跨链混用。
- Contracts：Hardhat 增加 Base 网络，支持部署/验证 ZapSimple 到 Base；部署后通过 `setTrustedAddresses` 配置 Base 上的 OKX Router/TokenApprove + V3/V4 PositionManager（如 Base 仅 V3，则 V4 可设为 0 地址）。
- Backend（EVM executor）：
  - 开仓/撤仓、OKX swap、Gas 价值与 PnL 相关逻辑从 `ChainConfig` 取参，不再假设 “USDT=18 / gas=BNB / explorer=bscscan / factory=bsc”。
  - PoolService 的 V3 factory 识别改为按链配置匹配（BSC: Pancake/Uniswap；Base: Uniswap 等），并增加兜底：当 exchange 无法识别时，使用链配置的默认 V3 PositionManager（避免 `exchange` 字段导致无法开仓）。

## Impact
- Backend（预期改动点）：
  - `backend/base/config/config.go`（链选择 + ChainConfig）
  - `backend/base/blockchain/client.go`（多链初始化 + client/chainId 映射）
  - `backend/base/models/*`（核心表增加 `chain` 字段与索引；默认 bsc）
  - `backend/service/web_server/*`（涉及开仓/查询/操作的 API 增加/透传 chain）
  - `backend/service/liquidity/*`（稳定币 decimals、OKX chainId、native gas 估值）
  - `backend/service/pool/pool.go`（factory 识别 + 默认 PM 兜底）
  - `backend/service/pricing/*`、`backend/service/strategy/*`、`backend/service/trade/*`（native 价格与 gas->USD，按链）
  - `backend/service/bot/*`（explorer 链接）
- Contracts：
  - `contracts/hardhat.config.js`（Base network + verify）
  - `contracts/scripts/deploy_zap_simple.js`（输出文案/verify 逻辑链无关化）
  - `contracts/README.md`（Base 部署说明）
- Backwards compatibility：
  - 默认启用 BSC 配置；旧数据通过 `chain` 默认值 `bsc` 保持行为不变。
  - 新增 Base 的 env 变量为可选；只有在开启 Base 时才要求配置（RPC、USDT、Zap/NPM 等）。
  - DB schema 会新增列（`chain`）但保持旧业务可运行；兼容期内 API 若未传 chain，则使用 `bsc`。
