# AutoLP 参数动态配置进展

## 日期：2026-01-10

## 完成的修改

### 后端

1. **`backend/base/models/system_config.go`**
   - 添加 3 个宽度策略字段：
     - `AutoLPWidthSidewaysPercent` - 横盘宽度 (%)
     - `AutoLPWidthMildUptrendPercent` - 温和上涨宽度 (%)
     - `AutoLPWidthRapidPumpPercent` - 急涨宽度 (%)
   - 添加 5 个退出卫士字段：
     - `AutoLPGuardVolumeDropPercent` - 成交量下降阈值
     - `AutoLPGuardPriceTxDropPercent` - 价格+交易笔数下降阈值
     - `AutoLPGuardNoExitMinFeeRate5m` - 手续费率保护阈值
     - `AutoLPGuardLowFeeRate5m` - 低手续费率阈值
     - `AutoLPGuardVolumeDropPercentLow` - 低费率时成交量下降阈值
   - 新增 `WidthGuardConfig` 结构体

2. **`backend/service/web_server/admin_system_config.go`**
   - 更新请求结构体支持新参数
   - 更新响应结构体添加 `WidthGuardDefaults`
   - 新增 `getWidthGuardDefaults()` 函数

### 前端

1. **`miniapp/src/components/SystemConfigCard.jsx`**
   - 重构为三个可折叠配置区域：
     - 硬筛阈值
     - 宽度策略
     - 退出卫士
   - 支持加载和保存所有参数

## 使用说明

1. 重启后端服务，GORM 会自动迁移新字段
2. 在 miniapp 管理界面打开"系统配置"卡片
3. 展开对应配置区域进行修改
4. 值为 0 时使用环境变量默认值
5. 点击"保存配置"按钮保存

## 环境变量对应

| 前端参数 | 环境变量 |
|---------|---------|
| 横盘宽度 | `AUTO_LP_WIDTH_SIDEWAYS_PERCENT` |
| 温和上涨宽度 | `AUTO_LP_WIDTH_MILD_UPTREND_PERCENT` |
| 急涨宽度 | `AUTO_LP_WIDTH_RAPID_PUMP_PERCENT` |
| 成交量下降阈值 | `AUTO_LP_GUARD_VOLUME_DROP_PERCENT` |
| 价格+交易笔数下降阈值 | `AUTO_LP_GUARD_PRICE_TX_DROP_PERCENT` |
| 手续费率保护阈值 | `AUTO_LP_GUARD_NO_EXIT_MIN_FEE_RATE_5M` |
| 低手续费率阈值 | `AUTO_LP_GUARD_LOW_FEE_RATE_5M` |
| 低费率时成交量下降阈值 | `AUTO_LP_GUARD_VOLUME_DROP_PERCENT_LOW_FEE` |
