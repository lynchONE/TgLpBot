## Context
- 当前 `wallet_swap_single` 的报价与执行流程都固定调用 OKX，返回模型只覆盖单 provider。
- 这次需求同时引入 0x 与 LI.FI，三家官方接口在报价字段、手续费表达、路径结构和执行交易数据格式上都不一致。
- 已核对的官方能力如下：
  - 0x Allowance Holder Quote 返回 `fees`、`route.fills`、`issues.allowance.spender` 和 `transaction`，并要求 `0x-api-key` 与 `0x-version: v2`。
  - LI.FI `GET /v1/quote` 返回 `estimate.toAmount`、`estimate.feeCosts`、`estimate.gasCosts`、`includedSteps` 与 `transactionRequest`，支持通过 `fee` 与 `integrator` 传入手续费配置。
  - OKX Swap 返回 `routerResult.dexRouterList`、`quoteCompareList` 与 `tx`，可以用于路径摘要展示。

## Goals / Non-Goals
- Goals:
  - 在 `webapp` 同链单币兑换中同时展示 OKX、0x、LI.FI 报价。
  - 页面仅展示 OKX 的兑换路径，不展示 0x 与 LI.FI 的兑换路径。
  - 使用“净到手”而不是“裸路由返回值”作为前端比较与默认排序口径。
  - 支持在确认弹窗中选定 provider，并按该 provider 执行最终兑换。
- Non-Goals:
  - 不在本次改动中新增跨链 bridge UI 或跨链执行流程。
  - 不改造 `miniapp` 的兑换页面。
  - 不恢复旧的批量扫描余额后一键兑换模式。

## Decisions
- Decision: 引入统一的 provider quote 领域模型
  - 后端统一输出 provider 标识、展示名称、净到手、最小到账、手续费条目、Gas 条目、路径摘要、可执行状态和 provider 错误。
  - `net_to_amount` 作为前端比较和默认排序的主字段；`gross_to_amount` 作为可选辅助字段，仅在 provider 能可靠给出时返回。

- Decision: 手续费处理采用“provider adapter 自解释”模式
  - 0x：读取官方返回的 `fees.zeroExFee` / `fees.integratorFee`，并将 quote 返回的用户实际到账值作为 `net_to_amount`。
  - LI.FI：请求时带上 `fee=0.0025` 与 `integrator`，读取 `estimate.feeCosts` 与 `estimate.toAmount` 作为净到手结果。
  - OKX：由于当前业务规则是“仅对正滑点部分收取 10%”，而报价阶段未必能精确得到正滑点金额，因此以 API 返回的可承诺到账值作为 `net_to_amount`，并单独返回 `fee_rule` 供前端展示，不把不可保证的正滑点收益计入排序。

- Decision: 执行阶段按所选 provider 重新获取可执行交易数据
  - 不让前端持有或回传完整原始交易数据。
  - 后端继续负责 amount 精度换算、allowance / approve、交易校验和错误转换。
  - 这样可以避免报价过期后直接提交旧 calldata，也便于统一服务端安全控制。

- Decision: 初期仅支持同链兑换的多 provider 能力
  - `webapp` 当前的一键兑换交互是链内 token-to-token swap。
  - LI.FI 在本次改动中固定使用 `fromChain == toChain` 的同链 quote，不开放 bridge 路径。

- Decision: 路径展示做统一归一化，但按 provider 控制可见性
  - OKX 路径来自 `routerResult.dexRouterList/subRouterList/dexProtocol`。
  - 0x 路径来自 `route.fills`，前端展示为 hop/source/proportion 摘要。
  - LI.FI 路径来自 `includedSteps` 与 `estimate.data.protocols`。
  - `webapp` 仅展示 OKX 的路径摘要；0x 与 LI.FI 报价即使后端保留归一化路径字段，也 MUST NOT 在报价卡片、详情区或确认弹窗中展示。

- Decision: 部分 provider 失败不阻断整体报价
  - 只要至少一个 provider 报价成功，接口就返回成功响应。
  - 对失败 provider 返回明确状态和错误原因，供前端在卡片内展示“不可用”。

## Risks / Trade-offs
- 不同 provider 的手续费可能以不同 token 表示，前端不能假设所有 fee 都能直接换算成目标币；排序必须优先使用后端给出的 `net_to_amount`。
- 0x 与 LI.FI 的执行交易都依赖远端返回的 `transaction` / `transactionRequest`，安全校验范围天然弱于当前 OKX 的固定 allowlist 模式，需要在实现时补齐同链、钱包地址、spender/approve 目标和必填字段校验。
- 执行前重新取价会导致“页面预览”和“最终成交”存在轻微漂移，前端需要在 provider 失效时保留用户输入并提示刷新报价。

## Migration Plan
1. 增加 0x / LI.FI 配置项与 provider adapter。
2. 扩展 `wallet_swap_single` quote / swap 请求和响应模型。
3. 改造 `SwapPanel` 展示与选择逻辑。
4. 通过后端单测和 `webapp` 构建验证新协议。

## Open Questions
- 无。当前按“仅改造 `webapp` 同链一键兑换”这一范围落地；若后续需要把同一能力同步到 `miniapp`，可在独立变更中处理。
