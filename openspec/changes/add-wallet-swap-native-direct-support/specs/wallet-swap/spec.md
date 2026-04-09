## ADDED Requirements
### Requirement: 钱包一键兑换 MUST 支持 EVM 原生币直兑
系统 MUST 在钱包一键兑换中支持 EVM 原生币作为输入或输出资产，不得强制用户先手动换成 wrapped native token。

#### Scenario: 使用原生币作为卖出资产
- **WHEN** 用户在 `webapp` 一键兑换里选择原生 `BNB/ETH` 作为 `From`
- **THEN** 系统 MUST 允许生成 OKX 报价
- **AND** MUST 使用 OKX 规定的原生币伪地址 `0xeeee...`
- **AND** 在真实执行时 MUST 发送 OKX 返回的 `tx.value`
- **AND** MUST NOT 对原生币执行 ERC20 approve

#### Scenario: 使用原生币作为买入资产
- **WHEN** 用户在 `webapp` 一键兑换里选择原生 `BNB/ETH` 作为 `To`
- **THEN** 系统 MUST 允许生成 OKX 报价
- **AND** 在执行后 MUST 使用钱包 native balance 口径校验到账结果

#### Scenario: 钱包余额列表展示原生币可兑换
- **WHEN** OKX 余额接口返回原生 `BNB/ETH` 且其价值达到显示阈值
- **THEN** 钱包余额区 MUST 展示该原生币
- **AND** MUST 允许用户直接点击作为兑换资产
