## Context
- 当前聪明钱事件明细表 `smart_lp_events` 已经保存 `wallet_address`、`pool_version`、`pool_id`、`token_id`、`tick_lower`、`tick_upper` 等字段。
- V3 路径会从 NPM 的 `IncreaseLiquidity/DecreaseLiquidity` 日志里写入 `token_id`，因此当前仓位查询可以直接围绕 NFT 仓位展开。
- V4 路径目前只记录 `ModifyLiquidity` 的池子、tick 和流动性变化，没有把事件中的 `salt` 落库，导致：
  - 钱包仓位接口需要额外扫描 V4 PositionManager 的 ERC721 `Transfer`
  - 池子详情接口无法像 V3 一样围绕明确的 NFT 仓位计算手续费
- 现有代码已经在 `buildV4PositionKey(owner, tickLower, tickUpper, tokenId)` 中把 `tokenId` 作为 position salt 使用，这说明将 `ModifyLiquidity.salt` 直接视为仓位 NFT 标识，与现有链上读取逻辑是一致的。

## Goals / Non-Goals
- Goals:
  - 让 V3/V4 都以 `token_id` 作为聪明钱仓位的主标识
  - 让 `smart_money_wallet_positions` 和 `smart_money_pool_adds` 共用同一类 position-ref 查询思路
  - 让 V3/V4 都能在聪明钱模块中返回 best-effort 的未领取手续费
- Non-Goals:
  - 不做历史 ClickHouse 数据的全量回填
  - 不把聪明钱接口升级为审计级收益核算系统
  - 不修改池子 marker / PnL 回放的业务口径，只在已有主键上增强 `token_id`

## Decisions
- Decision: V4 的 `token_id` 直接来源于 `ModifyLiquidity` 事件中的 `salt`
  - 写入格式使用十进制字符串，与现有 V3 `token_id` 格式保持一致
  - 若 `salt` 为空或解析失败，事件仍可入库，但 `token_id` 允许留空并走兼容分支

- Decision: 查询层抽象统一的 `position ref`
  - 优先从 `smart_lp_events` 聚合最近窗口内的 `(pool_version, pool_id, contract_address, token_id)` 引用，再按版本读取 live position
  - V3 继续依赖 `contract_address + token_id` 定位 NPM 仓位
  - V4 主路径依赖 `token_id + pool_id + 配置中的 PositionManager` 读取仓位
  - 历史 V4 空 `token_id` 数据仅保留为 fallback，不再作为默认主链路

- Decision: V3/V4 手续费都走“position info + pool state”模型
  - V3 使用 `positions(tokenId)` 与 `pool.CalcV3UnclaimedFees`
  - V4 使用 `GetV4PositionInfo(tokenId)` 与 `pool.CalcV4UnclaimedFees`
  - API 层统一输出 `claimable_fee0`、`claimable_fee1`、`claimable_fees_usd`、`fee_status`、`fee_error`

- Decision: 保持 `smart_lp_events.contract_address` 的历史兼容语义
  - 不强制迁移历史 V4 记录的 `contract_address`
  - 新的 V4 live position 查询不再依赖该字段指向 PositionManager，避免对已有 marker/PnL 回放键造成副作用

## Risks / Trade-offs
- V3 从 `collect` 模拟切换到 fee growth 计算后，链路更统一，但会更依赖池子状态读取；当 RPC 抖动时需要清晰回传 `fee_status=error/skipped`。
- V4 新数据可以直接命中 `token_id`，但历史窗口里旧事件仍可能没有 `token_id`；因此一段时间内会存在“新旧数据双路径”。
- 如果后续接入多个 V4 PositionManager，当前“从全局配置读取 V4 PositionManager”的假设需要再扩展。

## Migration Plan
1. 更新 SmartLP 扫描器，让新写入的 V4 事件带上 `token_id`
2. 抽取统一的 position-ref helper，并让钱包仓位与池子详情接口优先走新主路径
3. 保留 legacy V4 ERC721 扫描作为空 `token_id` 的兼容 fallback
4. 为新接口字段补充测试，并验证 V3/V4 在相同接口下都能返回仓位与手续费

## Open Questions
- 是否需要在响应里额外暴露 `token_id_source`（event / fallback）帮助前端区分数据来源；本变更可以先不做，除非实现阶段发现排障成本过高。
