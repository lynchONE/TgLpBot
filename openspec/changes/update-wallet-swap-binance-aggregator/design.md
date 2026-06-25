## Context
- 现有钱包兑换 API 已经暴露 `wallet_swap_single` quote/swap 动作，前端已有 provider quote 卡片和详情展示基础。
- 后端当前代码中存在 OKX、0x、LI.FI 三类兑换适配逻辑；本次需求只要求去掉 0x 与 LI.FI，并新增 Binance 聚合报价/构造交易。
- Binance 官方文档入口：
  - `https://web3.binance.com/zh-CN/dev-docs/catalog/web3-wallet/api/rest-api/trading-api#get-aggregated-quote`
  - `https://web3.binance.com/zh-CN/dev-docs/catalog/web3-wallet/api/rest-api/trading-api#build-swap-transaction`
- 该文档页目前会触发 AWS WAF challenge，实施前需要用可访问的官方文档或账号后台确认接口路径、认证头、请求字段和响应字段，不能凭推测写死字段。

## Goals / Non-Goals
- Goals:
  - 钱包兑换报价不再请求或展示 0x、LI.FI。
  - Binance 聚合报价返回的多个 route 必须在 WebApp 与 MiniApp 中可见、可比较、可选择。
  - 执行兑换时必须基于用户选中的 Binance route 重新调用构造交易接口，避免执行过期 calldata。
  - 后端必须继续承担 token amount 精度换算、余额校验、approve、交易发送、回执确认和历史记录。
- Non-Goals:
  - 不在本次改动中重做开仓 Zap 内部 OKX swap 路由限制。
  - 不新增跨链 bridge 流程；钱包兑换仍按当前链内 token-to-token swap 处理。
  - 不保留 0x/LI.FI 的隐藏降级路径。

## Decisions
- Decision: Binance route 替代旧 provider 列表
  - 后端统一返回 `provider=binance`，并在 `routes`/`quotes` 中列出 Binance 聚合结果中的多个 route。
  - 前端默认选择可执行且净到账最高的 route；用户可以在详情里切换 route。
  - 为兼容旧前端字段，报价顶层继续保留最佳 route 的 `to_amount`、`min_to_amount`、`estimated_gas` 等摘要字段。

- Decision: 执行阶段重新构造交易
  - 客户端只回传 route 标识或 Binance 文档要求的 route 参数，不回传完整交易 calldata。
  - 后端在执行前调用 `Build Swap Transaction`，校验链、钱包、tokenIn/tokenOut、amountIn、spender、tx.to、tx.value 和 tx.data 后再发送交易。
  - 如果 route 已过期或构造失败，后端返回明确错误，前端保留用户输入并提示刷新报价。

- Decision: 移除 0x/LI.FI 代码路径
  - `wallet_swap_single` 不再调用 0x/LI.FI adapter。
  - 执行接口收到 `provider=0x`、`provider=li.fi` 或 `provider=lifi` 时必须拒绝，而不是 silently fallback 到 Binance/OKX。
  - 配置项可以先保留以降低迁移风险，但钱包兑换主链路不得再读取或依赖它们。

- Decision: route 展示由后端归一化
  - 后端将 Binance 返回的 route、DEX/协议、份额、token hop、预计到账、最少到账、Gas、错误原因归一化为前端已有 `swapProviderQuote`/route hop 可表达的结构。
  - WebApp 与 MiniApp 使用同一套响应字段展示，避免两端自行解析 Binance 原始响应。

## Risks / Trade-offs
- Binance 文档目前被 WAF challenge 拦截，字段确认存在外部依赖。
  - Mitigation: 实施第一步先确认官方接口字段，并将 adapter 测试写成可注入 HTTP client 的响应解析测试。
- Binance 构造交易返回的 spender/tx.to 可能因链和路由变化而不同。
  - Mitigation: 后端必须做链内校验、钱包地址校验、非空 tx 校验和 ERC20/native 分支校验；不得把远端返回的交易无校验发送。
- 多 route 选择会带来报价过期问题。
  - Mitigation: 执行前重新构造交易，失败时要求刷新报价。

## Migration Plan
1. 确认 Binance 官方接口字段、认证方式和错误响应。
2. 新增 Binance exchange adapter 与响应归一化测试。
3. 替换后端钱包兑换报价/执行链路，移除 0x/LI.FI 主链路。
4. 更新 WebApp 与 MiniApp 的路由展示和选中 route 提交逻辑。
5. 运行后端单测与两端构建，并做针对性 diff 检查。

## Open Questions
- Binance `Build Swap Transaction` 是否要求传入 `quoteId`、完整 route payload，还是 route id + quote id；实施前必须以官方文档为准。
- Binance 聚合 API 的 API Key、签名头、时间戳和权限是否与 Binance Web3 Wallet 其他 API 共用；实施前必须确认。
