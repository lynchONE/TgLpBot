# Change: AutoLP 进场趋势判断升级（修正“下跌仍开单”）

## Why
- 当前 `Z5/Z60` 是“价格相对均值的偏离度”（`Z=(P-MA)/σ`），更像“位置”而不是“方向”；K 线明显走跌时，`Z60` 仍可能为正，从而误判趋势。
- 实际开仓候选主要由 `Z5` 状态机决定（`RAPID_PUMP / SIDEWAYS / MILD_UPTREND`），对“明显下跌但被判成 SIDEWAYS”缺少硬性门禁，导致错误开单风险。

## What Changes
- 引入**方向型信号**：用 `MA5 vs MA60`（均线差）判定 60m 方向（`UPTREND/DOWNTREND/SIDEWAYS`），替代 `Z60` 的正负作为“趋势方向”依据。
- 引入**下跌门禁**：当判定为 `DOWNTREND`（或短期动量显著为负）时，AutoLP 不再把该池子标记为可开仓候选（即使 `Z5` 状态为 `SIDEWAYS`）。
- 新增可调阈值与回退开关：默认开启新门禁，支持一键回退旧逻辑以便对比效果/快速止损。

## Impact
- Affected specs (new):
  - `specs/auto-lp-entry-signal/spec.md`
- Affected code (implementation stage):
  - Backend: `backend/service/auto_lp/auto_lp_service.go`（趋势判定/候选门禁/通知文案）
  - Backend: `backend/base/config/config.go`（新增阈值配置）
  - (可选) `backend/service/bot/auto_handlers.go`（/auto 策略说明文本同步）
- Backwards compatibility:
  - 行为会更“保守”（减少在下跌/回落时开仓），但提供开关可回退旧逻辑。

## Decisions (proposed)
1. 60m 趋势方向基于均线差：`ma_cross_pct=(MA5-MA60)/MA60*100`。
2. 进场门禁至少包含：`Trend60 != DOWNTREND`；可选叠加 `dev5_pct=(P-MA5)/MA5*100` 的短期下跌阈值。
3. 不增加额外 ClickHouse 查询：复用现有 `MA5/MA60` 统计结果，避免扫描阶段额外放大查询量。
4. 初始默认阈值建议（单位均为“百分比点”，如 `0.3` 表示 `0.3%`）：
   - `trend_filter_enabled=true`
   - `entry_trend_cross_pct=0.3`
   - `entry_block_dev5_pct=0.5`
5. `SIDEWAYS` 暂时仍允许作为开仓候选（保留 V1），但会被 `DOWNTREND`/短期下跌动量门禁拦截。
