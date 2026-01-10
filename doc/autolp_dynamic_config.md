# AutoLP 参数动态配置进展

## 日期：2026-01-10

## 完成的修改

### 后端 Models

1. **`backend/base/models/system_config.go`**
   - 添加 3 个宽度策略字段
   - 添加 5 个退出卫士字段
   - 新增 `WidthGuardConfig` 结构体传递默认值

### 后端 Service

1. **`backend/service/user/system_config.go`**
   - 新增 `GetWidthGuardConfig()` 函数
   - 支持从数据库动态读取配置，值为 0 时回退到环境变量

### 后端 AutoLP

1. **`backend/service/auto_lp/auto_lp_service.go`**
   - `analyzeSnapshot()` 函数改用 `GetWidthGuardConfig()` 动态读取宽度配置
   - `guardActiveAutoTasks()` 函数改用动态退出卫士配置
   - 移除了已废弃的 `noExitMinFeeRate5m` 和 `priceTxDropPct` 参数

2. **`backend/service/web_server/auto_monitor.go`**
   - 改用动态配置读取
   - 更新结构体移除废弃字段

### 后端 API

1. **`backend/service/web_server/admin_system_config.go`**
   - 支持新参数的 GET/POST
   - 返回宽度和退出卫士默认值

### 前端

1. **`miniapp/src/components/SystemConfigCard.jsx`**
   - 三个可折叠配置区域

## 动态生效机制

1. **数据库优先**：`GetWidthGuardConfig()` 从数据库读取配置
2. **环境变量回退**：配置值为 0 时使用环境变量默认值
3. **实时生效**：每次扫描/监控循环开始时重新读取配置，无需重启

## 最终配置参数

| 类型 | 参数 | 环境变量 |
|-----|------|---------|
| **宽度策略** | 横盘宽度 | `AUTO_LP_WIDTH_SIDEWAYS_PERCENT` |
| | 温和上涨宽度 | `AUTO_LP_WIDTH_MILD_UPTREND_PERCENT` |
| | 急涨宽度 | `AUTO_LP_WIDTH_RAPID_PUMP_PERCENT` |
| **退出卫士** | 成交量下降阈值 | `AUTO_LP_GUARD_VOLUME_DROP_PERCENT` |
| | 价格跌幅阈值 | `AUTO_LP_GUARD_PRICE_DROP_PERCENT` |
| | 交易笔数跌幅阈值 | `AUTO_LP_GUARD_TX_DROP_PERCENT` |
| | 低手续费率阈值 | `AUTO_LP_GUARD_LOW_FEE_RATE_5M` |
| | 低费率时成交量下降阈值 | `AUTO_LP_GUARD_VOLUME_DROP_PERCENT_LOW_FEE` |

## 已移除的参数

- ❌ `AUTO_LP_GUARD_PRICE_TX_DROP_PERCENT`
- ❌ `AUTO_LP_GUARD_NO_EXIT_MIN_FEE_RATE_5M`

