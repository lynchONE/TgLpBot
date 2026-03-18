# 变更：用统一的 HTTP RPC 监控链路替换 Smart Money Explorer 扫描方案

## Why
当前 Smart Money 把链上监控拆成两条链路：
- `watcher` 监听已监控钱包的 LP 事件；
- `crawler` 通过 Explorer `txlist` 扫描监控合约并发现新钱包。

其中 Explorer 方案在 BSC 上已经不再可靠，导致监控合约的 `last_scanned_block` 无法持续推进，前端长期显示“未扫描”。同时，这套双链路设计也与项目现有的“RPC 池优先，`.env` 回退”基础设施不一致。

## What Changes
- 删除 Smart Money 基于 Explorer `txlist` 的钱包发现方案。
- 删除 Smart Money 中独立的 `crawler` 运行链路，统一为单一的 HTTP RPC 轮询监控链路。
- 统一链路在同一轮区块处理中同时完成两类工作：
  - 扫描 LP PositionManager 相关事件，更新已监控钱包的 LP 事件和仓位状态；
  - 扫描区块内直接发送到监控合约的交易，提取发送方钱包并加入监控列表。
- 统一链路的 RPC 解析遵循“RPC 池优先，`.env` 回退”的统一规则。
- 明确 `last_scanned_block=0` 的初始化语义：初始化到当前最新区块，后续只做增量推进。
- 删除 Smart Money 运行时对 `BSCSCAN_API_KEY`、`ETHERSCAN_API_KEY`、`SMART_MONEY_CRAWLER_*`、`SMART_MONEY_RECONNECT_INTERVAL_SEC` 的依赖说明。

## Impact
- Affected specs:
  - `smart-money-monitoring`
- Affected code:
  - `backend/service/smart_money/`
  - `backend/service/web_server/smart_money.go`
  - `backend/base/config/config.go`
  - `backend/.env.example`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
- **BREAKING**:
  - Smart Money 合约发现不再依赖 Explorer API，也不再保留独立 `crawler` 方案。
  - Smart Money 统一改为 HTTP RPC 轮询模式，不再依赖 WebSocket 订阅作为运行前置条件。
