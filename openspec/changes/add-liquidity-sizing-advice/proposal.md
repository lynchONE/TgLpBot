# Change: 新增加仓金额建议与边际效率控制能力

## Why
- 当前系统已有开仓、补仓和开仓前流动性风控，但缺少一个“在开仓前先算这笔钱值不值得开、是否过大、合理建议是多少”的统一建议能力。
- 用户希望继续沿用现有开仓交互，只填写金额、区间、滑点等已有参数；参与建议计算的价格、tick、活跃流动性、资金规模和风险约束应由系统自动获取，而不是再让用户手填。
- 如果用户只是把金额一路调大，系统目前只能做基础流动性/价格偏差拦截，无法回答“为了拿更高区间占比是否已经进入低效率区间”。需要一个可解释的金额建议层，补上这块决策能力。

## What Changes
- 在现有开仓链路中新增“仓位规模建议”能力，优先集成到 `open_position_preview` 这类预检响应中，而不改变用户现有输入方式。
- 系统自动解析建议计算所需输入，而不是要求用户显式传参：
  - `price` / `current_tick`：从链上 slot0 / current tick 与现有池子快照获取
  - `tick_lower` / `tick_upper`：继续根据用户输入的区间百分比，沿用现有 `TickCalculator` 反推
  - `L_active`：优先读取 `pools.active_liquidity_usd`，失败时回退到池子快照或 DexScreener 流动性
  - `capital_total`：按当前选中钱包在当前链上的可用 stable 资金规模估算，不要求用户手输“总资金”
  - `risk_cap_per_position`、`target_share_min/max`：从配置读取，优先用户级默认值，缺省回退系统级默认值
- 建立统一计算口径：
  - 占比公式：`share = user_liquidity / (L_active + user_liquidity)`
  - 反推目标加仓：`user_liquidity = L_active * target_share / (1 - target_share)`
  - 当目标占比请求超过 `0.8` 时，强制截断到 `0.8`，并输出边际效率警告。
  - 最终建议金额同时受 `risk_cap_per_position` 和 `capital_total` 约束。
- 输出结果除推荐金额外，还必须返回：
  - 对应的预期占比
  - 最大可能单边资产风险暴露
  - 边际效率评价（`high` / `medium` / `low`）
  - 警告信息与关键计算说明
- 用户发起真正开仓时，现有开仓执行请求保持不变；V1 先提供建议与预警，不在后台静默篡改用户输入金额。

## Impact
- Affected specs:
  - `liquidity-sizing-advice`
- Affected code:
  - `backend/service/liquidity/*`
  - `backend/service/web_server/*`
  - `backend/service/user/*`
  - `backend/base/models/*`
  - 相关单元测试

## Assumptions
- `L_active`、`capital_total`、`risk_cap_per_position` 和返回的 `liquidity_to_add`、`risk_exposure` 在 V1 中统一按 U 计价。
- V1 中自动解析的 `capital_total` 以“当前选中钱包在当前链上的可用 stable 资金规模”为准，而不是跨钱包、跨链的总资产值；因为实际开仓执行路径本身也是从当前钱包 stable 资金发起。
- `risk_cap_per_position` 的配置允许两种语义：
  - 固定金额上限，例如 `500U`
  - 资金比例上限，例如 `20%`
  实际有效上限取两者中的更严格值。
- 输入中的 `price`、`tick_lower`、`tick_upper` 在 V1 中主要用于上下文、展示与基础校验；占比反推主公式仍以 `L_active` 为核心。

## Risks / Trade-offs
- “最大可能单边资产”在缺少更细粒度 token 构成、波动假设与价格路径时，只能采用保守近似：按“本次投入金额可能最终全部集中为单边资产”计算。
- 保守 / 中性 / 激进三档与配置中的 `target_share_min/max` 可能发生压缩或重叠；V1 会优先保证输出三档结构稳定，再在说明中标出目标被约束的原因。
- 若池子快照缺失或钱包资产读取异常，建议能力需要 best-effort 降级；不能因为建议数据暂时不可用而破坏现有开仓主链路。
- 该能力是建议值而非成交承诺值，后续若要与链上开仓执行严格联动，需要单独处理实时价格、滑点、token 配比和 quote 时效问题。
