## Context
Smart Money 当前存在两条链路：
- `watcher`：监听已监控钱包的 LP 事件。
- `crawler`：通过 Explorer `txlist` 扫描监控合约，把与这些合约直接交互的钱包加入监控列表。

Explorer 方案在 BSC 上已经失效，继续保留不仅会让监控合约游标卡死，也会让 Smart Money 成为项目里少数不走 RPC 池的模块。另一个实际问题是，WebSocket 节点的稳定性明显弱于现有 HTTP 节点池，继续依赖 WS 订阅会放大监控中断风险。

## Goals / Non-Goals
- Goals:
  - 删除 Smart Money 对 Explorer API 的运行时依赖。
  - 删除 Smart Money 中分离的 `watcher/crawler` 双链路，统一为单一的 HTTP 轮询监控链路。
  - 让 Smart Money 的 RPC 解析遵循“RPC 池优先，`.env` 回退”的统一规则。
  - 保持 `last_scanned_block` 可持续推进，前端能看到真实监控状态。
- Non-Goals:
  - 本次不引入第三方索引服务。
  - 本次不恢复 Explorer 式历史全量回放。
  - 本次不引入 trace/debug RPC 依赖。

## Decisions
- Decision: 将 Smart Money 改为单一的 HTTP 轮询监控器，在同一轮区块处理中同时执行 LP 事件处理和监控合约钱包发现。
  - Why: 两类能力本质上都依赖链上增量变化；统一后更容易共享区块游标、故障恢复、错误惩罚和状态展示，同时去掉对 WebSocket 稳定性的依赖。

- Decision: 监控合约钱包发现基于“区块内直接发送到监控合约地址的交易”。
  - Why: 当前产品需求就是识别“谁和监控合约直接发生了交易”。直接读取区块交易列表并按 `tx.to == contract` 过滤，语义清晰，也与现有需求完全一致。

- Decision: LP 事件继续通过 HTTP RPC 的 `eth_getLogs` / `FilterLogs` 处理。
  - Why: LP 事件天然是日志驱动，继续使用日志过滤可以避免扫描无关交易。

- Decision: 统一监控链路在每轮处理前都重新解析当前生效 HTTP RPC。
  - Why: RPC 池支持切换 current 节点，Smart Money 不能在启动时固定住某个 URL。

- Decision: 当 `last_scanned_block=0` 时，将游标初始化到当前最新区块，而不是从创世块开始回放。
  - Why: 新方案的目标是“从启用监控后持续增量发现”，不是做历史全量索引。

- Decision: 删除 Smart Money 对 `BSCSCAN_API_KEY` / `ETHERSCAN_API_KEY` / `SMART_MONEY_CRAWLER_*` / `SMART_MONEY_RECONNECT_INTERVAL_SEC` 的运行时依赖。
  - Why: 新方案完全建立在 HTTP RPC 之上，这些旧配置不应再决定链路是否可启动。

## Alternatives considered
- 保留 `watcher + crawler` 双链路，只把 `crawler` 的数据源从 Explorer 替换成 RPC。
  - 缺点：会保留重复的区块推进、错误处理和状态暴露逻辑，没有必要。

- 继续依赖 WebSocket 订阅新区块，再把 HTTP 作为兜底。
  - 缺点：实践中 WS 稳定性更差，切换逻辑也更复杂，收益不高。

- 使用 trace / debug API 获取更深层的合约调用关系。
  - 缺点：不同 RPC 提供商支持差异很大，不适合作为默认运行前提。

## Risks / Trade-offs
- 风险：仅识别直接发送到监控合约地址的交易，不能覆盖 router 转调或 internal call。
  - Mitigation: 明确当前监控语义就是“直接交互”；如后续要覆盖更深层调用，再单独设计 trace 方案。

- 风险：HTTP 轮询不是推送模型，理论上存在秒级延迟。
  - Mitigation: 使用较短轮询间隔，并在每轮中补齐 `last_processed+1 -> latest` 的所有缺块，不会长期落后。

- 风险：每块读取完整交易列表会增加 HTTP RPC 压力。
  - Mitigation: 当前仅在 BSC 单链运行，且块间隔较短；先以可维护性优先，后续再根据数据量决定是否做更细分优化。

## Migration Plan
1. 移除 Explorer `txlist` 调用和相关 key 依赖。
2. 将现有 `watcher/crawler` 收敛为单一的 HTTP 轮询监控器。
3. 统一通过 RPC 池解析 HTTP 节点，池子为空时回退 `.env`。
4. 调整 `last_scanned_block=0` 的初始化策略。
5. 更新前端文案和配置示例，避免继续暗示旧的 crawler / WS 方案。
6. 重启服务，使旧的“未扫描”合约在新的统一链路下重新推进游标。

## Open Questions
- 是否需要在前端显式提示“合约监控只覆盖直接发送到该合约的交易”。
- 是否需要为管理员提供“手动重置合约监控游标”的入口。
