# 变更：AutoLP 极端拉升回落识别（Flash Pump/Dump Guard）

## Why
当前 AutoLP 在极端拉升（例如短时间冲到局部最高点）时仍可能立即开仓，容易在“冲顶回落”场景中承受较大回撤。

我们希望更精准地规避“**一分钟内量拉升 + 价格拉升，随后 1 分钟内又快速回落**”的极端情况：这类走势往往是短时冲高（wick/flash pump），在冲顶时开仓风险极高。

关键点：想要“准确识别 1 分钟后会回落”，必须引入**比 5m 统计更贴近实时的参照**。最佳低依赖方案是用 **链上 TWAP（30~120s）对比当前 spot** 来判断是否属于“短时偏离过大”的冲高，而不是用 5m/60m 的均值方差去粗略判断。

## What Changes
- 增加 AutoLP **Flash Pump/Dump Guard**：在真正创建任务/发起开仓前，对“候选池”做一次**链上 spot vs TWAP 偏离度**校验；当偏离度过大时，认为是短时冲高风险，跳过/延迟开仓。
- Guard 目标：只针对“极端 1 分钟冲高”做拦截，不影响正常趋势/横盘开仓（通过阈值与迟滞控制）。
- 所有门禁参数均为**可配置**（建议系统级 DB 配置优先，环境变量回退），便于你逐步调参。
- 被门禁拦截时，复用现有“跳过原因”通知（若该能力已启用），方便回溯。

## Proposed Guard Rules (v1)
> 核心信号：`spotTick`（slot0）与 `twapTick`（observe 计算 60s TWAP）的偏离。

1) **Flash Pump Guard（短时冲高过滤）**
- 计算 `spotOverTwapPct = price(spotTick)/price(twapTick) - 1`（仅关注向上偏离，避免把快速下跌当成“冲高”）。
- 当 `spotOverTwapPct >= spot_over_twap_max_pct` 时：
  - **不立即开仓**，对该池子进入短暂“观察期”（`entry_guard_confirm_seconds`）。
  - 观察期结束后再评估是否允许开仓。
- 默认建议（先只抓“极端”）：`spot_over_twap_max_pct = 6% ~ 10%`，`entry_guard_confirm_seconds = 60s`。

2) **Dump Confirmation（确认是否“冲顶回落”）**
- 在观察期结束后（约 1 分钟），重新读取 `spotTick/twapTick`：
  - 若 `spotOverTwapPct` 明显回落（<= `spot_over_twap_resume_pct`）且价格相对观察期开始的 spot 回撤 `>= dump_retrace_pct`，则认为出现“冲顶回落”：
    - 对该池子设置更长的 `dump_cooldown_seconds`（例如 30 分钟），避免立刻再次被高费率/高成交量吸引进去。
  - 若仍保持偏离但未回撤（说明可能是持续趋势），允许继续按原规则开仓，或继续观察 1 次（可配置 `max_confirm_rounds`，默认 1）。
- 默认建议：`spot_over_twap_resume_pct = 2% ~ 3%`（迟滞，防抖），`dump_retrace_pct = 5%`，`dump_cooldown_seconds = 1800s`。

3) **V4 / Fallback（无 TWAP 支持时的替代信号）**
- 若无法读取 TWAP（例如不支持 `observe`，或 V4 暂无可用 TWAP），使用“**两次扫描的价格脉冲**”替代：
  - 若 `price(1m)` 上涨 >= `pulse_up_pct` 且下一次扫描 `price(1m)` 下跌 >= `pulse_down_pct`，判定为“冲顶回落”，触发 `dump_cooldown_seconds`。
- 该 fallback 只在无法拿到 TWAP 时启用，尽量以链上 TWAP 为主。

## Impact
- 影响规格：新增 `specs/auto-lp-entry-guard/spec.md`（本变更的正式规格）
- 影响代码（实施阶段）：
  - `backend/service/auto_lp/auto_lp_service.go`（开仓前门禁）
  - `backend/base/blockchain/v3_pool.go`（增加 observe/TWAP 读取）
  - `backend/base/models/system_config.go`、`backend/service/user/system_config.go`（若采用系统级 DB 可调参）
- 风险与收益：
  - ✅ 主要收益：更精准规避“极短时冲高回落”导致的追高开仓
  - ⚠️ 代价：极端行情下会延迟开仓（通常 60s），可能错过少量持续拉升行情；可通过阈值调大来只抓最极端情况

## Open Questions (need your confirmation)
1. 你希望门禁只对 V3 生效（链上 TWAP），还是 V4 也要用 fallback 一并生效？
2. 门禁参数希望放在：
   - A. 系统级配置（管理员可动态调参，DB 优先）
   - B. 仅环境变量（实现最简单）
3. 命中 Flash Pump 时的行为：
   - A. “延迟确认”：观察 60s 后再决定是否开仓（更不漏趋势）
   - B. “直接跳过”：立刻跳过并进入短冷却（更保守）
4. 命中“冲顶回落”后是否进入长冷却（如 30 分钟）？还是只跳过当次即可？
