## Context
- 钱包一键兑换已有 `SwapSingleTokenDetailedByProviderQuote`，可按 `okx` 或 `binance` 执行钱包直接发起的 token-to-token swap。
- 开仓业务里有两类兑换：
  - 钱包侧 entry swap：例如 `USDT -> entryToken`，当前使用独立 OKX swap 交易。
  - Zap 内部配比 swap：后端构造 `SwapParamsSimple`，由 `ZapSimple` 或 `AtomicIncreaseZap` 在同一笔交易内调用外部 router。
- 撤仓业务里清仓兑换属于钱包侧 swap：撤出 token0/token1 后再换回稳定币。
- `ZapSimple.sol` 与 `AtomicIncreaseZap.sol` 当前强校验 `swap.target == okxSwapRouter`，并校验 `approveTarget == okxTokenApprove`。要支持 Binance，必须把该模型改成受 owner 管理的 provider allowlist，而不是删除校验。

## Goals / Non-Goals
- Goals:
  - WebApp 与 MiniApp 开仓页面提供兑换渠道选择，默认自动择优，可限制为单一 provider。
  - 开仓 entry swap、Zap 内部配比 swap、撤仓清仓 swap 都遵循同一 provider 策略。
  - Zap 合约同时支持 OKX 与 Binance 的受信任 target/approveTarget。
  - 执行前重新获取报价/交易数据，避免使用过期 calldata 或过期 route。
  - 用户在确认和执行结果中能看到本次开仓/撤仓实际使用的 provider 与 route。
  - 保留现有余额校验、滑点、approve、receipt、到账 delta 计算和小额跳过逻辑。
- Non-Goals:
  - 不引入 0x、LI.FI 或跨链 bridge。
  - 不允许任意 router 调用；所有 Zap 内部外部调用必须在合约 allowlist 中。
  - 不用 fallback 掩盖错误；单 provider 模式下不得自动切换到另一个 provider。

## Decisions
- Decision: 增加统一 provider policy
  - 新增请求字段和任务字段，例如 `swap_provider_policy`，取值为 `best`、`okx`、`binance`。
  - `best` 为默认值，表示允许 OKX 与 Binance 同时参与择优。
  - `okx` 和 `binance` 表示单 provider 模式，只允许该 provider 报价和执行。
  - 任务创建后保存该策略，撤仓、部分撤仓、清仓 dust 默认使用任务保存的策略。

- Decision: 报价候选必须带 route 元数据
  - 内部报价结构包含 provider、providerLabel、quoteId/routeId、routeSummary、vendorName、expectedOut、estimatedGas、status、error。
  - OKX routeSummary 来自 OKX router result。
  - Binance routeSummary 使用 Binance route/vendor/dex router list 可稳定识别的字段；若 Binance 返回 vendorName，应单独保留。
  - 选择规则优先按相同 tokenOut 的 raw `expectedOut` 最大排序；当输出相同或差距小于后续配置阈值时，才比较 gas 或其他质量指标。

- Decision: 执行阶段必须重新报价
  - 预览结果只用于展示与确认。
  - 真实执行前重新查询允许集合内的 provider，并选择当前最优可执行候选。
  - 单 provider 模式下，如果该 provider 失败，直接失败并返回错误，不切换到其他 provider。
  - `best` 模式下，如果某个 provider 不可用，可选择另一个可执行 provider，但必须保留不可用 provider 的错误上下文。

- Decision: Zap 合约改为 provider allowlist
  - 保留 `SwapParams.target`、`SwapParams.approveTarget`、`SwapParams.callData` 结构，新增合约级 allowlist：
    - `trustedSwapTargets[target]`
    - `trustedApproveTargets[approveTarget]`
  - 部署或绑定初始化时默认写入 OKX 与 Binance 的 target/approveTarget。
  - owner 可启用/禁用某个 target 或 approveTarget。
  - `_executeSwap` 和 `_validateSwapParams` 必须要求 target 与 approveTarget 受信任，并继续校验 `tx.value == 0`、余额 delta、minOut。
  - 旧字段 `okxSwapRouter/okxTokenApprove` 可保留为兼容读接口，但实际校验以 allowlist 为准。

- Decision: Binance Zap calldata 必须用 Zap 地址作为执行钱包
  - 后端构造 Zap 内部 Binance calldata 时，`userWalletAddress` 必须使用 Zap 合约地址。
  - 必须拒绝 Binance 返回 native value 的路由。
  - 必须校验 `tx.to` 在合约 trusted target 中，approve spender 在 trusted approve target 中。
  - Binance route 过期时，不得复用旧 quoteId；执行前重新获取 quoteId 并 build swap transaction。

- Decision: 双端 UI 放在开仓确认区域
  - WebApp `OpenPositionModal` 与 MiniApp open position flow 增加 provider policy 选择控件。
  - 默认选中 `自动择优`。
  - entry swap 预览卡展示当前预计 provider 与 route。
  - 最终确认弹窗/确认面板展示“执行时会重新择优，结果以实际成交为准”。
  - 开仓成功、撤仓成功、部分撤仓成功的结果/进度信息展示实际 provider 与 route。

## Risks / Trade-offs
- Zap 合约 allowlist 会改变合约安全边界。
  - Mitigation: 只允许 owner 管理 target/approveTarget，测试覆盖不可信 target、approveTarget、native value、minOut 和余额 delta。
- Binance 与 OKX 报价来源口径不同，预计到账和 gas 字段可能不完全可比。
  - Mitigation: 第一阶段以相同 tokenOut 的 raw expectedOut 作为主排序，并在日志里保留 provider 与 expectedOut。
- 执行前重新择优可能和用户预览看到的 provider 不一致。
  - Mitigation: UI 明确提示执行前会重新报价；执行结果返回实际 provider 与 route。
- 旧任务没有 provider policy。
  - Mitigation: 读取旧任务时使用显式迁移默认值 `best`，这是 feature policy 默认值，不是吞错 fallback。

## Migration Plan
1. 合约：把 `ZapSimple` 与 `AtomicIncreaseZap` 的单 OKX router 校验改为 provider allowlist，补测试。
2. 部署脚本：初始化 OKX 与 Binance trusted target/approveTarget，并支持更新 trusted 列表。
3. 数据模型/API：新增 `swap_provider_policy` 请求字段和任务字段，默认 `best`。
4. 后端：实现 OKX/Binance 报价归一化、provider policy 过滤、择优和执行前重新报价。
5. 后端：开仓 entry swap、Zap 内部 swap、撤仓/部分撤仓/清仓 swap 接入统一策略。
6. WebApp：开仓页增加 provider policy 选择，预览/确认/结果展示 provider 与 route。
7. MiniApp：同步增加 provider policy 选择，预览/确认/结果展示 provider 与 route。
8. 验证：合约测试、后端单测、WebApp/MiniApp 构建和针对性 diff 检查。

## Open Questions
- 单 provider 模式是否也要在撤仓时允许用户临时覆盖任务保存的 provider policy，还是撤仓固定沿用开仓时选择？
- `best` 模式下，当 expectedOut 差距极小但 gas 差距很大时，是否需要配置一个二级阈值切换到 gas 更低的 provider？

