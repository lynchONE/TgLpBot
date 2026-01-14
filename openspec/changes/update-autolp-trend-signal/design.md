# Design: AutoLP 趋势/动量进场门禁（V2）

## 背景与问题
当前实现中：
- `Z5/Z60` 计算的是“当前价格相对均值的偏离”，无法表达“价格正在上升/下降”的**方向**。
- 60m 趋势 `Trend60` 由 `Z60` 的正负决定：当价格持续下跌但仍高于 60m 均值时，`Z60` 可能仍为正，从 K 线视角会出现“明明在跌却被判断为上涨/不跌”的错觉。
- 候选开仓（`CANDIDATE`）只依赖 `Z5` 状态（`RAPID_PUMP / SIDEWAYS / MILD_UPTREND`），对“下跌但被判为 SIDEWAYS”缺少强约束。

## 目标
- 让“趋势方向”与 K 线直觉一致：**趋势=方向**，而不是“相对均值的位置”。
- 在明显下跌/回落阶段禁止开仓，降低错误进场率。
- 保持扫描性能：不额外增加 ClickHouse 查询数。

## 非目标
- 不做完整量化回测框架（先靠日志/对比与少量样本验证）。
- 不改变硬筛、评分与宽度算法（仅对“候选门禁/趋势判定”做最小必要改动）。

## 信号定义（复用现有统计）
已存在统计：
- `MA5/σ5`：最近 5 分钟价格均值/波动
- `MA60/σ60`：最近 60 分钟价格均值/波动（若 60m 数据不足则回退到 5m 数据窗口 60m）

新增派生信号：
1) **均线差方向**（趋势方向核心）
- `ma_cross_pct = (MA5 - MA60) / MA60 * 100`
- 判定阈值：`entry_trend_cross_pct`（建议默认 `0.3`，单位为百分比点：`0.3` 表示 `0.3%`）
  - `ma_cross_pct >= +entry_trend_cross_pct` → `UPTREND`
  - `ma_cross_pct <= -entry_trend_cross_pct` → `DOWNTREND`
  - else → `SIDEWAYS`

2) **短期动量偏离**（用于捕捉回落）
- `dev5_pct = (P - MA5) / MA5 * 100`
- 门禁阈值：`entry_block_dev5_pct`（建议默认 `0.5`，单位为百分比点；表示“低于 MA5 的跌幅”）
  - `dev5_pct <= -entry_block_dev5_pct` → 视为短期下跌动量，禁止开仓

## 候选开仓门禁（规则）
在现有 `Z5` 状态机筛选基础上（保留 V1：`RAPID_PUMP / SIDEWAYS / MILD_UPTREND` 仍可候选，其中 `SIDEWAYS` 暂时允许），叠加：
- 若 `trend_filter_enabled=false`：沿用旧逻辑（便于快速回退）。
- 否则：
  - 当 `Trend60 == DOWNTREND` → 禁止开仓（不标记为 `CANDIDATE`）。
  - 当 `dev5_pct <= -entry_block_dev5_pct` → 禁止开仓（即使 `Trend60` 为 `SIDEWAYS`）。

> 说明：`Trend60` 与 `dev5_pct` 均依赖 `MA5/MA60` 可靠性；若样本数不足（如 `n5<4` 或 `n60<12`），建议直接判为 `UNKNOWN` 并禁止开仓（安全优先）。

## 可观测性
- Top 推送与日志补充展示：
  - `ma_cross_pct`、`dev5_pct`、最终门禁命中原因（例如 `BLOCK: DOWNTREND` / `BLOCK: DEV5_DROP`）
- 便于肉眼对照 K 线与策略判断，快速调整阈值。

## 风险与缓解
- 变得更保守，可能减少开仓次数：通过可配置阈值 + 回退开关控制。
- MA 交叉对“刚开始转跌”的识别滞后：由 `dev5_pct` 的短期动量门禁补齐。
