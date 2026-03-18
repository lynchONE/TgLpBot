## ADDED Requirements

### Requirement: Smart Money 合约发现不得依赖 Explorer 交易列表接口
Smart Money 合约监控 SHALL 使用 BSC 原生 HTTP RPC 数据源发现监控合约相关钱包，不得再依赖 BscScan、Etherscan 或其他 Explorer 的 `txlist` 风格接口作为运行时前置条件。

#### Scenario: 启动合约监控
- **WHEN** Smart Money 统一监控链路启动并存在可用的 BSC HTTP RPC
- **THEN** 系统使用 RPC 数据源执行监控合约扫描，而不要求配置 `BSCSCAN_API_KEY` 或 `ETHERSCAN_API_KEY`

#### Scenario: Explorer key 缺失
- **WHEN** Smart Money 运行环境未配置任何 Explorer API key
- **THEN** 只要 BSC HTTP RPC 可用，监控合约扫描仍然可以启动

### Requirement: Smart Money 必须使用统一的 HTTP 轮询监控链路
Smart Money SHALL 使用同一条基于 HTTP RPC 轮询的链上监控链路处理 LP 事件监听和监控合约钱包发现，不得再保留彼此独立的 watcher/crawler 双链路，也不得依赖 WebSocket 订阅作为运行前置条件。

#### Scenario: 轮询发现新区块
- **WHEN** 统一监控链路通过 HTTP RPC 轮询发现新区块
- **THEN** 系统在同一轮区块处理中同时执行 LP 事件处理和监控合约交易处理

#### Scenario: 统一链路恢复
- **WHEN** 监控链路因 HTTP RPC 故障或进程重启恢复
- **THEN** 系统基于同一套区块游标与回补逻辑恢复两类处理，而不是分别维护独立恢复流程

### Requirement: Smart Money 合约监控必须遵循 RPC 池优先规则
Smart Money 统一监控链路 SHALL 在每轮处理前优先解析 RPC 池中的当前 HTTP 节点；只有 RPC 池没有可用节点时，才允许回退到 `.env` 中的 BSC HTTP RPC 配置。

#### Scenario: RPC 池存在 current HTTP 节点
- **WHEN** RPC 池中存在 BSC 的可用 current HTTP 节点
- **THEN** Smart Money 统一监控链路使用该节点执行本轮扫描

#### Scenario: RPC 池为空
- **WHEN** RPC 池中没有可用的 BSC HTTP 节点
- **THEN** Smart Money 统一监控链路回退使用 `.env` 中的 BSC HTTP RPC 配置

### Requirement: Smart Money 合约监控必须持续推进区块游标
Smart Money 合约监控 SHALL 维护每个监控合约的 `last_scanned_block`，并在成功完成一个区块范围扫描后推进到已完成的结束区块。

#### Scenario: 正常推进游标
- **WHEN** 某个监控合约成功完成一次区块范围扫描
- **THEN** 该合约的 `last_scanned_block` 被更新为本次已完成扫描范围的结束区块

#### Scenario: 首次初始化游标
- **WHEN** 某个监控合约的 `last_scanned_block=0`
- **THEN** 系统将其初始化到当前最新区块，并从后续新区块开始执行增量扫描

### Requirement: Smart Money 合约钱包发现基于监控合约直接交易
Smart Money 统一监控链路 SHALL 基于直接发送到监控合约地址的交易识别发送方钱包，并将这些钱包加入监控列表。

#### Scenario: 监控合约收到直接交易
- **WHEN** 某笔交易直接发送到监控合约地址
- **THEN** 系统解析该交易的发送方钱包，并将其作为候选监控钱包写入监控列表

#### Scenario: 监控合约没有新交易
- **WHEN** 某个扫描区块范围内监控合约没有任何新交易
- **THEN** 系统不新增钱包，但仍按已完成范围推进游标
