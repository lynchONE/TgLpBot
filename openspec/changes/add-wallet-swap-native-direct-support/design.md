## Context
- 现有钱包兑换链路默认按 ERC20 处理输入和输出资产。
- OKX 在原生币场景下会返回带 `tx.value` 的交易，这与 ERC20 授权模式不同。

## Goals / Non-Goals
- Goals:
  - 支持原生 `BNB/ETH -> ERC20`
  - 支持 `ERC20 -> 原生 BNB/ETH`
  - 保持现有 ERC20 兑换链路兼容
- Non-Goals:
  - 不改动开仓 Zap 内部的 swap 路由限制
  - 不新增额外的 wrap/unwrap 中间步骤

## Decisions
- Decision: 继续使用 OKX `/swap` 构造交易，但对原生币伪地址单独处理。
  - 输入是原生币时，直接发送 OKX 返回的 `tx.value`，不再请求 approve。
  - 输出是原生币时，到账校验改为读取钱包 native balance，并把 gas 成本加回后计算净到账。
- Decision: 前端常用代币列表显式加入链原生币入口。
  - 用户既可以从钱包余额里点原生币，也可以直接在常用代币里选择原生币作为 `From/To`。

## Risks / Trade-offs
- 原生币到账校验天然会受 gas 支出影响。
  - Mitigation: 使用 `after - before + gasCost` 作为净到账估算。
- 旧的 hash-only OKX helper 仍保留在代码里。
  - Mitigation: 钱包单币兑换主链路改为复用新的原生币兼容执行器。

## Migration Plan
1. 放开前后端对 `0xeeee...` 的限制。
2. 更新 OKX 执行器，支持 native `tx.value` 和 native balance 校验。
3. 前端增加原生币入口并重新部署。

## Open Questions
- 暂无。
