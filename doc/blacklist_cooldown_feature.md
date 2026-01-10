# 黑名单和冷却池子功能

**日期**: 2026-01-10

## 功能概述

1. **用户黑名单** - 热门池子长按标记黑名单，AutoLP开单前检查跳过
2. **冷却交易对/代币** - 连续跌破2次后，非稳定币代币（如 BTC）写入 Redis 冷却，30分钟自动过期。期间所有包含该代币的池子在 AutoLP 中禁止开仓。
3. **监控界面展示** - 显示冷却中的代币和剩余时间，位于监控任务列表底部。

---

## 后端实现

### 新增文件
* `service/blacklist/blacklist.go` (Set)
* `service/blacklist/cooldown.go` (String + TTL)
* `service/web_server/blacklist.go`, `cooldowns.go`

### 修改文件
* `strategy_service.go`: 冷却非稳定币代币
* `auto_lp_service.go`: 检查候选池代币是否冷却
* `server.go`: 注册路由

---

## 前端实现

### 修改文件
* `api.js`: 添加黑名单/冷却 API
* `HotPoolCard.jsx`: 长按黑名单支持
* `App.jsx`: 冷却列表展示（底部）
* `SystemConfigCard.jsx`: 优化默认值显示，输入框占位符显示当前生效的系统默认值

### 交互优化
* 硬筛配置输入框现在会显示灰色的默认值（如 "50000"），而不是 "0"，方便用户知道留空时的实际取值。

---

## 验证结果
- ✅ 后端编译通过
- ✅ 前端编译通过
