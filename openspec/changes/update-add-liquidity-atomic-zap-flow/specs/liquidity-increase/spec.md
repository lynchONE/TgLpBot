## ADDED Requirements

### Requirement: 补充流动性 MUST 以单笔原子 Zap 执行
系统 MUST 在 `task_add_liquidity` 中通过单笔链上 Zap 交易完成补仓所需的换币与加仓，不得先落地独立的前置 swap 再执行补仓。

#### Scenario: 池子不包含稳定币时 V4 补仓仍然只发送一笔交易
- **GIVEN** 用户对一个 `Uniswap V4` 任务发起补仓
- **AND** 该池子不包含链上稳定币，需要先把稳定币换成 entry token
- **WHEN** 后端执行本次补仓
- **THEN** 系统 SHALL 构造一笔原子 Zap 交易，在同一笔交易内完成 `stable -> entry token`、必要的配比换币和 `modifyLiquidities`
- **AND** MUST NOT 先广播独立的 OKX swap 交易

#### Scenario: 池子包含稳定币时 V3 补仓使用单笔 zap increase
- **GIVEN** 用户对一个 `Uniswap V3` 任务发起补仓
- **AND** 该池子已经包含链上稳定币
- **WHEN** 后端执行本次补仓
- **THEN** 系统 SHALL 通过单笔 `zapIncreaseV3` 交易完成补仓
- **AND** MUST NOT 在补仓前额外发送独立换币交易

### Requirement: 原子补仓失败 MUST 不留下已成交的前置换币
当补仓模拟、gas 估算或链上执行失败时，系统 MUST 不留下“换币已经成交但补仓未完成”的独立前置换币结果。

#### Scenario: estimateGas 阶段发现补仓会回滚
- **GIVEN** 本次原子补仓在 `eth_estimateGas` 阶段会触发 revert
- **WHEN** 后端处理补仓请求
- **THEN** 系统 MUST 直接返回该错误
- **AND** MUST NOT 广播独立的前置 swap 交易
- **AND** MUST NOT 形成部分完成的补仓状态

#### Scenario: 链上执行整笔交易回滚
- **GIVEN** 原子补仓交易已经广播
- **AND** 该交易最终在链上回滚
- **WHEN** 后端处理失败结果
- **THEN** 系统 MUST 将本次补仓视为整笔失败
- **AND** 资金状态 MUST 不保留 swap 成功但加仓失败的中间结果

### Requirement: 原子补仓 MUST 校验仓位所有权、NFT 授权与最新区间
系统 MUST 在原子补仓中校验 `tokenId` 的所有权、Zap 对 NFT 的授权状态，并尽量使用最新链上区间元数据构建补仓参数。

#### Scenario: NFT 未授权给原子 Zap
- **GIVEN** 用户钱包持有目标 `tokenId`
- **AND** 该 `tokenId` 尚未授权给当前原子 Zap 地址
- **WHEN** 用户执行补仓
- **THEN** 系统 MUST 在主补仓交易前先完成 NFT 授权，或返回明确的 NFT 授权错误
- **AND** MUST NOT 广播任何独立 swap 交易

#### Scenario: 任务缓存区间与链上区间不一致
- **GIVEN** 任务表中的 `tickLower/tickUpper` 与链上真实仓位区间不一致
- **WHEN** 后端构造原子补仓请求
- **THEN** 系统 MUST 优先使用最新链上区间做模拟和执行
- **AND** 在补仓成功后 MUST 回写同步后的区间信息

### Requirement: 原子补仓结果 MUST 以实际消耗、退款与主交易哈希记账
系统 MUST 根据原子 Zap 实际消耗、退款、dust、gas 和主交易哈希更新任务状态与交易记录，不得继续按用户请求的稳定币金额直接累计本金。

#### Scenario: 补仓成功但有部分金额作为 dust 退回
- **GIVEN** 本次原子补仓成功
- **AND** Zap 在执行后向钱包退回了部分未使用金额
- **WHEN** 后端写入交易记录和任务状态
- **THEN** 系统 MUST 按实际稳定币消耗更新投入金额
- **AND** MUST 单独保留实际 dust 与 gas 信息
- **AND** MUST 返回本次原子补仓的主交易哈希
