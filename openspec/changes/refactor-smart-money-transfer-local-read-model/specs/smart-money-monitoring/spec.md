## ADDED Requirements

### Requirement: Smart Money 监控链路必须持久化已监控钱包的普通转账事件
Smart Money 统一监控链路 SHALL 在增量区块轮询过程中持续解析并持久化已激活聪明钱包的普通转账事件，作为资产管理和盈利日历的本地读模型数据源。

#### Scenario: 监控钱包发生 ERC20 转账
- **GIVEN** 某钱包已经处于激活监控状态
- **WHEN** watcher 扫描到包含该钱包 ERC20 `Transfer(address,address,uint256)` 事件的区块窗口
- **THEN** 系统将该转账按钱包方向解析并写入本地转账事件表
- **AND** 该记录可被后续日聚合直接复用

#### Scenario: 监控钱包发生原生币转账
- **GIVEN** 某钱包已经处于激活监控状态
- **WHEN** watcher 扫描到该钱包作为 `from` 或 `to` 且 `value > 0` 的原生币交易
- **THEN** 系统将该笔原生币转账写入本地转账事件表
- **AND** 不要求等待资产接口读取时再补扫链上交易

### Requirement: Smart Money 普通转账持久化必须排除 LP 同 tx 内部流转
Smart Money 监控链路 SHALL 排除 LP add/remove 同一交易内产生的内部 token 流转，不得把这些为做市服务的转账误记为普通钱包转账事件。

#### Scenario: 钱包执行 LP add 或 remove
- **WHEN** watcher 处理到某笔已经被识别为 LP add/remove 的交易
- **THEN** 同一 tx 内由 LP 操作引起的 token 流转不得写入普通转账事件表
- **AND** 该交易仍然继续按现有 LP 事件链路入库
