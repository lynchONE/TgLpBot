# Change: AutoLP 硬筛新增（中文交易对 + 费率上限）& MiniApp 代币跳转 GMGN

## Why
- 近期出现部分交易对/代币符号包含中文的池子，存在较高风险与噪声，希望可一键禁止 AutoLP 开仓。
- 部分池子的 `fee_percentage`（池子费率档位）过高时不符合策略偏好，希望可配置“超过阈值就禁止开单”。
- MiniApp 实时仓位任务面板当前点击代币跳转到 BscScan，期望改为跳转到 GMGN，便于快速查看代币信息。

## What Changes
- 系统级 AutoLP 硬筛新增 2 个可配置项（MiniApp 管理员可配置）：
  - `autolp_filter_chinese_tokens`：开启后，交易对/代币符号包含中文的池子禁止开单（AutoLP 不应对其创建开仓任务或作为换仓目标）。
  - `autolp_max_fee_percentage`：当该值 `> 0` 时，若池子 `fee_percentage > autolp_max_fee_percentage` 则禁止开单。
- MiniApp「实时仓位任务面板」点击代币地址：从 `bscscan.com/token/<addr>` 改为跳转 `gmgn` 的 BSC 代币页。

## Impact
- Affected specs (new):
  - `specs/auto-lp-hard-filter/spec.md`
  - `specs/miniapp-position-links/spec.md`
- Affected code (implementation stage):
  - Backend: `backend/base/models/system_config.go`, `backend/service/user/system_config.go`, `backend/service/web_server/admin_system_config.go`, `backend/service/auto_lp/auto_lp_service.go`, `backend/base/config/config.go`
  - MiniApp: `miniapp/src/components/SystemConfigCard.jsx`, `miniapp/src/components/PositionCard.jsx`
- Data model: `system_configs` 表新增字段（GORM AutoMigrate）
- Backwards compatibility: 默认不改变现有行为（新增硬筛默认关闭 / 上限为 0 表示不启用）

## Decisions (implemented)
1. `autolp_max_fee_percentage` 单位按百分比理解：例如 `1.0` 表示 `1%`（常见档位 `0.01/0.05/0.3/1`）。
2. “包含中文” 同时检查 `trading_pair` 与 `token0_symbol/token1_symbol`。
3. GMGN 链接使用：`https://gmgn.ai/bsc/token/<tokenAddress>`。
