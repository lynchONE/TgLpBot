# Vercel Serverless Functions 合并记录

## 日期：2026-01-10

## 问题
Vercel Hobby 计划限制最多 12 个 Serverless Functions，原有 13 个 API 文件超出限制。

## 解决方案
按功能将 API 合并为 6 个文件，通过 `endpoint` 查询参数区分端点。

## 合并结果

| 新文件 | 包含的端点 | 说明 |
|--------|-----------|------|
| `admin.js` | autolp_disable, autolp_stats, realtime_positions, realtime_users, system_config | 管理员功能 |
| `pools.js` | hot_pools, pool_ohlcv | 热门池子相关 |
| `positions.js` | realtime_positions, me, auto_monitor | 用户仓位相关 |
| `settings.js` | config, autolp_config, global_config | 配置相关 |
| `task_action.js` | delete, pause, stop | 任务操作 |
| `trading.js` | open_position, blacklist, cooldowns | 交易相关 |

## 删除的文件
- `hot_pools.js`
- `pool_ohlcv.js`
- `realtime_positions.js`
- `me.js`
- `auto_monitor.js`
- `config.js`
- `autolp_config.js`
- `global_config.js`
- `open_position.js`
- `blacklist.js`
- `cooldowns.js`

## 前端更新
`miniapp/src/lib/api.js` 中所有 API 调用路径已更新，使用新的合并端点格式：
- `/api/pools?endpoint=hot_pools`
- `/api/positions?endpoint=realtime_positions`
- `/api/settings?endpoint=config`
- `/api/trading?endpoint=open_position`
