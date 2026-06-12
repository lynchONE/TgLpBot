## Context
聪明钱雷达的实际目标是发现“某个 LP 池子里近期发生过大额加池的钱包”。按代币地址筛选会把同一个 token 的多个池子、多个协议和不同费率档混在一起，既增加 RPC/索引成本，也让候选钱包证据不够精确。

现有聪明钱 watcher 已经具备 V3/V4 LP 事件解析能力：
- `parseLog` 支持 V3 `IncreaseLiquidity` / `DecreaseLiquidity` 和 V4 `ModifyLiquidity`。
- `EnrichLPEvent` 可补齐 token、tick、fee、pool。
- `resolveAmountsFromReceipt` 可为 V4 从 receipt transfer 中补齐金额。
- `ComputeEventAmountUSD` 可用现有价格服务计算 USD 金额。

本次雷达改造的重点不是重写 LP 解析，而是增加一条“未知钱包发现”路径：按指定池子扫描链上 LP 日志，解析并聚合候选钱包，只返回预览结果。

## Goals / Non-Goals
- Goals:
  - 用户按池子地址或 V4 poolId 筛选大额加 LP 钱包。
  - V3 和 V4 都支持。
  - 雷达使用项目现有 RPC 池，不依赖 Bitquery API Key。
  - 尽量复用现有聪明钱 LP 解析、元数据补全和金额计算代码。
  - 不增加新的环境变量；扫描限制使用代码常量。
  - 无法解析钱包、池子、金额或价格时返回明确错误或排除原因，不用 fallback 伪造成有效候选。
- Non-Goals:
  - 不再支持按 token address 直接扫描。
  - 不引入新的第三方索引服务。
  - 不把雷达预览事件写入正式 `sm_lp_events` 历史。
  - 不自动开启任何真实资金跟单。

## Decisions
- Decision: 雷达入口改为池子维度。
  - Why: 池子是 CLMM LP 行为的最小可验证对象，能避免按 token 混入无关池子。

- Decision: 池子雷达不再使用数据源环境变量。
  - Why: 本能力已经固定为 RPC 路径，RPC endpoint 由现有 RPC 池管理；额外开关只会增加配置负担。

- Decision: 复用现有解析能力，但拆出可复用的“解析预览事件”函数。
  - Why: watcher 当前面向已监控钱包，先用 `tx.from` 过滤；雷达面向未知钱包，需要按池子扫描并重新做 wallet 归因。

- Decision: V3 wallet 归因优先使用 PositionManager 的 owner 事件或 `ownerOf(tokenId)`。
  - Why: `tx.from` 可能是 zap、代理或合约，不能作为雷达候选钱包的唯一依据。若无法可靠归因，事件必须排除并记录原因。

- Decision: V4 wallet 归因必须从可验证来源解析，不能用空钱包或 `tx.from` 默认兜底。
  - Why: V4 `ModifyLiquidity` 本身不总是直接给出最终 LP owner。可解析 PositionManager tokenId 时使用 token owner；否则事件排除。

- Decision: 导入上下文保存池子标识。
  - Why: 本功能已从“按 token 发现”改为“按 pool 发现”，`source_contract` 保存池子地址或 poolId 才能追溯证据。

## RPC Scan Shape
V3:
1. 读取输入池子的 `token0`、`token1`、`fee`，识别所属 V3 factory / PositionManager。
2. 按时间范围定位区块范围。
3. 扫描对应 PositionManager 的 `IncreaseLiquidity` 日志。
4. 对每个 `tokenId` 读取 position metadata，过滤出目标池子。
5. 解析钱包、金额、时间、交易哈希、区间和交易对。
6. 计算 USD 金额并按钱包聚合。

V4:
1. 输入为 V4 `poolId`，或由池子列表中已有的 V4 主标识传入。
2. 按时间范围定位区块范围。
3. 扫描配置的 V4 PoolManager `ModifyLiquidity` 日志，并过滤目标 `poolId`。
4. 仅处理加仓方向事件。
5. 复用 receipt transfer 解析补齐 token 金额。
6. 解析 PositionManager tokenId / owner；无法归因则排除。
7. 计算 USD 金额并按钱包聚合。

## Constants
首版使用代码常量：
- 默认最大时间窗口：7 天。
- 单次扫描日志上限：5000 条。
- 单次区块扫描 chunk：从现有 watcher 的 RPC 行为取保守值，遇到 RPC range limit 时可在代码中降档重试。
- 解析并发：复用现有 bounded worker 模式，不暴露配置。

## Risks / Trade-offs
- 风险：部分 RPC 对 `eth_getLogs` 范围限制严格。
  - Mitigation: 分 chunk 扫描，遇到 range limit 时缩小 chunk 并返回明确错误。
- 风险：V4 owner 归因可能不完整。
  - Mitigation: 只返回可验证 owner 的候选，不使用 `tx.from` 兜底。
- 风险：大窗口扫描耗时较长。
  - Mitigation: 首版限制最大窗口和最大日志数；超限要求用户缩小时间范围。

## Migration Plan
1. 后端新增 RPC 池子雷达能力，并移除雷达对 Bitquery 配置的依赖。
2. 预览 API 从 token 参数迁移到 pool 参数。
3. 双端 UI 将输入文案和参数改为池子地址 / poolId。
4. 错误提示移除 Bitquery API Key 要求。
5. 保留批量导入接口，但导入来源上下文改为池子标识。

## Open Questions
- 来源值是否继续沿用 `token_liquidity_indexer`，还是新增 `pool_liquidity_radar`。建议新增更准确的来源值，但需要同步 UI 来源展示。
