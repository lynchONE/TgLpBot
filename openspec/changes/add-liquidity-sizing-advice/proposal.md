# Change: 新增加仓金额建议与边际效率提示

## Why
- 当前系统已经有开仓、补仓和基础风控，但缺少一个“按当前区间活跃流动性推算不同占比需要投入多少资金”的统一建议能力。
- 用户希望继续沿用现有开仓交互，只填写金额、区间、滑点等已有参数；参与建议计算的价格、tick 和活跃流动性应由系统自动获取，而不是再让用户手填。
- 建议结果需要与当前钱包可用余额解耦，否则会把三档建议压成同一个值，失去“保守 / 中性 / 激进”分档意义。

## What Changes
- 在现有开仓预检链路中新增“仓位规模建议”能力，优先集成到 `open_position_preview` 响应中，不改变用户现有输入方式。
- 系统自动解析建议计算所需输入，而不要求用户显式传参：
  - `price` / `current_tick`：从链上读数与现有池子快照获取
  - `tick_lower` / `tick_upper`：继续根据用户输入的区间百分比反推
  - `L_active`：优先读取池子快照中的 `active_liquidity_usd`，失败时回退到其他池子流动性估值来源
  - `target_share_min/max`：从配置读取，优先用户级默认值，缺省回退系统级默认值
- 建立统一计算口径：
  - 占比公式：`share = user_liquidity / (L_active + user_liquidity)`
  - 反推目标加仓：`user_liquidity = L_active * target_share / (1 - target_share)`
  - 当目标占比请求超过 `0.8` 时，强制截断到 `0.8`，并输出边际效率警告
- 三档建议只基于活跃流动性和目标占比推导，不再用当前钱包 stable 余额、总资金上限或比例风控去截断推荐金额。
- 输出结果除推荐金额外，还返回：
  - 对应的预期占比
  - 最大可能单边资产风险暴露
  - 边际效率评价（`high` / `medium` / `low`）
  - 警告信息与关键计算说明

## Impact
- Affected specs:
  - `liquidity-sizing-advice`
- Affected code:
  - `backend/service/liquidity/*`
  - `backend/service/web_server/*`
  - `backend/service/user/*`
  - `backend/base/models/*`
  - `webapp/src/components/*`
  - `miniapp/src/*`

## Assumptions
- `L_active`、返回的 `liquidity_to_add` 和 `risk_exposure` 在 V1 中统一按 U 计价。
- 三档默认目标分别为 `0.20 / 0.40 / 0.65`，再夹到配置中的 `[target_share_min, target_share_max]` 区间内。
- `price`、`tick_lower`、`tick_upper` 在 V1 中主要用于上下文、展示与基础校验；占比反推核心仍以 `L_active` 为主。

## Risks / Trade-offs
- 建议结果与钱包余额解耦后，某些档位可能高于当前钱包当下可执行预算；这是有意保留的“规模建议”，不是自动代下单金额。
- “最大可能单边资产”在缺少更细粒度 token 构成和价格路径输入时，只能采用保守近似：按“本次投入金额可能最终全部集中为单边资产”计算。
- 如果活跃流动性估值本身不准，建议金额也会随之偏移；V1 通过返回数据来源和 warning 来降低误解风险。
