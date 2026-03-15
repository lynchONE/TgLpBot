# Change: Update Smart Money 仓位 tokenId 追踪与统一查询链路

## Why
- 当前 `smart_lp_events` 虽然已经有 `token_id` 字段，但只有 V3 扫描链路会稳定写入；V4 `ModifyLiquidity` 事件当前把 `salt` 解出来后直接丢弃，导致 `token_id` 为空。
- 这直接造成聪明钱模块在查询当前仓位时走了两套主路径：V3 依赖事件中的 `token_id`，V4 则需要额外扫描 PositionManager 的 ERC721 `Transfer` 日志再反查，池子详情里的手续费估算也只有 V3 做了特例支持，V4 缺失。
- 用户的目标是把“钱包 + LP NFT tokenId”作为统一仓位标识保存下来，这样聪明钱模块就能直接查询当前仓位明细与未领取手续费，并让 V3/V4 的思路一致。

## What Changes
- SmartLP 扫描：
  - V4 解析 `ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)` 的 `salt`，并将其按十进制字符串写入 `smart_lp_events.token_id`。
  - V3 继续沿用现有 `token_id` 写入逻辑，不再把 V3 视为特殊方案，后续查询链路与 V4 对齐。
- Backend 查询链路：
  - 为聪明钱模块新增统一的“position ref”读取逻辑，优先从 `smart_lp_events` 直接得到 `(pool_version, pool_id, token_id)` 并据此查询当前仓位。
  - `GET /api/smart_money_wallet_positions` 统一按 `token_id` 加载 V3/V4 当前仓位，并返回 best-effort 的手续费字段。
  - `GET /api/smart_money_pool_adds` 统一按相同仓位引用模型计算 V3/V4 的当前手续费估算。
  - V3 手续费计算从现有 `collect` 模拟切到 `positions(tokenId)` + pool fee growth 计算，和 V4 保持同一类主思路。
- 兼容性：
  - 对历史 V4 事件中仍为空的 `token_id`，保留 legacy fallback（扫描 ERC721 持仓）作为兼容路径，但不再作为新数据的主方案。
  - 接口以新增字段为主，不破坏现有客户端的基础展示。

## Impact
- 影响规格：`miniapp-smart-money`
- 影响代码：
  - `backend/service/smart_lp/smart_lp_monitor.go`
  - `backend/service/web_server/smart_money_wallet_positions.go`
  - `backend/service/web_server/smart_money_pool_adds.go`
  - `backend/service/pool/v3_fees.go`
  - `backend/service/pool/v4_fees.go`
  - 相关 Smart Money 测试文件
- 数据影响：
  - 无需新增表或列
  - `smart_lp_events.token_id` 对 V4 的填充语义会从“通常为空”变为“新采集事件默认可用”
- 风险：
  - 手续费和当前仓位查询会增加 RPC 读取压力，需要继续保留并发限制、超时和缓存
  - 历史 V4 数据不会自动补全 `token_id`，因此旧窗口内的部分事件仍可能只能走兼容路径
