# Change: Add Smart Money "金狗通知" tab + Bark alerts

## Why
- Smart Money 模块希望更快捕捉“多个钱包同时加 LP”的热点池子（潜在金狗）。
- Telegram 内看盘时，用户更需要被动提醒而不是手动刷新页面。
- Bark 已在全局配置中提供通知地址（Key/Server/Group），可复用减少配置成本。

## What Changes
- MiniApp:
  - Smart Money 模块新增一个 Tab：`金狗通知`。
  - 在该 Tab 中提供配置：
    - 启用/停用金狗提醒
    - 触发阈值：同一池子在指定时间窗口内“加 LP 的不同钱包数”达到 `N` 时触发
    - 时间窗口（分钟）与冷却时间（分钟）（避免同一池子反复刷屏）
  - Bark 通知地址不在此处配置，复用全局配置 `GlobalConfig` 中的 Bark 相关字段。

- Backend:
  - 新增 API：`GET/POST /api/smart_money_golden_dog_config` 读取/更新用户配置。
  - 新增后台 worker：定期扫描 ClickHouse `smart_lp_events`，当某池子在窗口内 `action='add'` 的 distinct wallets 达到阈值时，通过 Bark 推送提醒。
  - 推送内容：当前池子交易对名称 + 钱包数量 + “建议立即关注”。

## Impact
- 新增 MySQL 表（AutoMigrate）：
  - `smart_money_golden_dog_configs`（按 user/chain 存储配置）
  - `smart_money_golden_dog_alert_states`（按 user/chain/pool 存储最近一次通知时间，用于冷却去重）
- 新增后台 service：`backend/service/smart_money_golden_dog/*`
- MiniApp Smart Money UI 结构新增一个 Tab 与配置表单。

## Decisions (defaults)
1. 默认阈值：`N=3`（3 个不同钱包）
2. 默认时间窗口：`10` 分钟
3. 默认冷却时间：`30` 分钟（同一池子在冷却期内不重复推送）
4. 仅统计 `action='add'` 的事件，distinct 钱包数以 `wallet_address` 去重。
5. Bark 仅在全局配置 `bark_enabled=true` 且 `bark_key` 可解密时发送；否则跳过。

