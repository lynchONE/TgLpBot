# 变更：移除 AutoLP 自动开单模式

## Why
当前系统仍提供 AutoLP 自动开单模式，包括池子扫描、候选评估、自动开仓、换仓、门禁/硬筛、Telegram `/auto` 菜单以及 MiniApp 对应监控与管理入口。

现需求是将该模式整体下线，避免系统继续基于扫描结果自动开单，也避免用户继续看到已经废弃的 AutoLP 配置、监控与管理能力。

## What Changes
- 删除后端 AutoLP 服务与运行链路，包括 PoolM 扫描、候选评估、自动开仓、换仓、跳过通知、统计与管理员强制关闭能力。
- 删除 Telegram Bot 中的 `/auto` 命令、AutoLP 配置回调、输入态处理与帮助文案。
- 删除 Web API 与 MiniApp 中所有 AutoLP 相关入口，包括配置、监控、盈利曲线、管理员统计/关闭入口，以及实时仓位中的 Auto 专属标签与展示。
- 删除策略层中基于 AutoLP 池子扫描结果判断“是否满足再次开单/再平衡开单”的硬筛检查逻辑。
- 保留手动开单、普通仓位展示、SmartLP 与其他非 AutoLP 能力。
- **BREAKING**：现有 AutoLP 相关命令、API、页面与自动开单行为全部移除。

## Impact
- Affected specs: `auto-lp-removal`
- Affected code:
  - `backend/service/auto_lp/`
  - `backend/service/bot/bot.go`
  - `backend/service/bot/auto_handlers.go`
  - `backend/service/bot/auto_config_callbacks.go`
  - `backend/service/bot/auto_config_input_handlers.go`
  - `backend/service/strategy/strategy_exit_retry.go`
  - `backend/service/strategy/hard_filter.go`
  - `backend/service/web_server/autolp_config.go`
  - `backend/service/web_server/autolp_pnl_curve.go`
  - `backend/service/web_server/admin_auto_lp.go`
  - `backend/service/web_server/auto_monitor.go`
  - `miniapp/src/App.jsx`
  - `miniapp/src/lib/api.js`
  - `miniapp/src/components/AutoMonitorCard.jsx`
  - `miniapp/src/components/AutoPnLCurveCard.jsx`
  - `miniapp/src/components/AdminPage.jsx`
  - `miniapp/api/settings.js`
  - `miniapp/api/positions.js`
  - `miniapp/api/admin.js`
