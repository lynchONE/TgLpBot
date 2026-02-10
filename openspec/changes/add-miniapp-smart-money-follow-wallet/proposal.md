# Change: Add Smart Money wallet LP copy-trading (跟单) in MiniApp

## Why
- Smart Money 模块的核心价值之一是「发现并跟随聪明钱包的 LP 行为」。
- 目前 MiniApp 只能查看钱包参与与仓位，但无法直接配置「跟单」自动化，用户需要手动开/撤 LP。
- 希望支持：针对指定钱包，钱包开 LP 我也开、钱包撤 LP 我也撤，并支持随机延迟与资金控制。

## What Changes
- MiniApp:
  - 在 Smart Money 的「钱包仓位」弹窗内新增「跟单」配置区：
    - 开关（启用/停用）
    - 单次跟单金额（USDT）
    - 跟单最大金额（USDT，限制同时占用的总额）
    - 随机延迟（秒，0~60，可配置 min/max 或 max）

- Backend:
  - 新增 API：`GET/POST /api/smart_money_follow_config`
    - 读取/更新当前用户对某个 target wallet 的跟单配置
  - 新增后台 worker：消费 ClickHouse `smart_lp_events` 中 target wallet 的 add/remove 事件
    - add 事件：在随机延迟后为当前用户创建对应 pool 的策略任务并入场
    - remove 事件：在随机延迟后对对应的跟单任务发起退出（撤 LP 并换回 USDT）
    - 仅对跟单创建的任务生效（不影响用户手动/AutoLP 的任务）

## Impact
- 新增 MySQL 表（AutoMigrate）：
  - `smart_money_follow_configs`
  - `smart_money_follow_jobs`
  - `smart_money_follow_tasks`
- 新增后台 service：`backend/service/smart_money_follow/*`
- MiniApp 增加一个 Vercel proxy：`miniapp/api/smart_money_follow_config.js`

## Decisions (defaults)
1. 启用跟单时不回溯历史事件：首次启用会把游标初始化到当前最新 `event_seq`，仅跟随后续事件。
2. 延迟范围默认 `0~60s`，可在配置中调整（限制最大 60s 以符合「一分钟以内」）。
3. 单次跟单金额以 USDT 计；若超出最大金额（已占用 + 本次）则跳过该次入场并记录 job 失败原因。

