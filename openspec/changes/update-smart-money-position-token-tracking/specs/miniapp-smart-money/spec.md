## ADDED Requirements

### Requirement: SmartLP 统一持久化 LP 仓位 tokenId
系统 MUST 将聪明钱可识别的 LP 仓位标识持久化到 `smart_lp_events`，使后续查询可以直接按 NFT 仓位读取当前状态。

- V3 MUST 从 PositionManager 的 `IncreaseLiquidity` / `DecreaseLiquidity` 事件解析并写入 `token_id`
- V4 MUST 从 `ModifyLiquidity` 事件的 `salt` 解析并写入 `token_id`
- 当历史数据或异常事件无法解析 `token_id` 时，系统 MAY 保留为空，但不得阻塞事件入库

#### Scenario: V4 ModifyLiquidity 写入 token_id
- **WHEN** SmartLP 扫描到某钱包的 V4 `ModifyLiquidity` 事件，且 `salt` 可解析为非零仓位标识
- **THEN** 写入的 `smart_lp_events.token_id` MUST 为该 `salt` 对应的十进制字符串

### Requirement: Smart Money 钱包仓位接口按 tokenId 统一查询 V3/V4 当前仓位
`GET /api/smart_money_wallet_positions` MUST 优先使用 `smart_lp_events` 中最近事件的 `token_id` 作为当前仓位查询入口，并对 V3/V4 返回一致的仓位与手续费字段。

接口返回的每个仓位 MUST 包含：
- `position_id`
- `pool_version`
- `pool_id`
- `claimable_fee0`
- `claimable_fee1`
- `claimable_fees_usd`
- `fee_status`

其中：
- V3 SHALL 通过 `positions(tokenId)` 与池子 fee growth 计算未领取手续费
- V4 SHALL 通过 PositionManager `positions/positionInfo(tokenId)` 与池子状态计算未领取手续费
- 对历史 V4 空 `token_id` 数据，系统 MAY 回退到 legacy NFT 扫描兼容路径

#### Scenario: V4 钱包仓位通过事件 token_id 直接查询
- **WHEN** 某钱包最近窗口内存在带 `token_id` 的 V4 聪明钱事件
- **THEN** `GET /api/smart_money_wallet_positions` MUST 直接基于该 `token_id` 查询当前仓位，而不是先扫描整钱包 NFT 列表

#### Scenario: 历史 V4 空 token_id 兼容
- **WHEN** 某钱包只有历史遗留的 V4 聪明钱事件且 `token_id` 为空
- **THEN** 系统 MAY 使用 legacy NFT 扫描作为 fallback，并在无法直接给出手续费时返回降级状态

### Requirement: Smart Money 池子详情接口使用同一仓位引用模型
`GET /api/smart_money_pool_adds` MUST 对 V3/V4 使用相同的 tokenId 驱动仓位引用模型，为可识别仓位返回 best-effort 的手续费估算。

接口中的每个 wallet row SHOULD 在 `token_id` 可用时返回：
- `claimable_fee0`
- `claimable_fee1`
- `claimable_fees_usd`
- `fee_status`
- `fee_error`

当链上读取失败、历史数据缺少 `token_id` 或 RPC 不可用时：
- `fee_status` MUST 明确为 `skipped` 或 `error`
- 接口 MUST 保持其余仓位数据可用

#### Scenario: V3/V4 池子详情返回统一手续费字段
- **WHEN** 某池子的聪明钱明细行存在可识别的 `token_id`
- **THEN** `GET /api/smart_money_pool_adds` MUST 按相同字段结构返回该仓位的手续费估算，而不区分 V3 或 V4 专属响应格式
