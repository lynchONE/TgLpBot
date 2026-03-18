# 变更：移除 ClickHouse 并仅保留 Pools 业务

## Why
当前项目中的 ClickHouse 同时承载 `/api/pools`、Hot Pools、Smart Money、SmartLP、AutoLP/PoolM 等多条分析链路，维护和演进成本较高。

现需求是只保留 `pools` 业务，其余所有依赖 ClickHouse 的业务、代码、配置和前端入口全部删除；同时将池子数据改为保存到 MySQL，并让保留的业务统一读取 MySQL。

## What Changes
- 新增 MySQL 版 `pools` 持久化方案，复用现有外部抓取逻辑拉取池子数据并写入 MySQL；`/api/pools` 及保留的池子读取逻辑改为读 MySQL。
- 删除 ClickHouse 初始化、配置项、环境变量、依赖包、Schema/Migration 与 `base/clickhouse` 目录。
- 删除所有仅依赖 ClickHouse 的后端业务与 API，包括 Hot Pools、Smart Money、SmartLP、AutoLP/PoolM 相关服务、任务、路由和测试。
- 删除 `miniapp/`、`webapp/` 中所有依赖上述已删除业务的页面、组件、代理接口和菜单入口，仅保留仍与 pools 业务相关的能力。
- 删除 Telegram Bot 中与 Smart Money / AutoLP 相关的命令、菜单和回调入口。
- **BREAKING**：`/api/hot_pools`、`/api/smart_money*`、`/api/auto_monitor`、`/api/autolp_*` 等入口全部下线，不再保留兼容分支。

## Impact
- Affected specs: `clickhouse-removal`
- Affected code:
  - `backend/base/clickhouse/`
  - `backend/base/config/config.go`
  - `backend/base/models/`
  - `backend/main.go`
  - `backend/go.mod`
  - `backend/.env.example`
  - `backend/service/auto_lp/`
  - `backend/service/web_server/server.go`
  - `backend/service/web_server/hot_pools.go`
  - `backend/service/web_server/smart_money_*.go`
  - `backend/service/web_server/auto_monitor.go`
  - `backend/service/smart_lp/`
  - `backend/service/smart_money_follow/`
  - `backend/service/smart_money_golden_dog/`
  - `backend/service/bot/`
  - `miniapp/api/`
  - `miniapp/src/`
  - `webapp/api/`
  - `webapp/src/`
- Related changes:
  - 本提案覆盖并扩展 `openspec/changes/remove-autolp-mode/` 的删除范围；后续应以本提案为准推进实现。
