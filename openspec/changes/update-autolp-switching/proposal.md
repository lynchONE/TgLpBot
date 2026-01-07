# Change: AutoLP 满仓换仓（基于收益提升 + 冷却时间）

## Why
当用户的 AutoLP 已达到最大开仓数量（满仓）时，当前实现会直接停止开新仓，即使扫描结果出现明显更优的候选池也无法“调仓换池”，容易长期持有低收益池子。

此外，频繁换仓会显著增加 Gas 成本并引入并发风险（同一用户多线程扫描/多次触发导致重复撤仓开仓）。因此需要一个**可配置阈值**与**可配置冷却时间**来避免抖动和重复执行。

## What Changes
- 在 AutoLP 满仓时，增加“换仓”逻辑：
  - 以扫描结果中的 **Top1 候选池**作为目标（并满足用户侧可开仓约束）
  - 找到该用户当前 AutoLP 仓位中 **最低收益** 的一个作为被替换对象
  - 仅当目标池收益相对提升达到用户配置阈值，并且不在冷却窗口内时，才触发换仓
- 换仓严格复用现有的撤仓/开仓流程（不新增链上交易拼装逻辑），通过设置任务的 `exit_pending_action=switch` 与 `switch_target_*` 字段让策略系统完成“撤出 → 兑换 USDT → 按新池子重开仓”。
- 新增用户级配置项：
  - `switch_min_improvement_pct`（已有字段但未接入实际执行与 UI；**0 表示禁用换仓**）
  - `switch_cooldown_seconds`（默认 300 秒，可配置；以**换仓完成**为冷却起点）
- 在 Telegram Bot `/auto` 菜单中新增入口，让用户可自助配置“换仓阈值/冷却时间”。

## Impact
- Affected specs: `specs/auto-lp-switching/spec.md` (new capability)
- Affected code (implementation stage): `backend/service/auto_lp/auto_lp_service.go`, `backend/base/models/auto_lp_user_config.go`, `backend/service/bot/auto_*`
- Data model: adds columns to `auto_lp_user_configs` via GORM AutoMigrate
- Risk: 涉及资金与自动交易行为；默认应保持安全（不配置则不换仓），并通过冷却/并发保护降低抖动与重复执行风险

## Open Questions (need your confirmation)
1. ✅ “收益率”对比口径：使用 `FeeRate5mPct`（5m 手续费/TVL）。
2. ✅ 目标池选择：必须严格使用扫描结果的 Top1 候选池。
3. ✅ 冷却时间起点：以“换仓完成（新仓开仓成功）”为准。
4. ✅ `switch_min_improvement_pct=0`：禁用换仓。
