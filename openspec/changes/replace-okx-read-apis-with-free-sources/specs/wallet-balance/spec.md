## ADDED Requirements

### Requirement: 钱包余额查询不得依赖 OKX balance API
钱包余额查询 MUST 使用 RPC 作为默认数据源，并 MUST NOT 调用 OKX `all-token-balances-by-address`。

#### Scenario: 查询钱包原生币余额
- **WHEN** 系统需要某个钱包在某条链上的原生币余额
- **THEN** 后端 MUST 通过 RPC `eth_getBalance` 查询

#### Scenario: 查询已知 token 余额
- **WHEN** 系统需要钱包的 ERC20/BEP20 token 余额
- **THEN** 后端 MUST 从项目已知 token 集中选择候选 token
- **AND** MUST 通过 RPC 调用 `balanceOf` 校验余额

### Requirement: 已知 token 集必须覆盖项目业务 token
钱包余额 RPC 扫描的已知 token 集 MUST 至少包含稳定币、wrapped native、任务 token、用户交易历史 token、钱包兑换限价单 token 和热门池 token。

#### Scenario: 用户曾交易过某个 token
- **WHEN** 用户钱包兑换或 LP 业务历史中出现某个 token
- **THEN** 该 token MUST 进入钱包余额已知 token 集
- **AND** 后续余额查询 MUST 能通过 RPC 扫描该 token

### Requirement: 第三方钱包 API 只能作为可选发现增强
第三方钱包 API MAY 作为候选 token 发现来源，但 MUST NOT 成为钱包余额查询的默认强依赖。第三方发现出的非零余额 MUST 再通过 RPC 校验。

#### Scenario: 启用第三方发现
- **WHEN** 配置启用了 Alchemy、Moralis、GoldRush/Covalent、DeBank、Zerion 或同类钱包 API
- **THEN** 后端 MAY 使用第三方接口发现候选 token
- **AND** MUST 对第三方返回的非零余额执行 RPC `balanceOf` 校验

#### Scenario: 第三方发现失败
- **WHEN** 第三方钱包 API 超限、失败或未配置
- **THEN** 后端 MUST 继续返回 RPC 已知 token 集扫描结果
- **AND** MUST NOT 返回伪造的空余额来掩盖第三方失败
