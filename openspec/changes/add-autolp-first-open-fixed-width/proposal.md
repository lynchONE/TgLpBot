# Change: AutoLP 首次开仓支持固定区间宽度（MiniApp 管理员可配置）

## Why
- AutoLP 任务首次开仓的区间宽度当前由状态机/共振逻辑动态决定，在部分行情下可能导致首次开仓区间过窄或过宽。
- 期望给管理员一个可控开关：首次开仓使用固定的“总宽度(%)”，以便在不改动后续再平衡逻辑的前提下，统一首次入场的风险/频率特征。

## What Changes
- 新增系统级配置项（MiniApp 管理员可读写）：
  - `autolp_first_open_fixed_width_enabled`：是否启用 AutoLP 任务首次开仓固定区间
  - `autolp_first_open_fixed_width_percent`：首次开仓固定“总宽度(%)”
- AutoLP 在创建并首次开仓 Auto 任务时：
  - **仅首次开仓**使用上述固定总宽度计算实际 `tick_lower/tick_upper`
  - 任务的 `range_lower_percentage/range_upper_percentage`（用于后续再平衡）仍按原有逻辑计算与保存
- 除首次开仓以外：任务后续再平衡流程保持原逻辑不变

## Impact
- Backend:
  - `backend/base/models/system_config.go`
  - `backend/service/user/system_config.go`
  - `backend/service/web_server/admin_system_config.go`
  - `backend/service/auto_lp/auto_lp_service.go`
- MiniApp:
  - `miniapp/src/components/SystemConfigCard.jsx`

