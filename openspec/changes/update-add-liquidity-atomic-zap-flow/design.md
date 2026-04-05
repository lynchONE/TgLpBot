## Context
- 当前补仓入口固定接收稳定币金额，后端会先根据 `entry token` 规则决定是否先做一次 `stable -> entry token` 换币。
- 当池子不包含稳定币时，现有补仓链路至少包含两段独立动作：
  - 钱包直接发送前置 swap
  - 钱包再发送 V3 `increaseLiquidity` 或 V4 `modifyLiquidities`
- 这条链路在 V4 上尤其脆弱，因为链上真实结算和离线估算经常存在偏差，容易出现“前置 swap 成功、补仓失败”的资金残留。

## Goals / Non-Goals
- Goals:
  - 让 `task_add_liquidity` 在 V3 / V4 上都以单笔交易完成补仓所需的换币与加仓。
  - 消除“前置 swap 已落链，但补仓失败”的中间态。
  - 为补仓单独建立可部署、可绑定、可模拟的原子 Zap 合约和后端调用链。
  - 让补仓记账使用实际投入、实际退款、实际 gas 与主交易哈希。
- Non-Goals:
  - 本次不重构开仓 `EnterTaskFromUSDT` 的旧 funding 链路。
  - 本次不改造撤仓逻辑。
  - 本次不支持任意多段 swap pipeline，只覆盖当前补仓需要的两段换币模型。

## Decisions
### Decision: 使用独立 `AtomicIncreaseZap`，不继续扩展 `ZapSimple`
- 初始方案尝试直接在 `ZapSimple.sol` 中增加 `zapIncreaseV3` / `zapIncreaseV4`。
- 实际编译后合约大小超过 24KB 限制，因此改为拆出独立的 `AtomicIncreaseZap.sol`。
- 这样可以把“开新仓”和“已有仓位补仓”解耦，避免影响现有 `zapInV3` / `zapInV4` 行为与部署。
- Alternatives considered:
  - 继续压缩 `ZapSimple.sol` 以塞进大小限制：维护成本高，且会把两个职责完全不同的执行路径耦合在一起，因此不采用。

### Decision: 原子补仓最多支持两段 swap，且都必须在同一笔交易内完成
- 保留“输入是稳定币金额”的产品形态。
- 当稳定币不在池子里时，原子补仓需要同时覆盖两类换币：
  - `entrySwap`: `stable -> entry token`
  - `rebalanceSwap`: `token0 <-> token1`
- 当稳定币本身就是 `token0` 或 `token1` 时，`entrySwap` 为空，仅执行 `rebalanceSwap` 或直接补仓。
- Alternatives considered:
  - 保留前置 swap 为独立交易：无法满足“失败时不残留 swap”的目标，因此不采用。
  - 做成任意数量的 swap pipeline：复杂度过高，不适合作为本次最小可行变更，因此不采用。

### Decision: 后端负责预览与模拟，合约负责基于实时余额完成补仓
- 后端继续负责：
  - 读取最新 `tokenId` 元数据
  - 同步真实 ticks / token 顺序
  - 调用 OKX API 构造 swap calldata
  - 在广播前通过 `eth_call` 模拟 `zapIncreaseV3` / `zapIncreaseV4`
- 合约负责：
  - 拉取稳定币资金
  - 在同一笔交易内执行可选 swap
  - 基于合约内实际到账余额完成 `increaseLiquidity` / `modifyLiquidities`
  - 退款未使用资金与 dust

### Decision: 使用独立私有绑定 kind `atomic_increase_zap`
- 原子补仓不会复用 `zap_simple` 的私有绑定记录。
- 新增 `wallet_chain_contract.kind = atomic_increase_zap`，配套独立部署、缓存和配置逻辑。
- 这样无需提升 `PRIVATE_ZAP_VERSION`，也不会影响已有开仓/撤仓链路。

### Decision: 补仓结果以实际消耗与主交易哈希记账
- 任务的 `amount_usdt` 改为按实际稳定币消耗累加，而不是按用户请求金额直接累加。
- 交易记录补充累计：
  - 实际稳定币投入
  - gas 消耗
  - token dust
- Web API 返回本次原子补仓主交易哈希，便于前端直接展示和排查。

## Risks / Trade-offs
- 第一次补仓可能需要额外发送 NFT 授权交易。
  - Mitigation: 后端在主交易前自动检查并补授权。
- 两段 swap 的报价时效更敏感，过期会导致整笔交易回滚。
  - Mitigation: 广播前重新拉取 OKX calldata，并强制先模拟再发送。
- 原子补仓交易 gas 更高。
  - Mitigation: 通过 `ATOMIC_ADD_LIQUIDITY_ENABLED` 灰度开关控制切量，必要时可快速回退。

## Migration Plan
1. 新增 `AtomicIncreaseZap.sol`，实现 V3/V4 原子补仓、两段 swap、dust refund 和 NFT ownership/approval 校验。
2. 重新编译合约，并生成后端专用的 ABI / bytecode 嵌入文件与 Go 绑定。
3. 后端新增独立的 atomic zap 部署与绑定逻辑，将补仓主流程切到新的原子调用路径。
4. 调整 `task_add_liquidity` 和 trade record 记账逻辑，改为记录实际投入、退款和主交易哈希。
5. 通过 `ATOMIC_ADD_LIQUIDITY_ENABLED` 灰度开启并验证。

## Open Questions
- 暂无。
