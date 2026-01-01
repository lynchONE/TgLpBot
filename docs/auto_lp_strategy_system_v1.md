# 自动 LP 扫描与执行策略系统设计 (V1.0)

## 1. 核心流程概述

系统分为三层架构：

1) **数据采集层（Scanner Thread）**：轮询 PoolM API，抓取 Top Fees 池子（`5/15/60/360` 分钟维度）。
2) **策略计算层（Analyzer Thread）**：基于 ClickHouse 的价格历史计算 `Z5/Z60`，并按状态机 + 共振规则输出区间宽度与执行动作。
3) **执行控制层（Executor & Guardian）**：复用现有 Zap 合约开仓；对 `is_auto=true` 的任务做退出卫士（5m 成交量回落 / 价格+交易笔数回落）。

> V1.0 原则：默认只扫描不交易；自动开仓必须显式开启配置。

---

## 2. 数据采集层 (Scanner Thread)

### 2.1 轮询频率

- 默认：每 60 秒 1 次（可通过 `AUTO_LP_SCAN_INTERVAL_SECONDS` 调整）。

### 2.2 数据源

API：

`https://mapi.poolm.xyz/api/pools/top-fees/{timeframe}?chain=bsc&dex=pcsv3,univ3,univ4`

V1.0 需要同时请求：

- `timeframe=5`（高频）
- `timeframe=15`（中频）
- `timeframe=60`（趋势）
- `timeframe=360`（中期）

并将结果写入 ClickHouse 作为后续分析与回放数据源。

> 注意：该 API 对请求头有校验，需带 `Origin=https://poolm.xyz` 与 `Referer=https://poolm.xyz/`，否则可能返回 403。

### 2.3 返回结构（核心字段）

响应示例（简化）：

```json
{
  "success": true,
  "timeframe": "5 minutes",
  "requested_protocol": [
    "v3",
    "v4"
  ],
  "requested_dex": [
    "PancakeswapV3",
    "UniswapV3",
    "UniswapV4"
  ],
  "requested_chain": "bsc",
  "total_pools": 100,
  "data": [
    {
      "chain": "bsc",
      "protocol_version": "v3",
      "pool_address": "0x....",
      "factory_name": "PancakeswapV3",
      "factory_address": "0x....",
      "trading_pair": "AAA/USDT",
      "token0_symbol": "AAA",
      "token1_symbol": "USDT",
      "token0_address": "0x....",
      "token1_address": "0x....",
      "stable_coin_symbol": "USDT",
      "fee_rate": 2500,
      "fee_percentage": 0.25,
      "transaction_count": 690,
      "total_fees": 378.877227,
      "total_volume": 227326.34,
      "current_pool_value": 193855.99,
      "last_swap_at": "2025-12-27T08:22:10.929Z"
    }
  ]
}
```

字段含义要点：

- `protocol_version`: `"v3"` / `"v4"`
- `pool_address`:
  - V3：池子合约地址（20 bytes）
  - V4：PoolId（32 bytes，长度 66 的 `0x...`）
- `total_fees`/`total_volume`：对应 timeframe 聚合后的费用/成交量（单位通常为 USD 口径）
- `current_pool_value`：当前池子价值（近似 TVL，USD）

---

## 3. 策略计算层（Analyzer Thread）

### 3.1 基础硬性指标筛选（基于 5m 数据）

只有满足以下条件的池子才会进入波动率分析：

- `current_pool_value > 50,000`
- `fee_percentage > 0.2`
- `total_fees / current_pool_value > 0.05%`（费用率，5m）
- `total_fees > 100`
- `total_volume > 5,000`

### 3.2 Z-Score（ClickHouse）

基于 ClickHouse 中的价格历史计算标准差 `σ` 和均值 `MA`，并计算 Z-Score：

`Z = (Price_current - MA_price) / σ`

其中：

- `Z5` 使用 `now()-5m` 的窗口
- `Z60` 使用 `now()-60m` 的窗口

### 3.3 状态机（Z5）与动态区间宽度

按 `Z5` 贴标签，并计算区间宽度（L/U 为下/上侧百分比宽度）：

说明：

- 只有 `RAPID_PUMP / SIDEWAYS / MILD_UPTREND` 会作为“开仓候选”；其他状态只做记录分析，不开仓。
- 宽度不再用 `Base*系数` 的方式调整，而是按不同状态使用独立的“总宽度配置”（见 ENV）。

状态与宽度：

- `CRASH (Z5 < -3)`：极端下跌（不作为开仓候选）
- `RAPID_PUMP (Z5 > +3)`：使用 `AUTO_LP_WIDTH_RAPID_PUMP_PERCENT`，贴边（下 10% / 上 90%）
- `SIDEWAYS (|Z5| < 0.5)`：使用 `AUTO_LP_WIDTH_SIDEWAYS_PERCENT`，对称（下 50% / 上 50%）
- `MILD_UPTREND (0.5 < Z5 < 1.5)`：使用 `AUTO_LP_WIDTH_MILD_UPTREND_PERCENT`，非对称（下 20% / 上 80%）
- `MILD_DOWNTREND (-1.5 < Z5 < -0.5)`：防守：下 80% / 上 20%（不作为开仓候选）
- `CONSOLIDATION`：波动率收敛（σ5 << σ60）（不作为开仓候选）
- `FALLBACK`：数据不足或 σ=0（不作为开仓候选）

### 3.4 共振分析（Z5 vs Z60）

为了过滤 5 分钟级别假突破：

- 强共振：5m 状态方向与 60m 趋势一致 → 仓位 *2（余额不足时降级为 1x）
- 背离：5m 方向与 60m 趋势相反 → 区间加宽（避免诱多/诱空）
- 共振门槛：若启用 `AUTO_LP_RESONANCE_MIN_FEE_RATE_5M` / `AUTO_LP_RESONANCE_MIN_TOTAL_VOLUME_5M` / `AUTO_LP_RESONANCE_MIN_ABS_Z60`，任一不达标则共振视为 NONE（独立于硬筛）

相关实现：`backend/services/auto_lp_service.go`。

---

## 4. 执行控制层（Zap 合约调用与退出监控）

### 4.1 开仓执行（Zap In）

V1.0 执行策略：

- 每次扫描最多自动开 1 个新仓位（避免过度交易）
- 自动执行需显式开启 `AUTO_LP_EXECUTE_ENABLED=1`
- V1.0 不再按 `stable_coin_symbol` 做硬筛过滤；若池子不含 USDT 且 `AUTO_LP_ALLOW_ENTRY_SWAP=0`，则会因无法入场而跳过/失败（建议先不开执行观察）

执行流程：

1) 选择候选池（按 Score）
2) 链上读取池子信息（V3: pool meta；V4: PoolKey）
3) 链上读取当前 tick
4) 根据状态机选择对应的总宽度（`AUTO_LP_WIDTH_*`）+ L/U 宽度计算 tickLower/tickUpper
   - `AUTO_LP_BASE_WIDTH_PERCENT` 主要用于回退/非候选状态
5) 创建 `strategy_tasks` 记录（`is_auto=true`）
6) 调用 `LiquidityService.EnterTaskFromUSDTWithOptions` 进行 Zap 开仓（`RAPID_PUMP` 时 gas multiplier 可提高）

### 4.2 退出监控（Exit / Rebalance / StopLoss）

V1.0 不重复造轮子，复用现有 `StrategyService`：

- 任务进入数据库后，StrategyService 会自动轮询任务状态
- 超出 tick 区间触发再平衡（退出 -> 按新 tick 重开）
- 可选止损策略（按用户 GlobalConfig 配置）
- 停止后不会自动重开；后续只有在再次满足扫描候选条件时，AutoLP 才会重新创建并开仓（可理解为“重置后再开”）

---

## 5. 配置（ENV）

关键开关：

- `AUTO_LP_ENABLED=1`：启用扫描服务
- `AUTO_LP_EXECUTE_ENABLED=1`：允许自动开仓（危险开关）
- `AUTO_LP_DEBUG=1`：输出扫描/筛选/候选池日志到控制台（用于排查 429/筛选条件）

目标用户：

- `AUTO_LP_USER_ID`：直接指定 user_id
- 未指定时：通过 `ADMIN_WALLET_ADDRESS` 在 `wallets` 表中反查 user_id

执行参数：

- `AUTO_LP_AMOUNT_USDT`：单次投入 USDT（必须 > 0 才会执行）
- `AUTO_LP_BASE_WIDTH_PERCENT`：基础宽度（例如 5 表示 5%）
- `AUTO_LP_MAX_ACTIVE_TASKS`：自动策略允许的最大活跃任务数
- `AUTO_LP_EMERGENCY_GAS_MULTIPLIER`：紧急 Gas 倍数（默认 2.0）

过滤参数（0 表示不限制）：

- `AUTO_LP_MIN_POOL_VALUE_USD`
- `AUTO_LP_MIN_FEE_PERCENTAGE`
- `AUTO_LP_MIN_FEE_RATE_5M`（5m 手续费 / TVL，百分比）
- `AUTO_LP_MIN_TOTAL_FEES_5M`
- `AUTO_LP_MIN_TOTAL_VOLUME_5M`
- `AUTO_LP_MIN_TX_5M` / `AUTO_LP_MIN_TX_60M`
- `AUTO_LP_MIN_FEE_APR_5M` / `AUTO_LP_MIN_FEE_APR_60M`
- `AUTO_LP_MAX_SURGE_RATIO`
- `AUTO_LP_MAX_CANDIDATES`
- `AUTO_LP_RESONANCE_MIN_FEE_RATE_5M`（共振门槛：5m 手续费 / TVL，百分比）
- `AUTO_LP_RESONANCE_MIN_TOTAL_VOLUME_5M`（共振门槛：5m 成交量）
- `AUTO_LP_RESONANCE_MIN_ABS_Z60`（共振门槛：|Z60|）
- `AUTO_LP_REQUIRE_STABLE_SYMBOL`（保留字段：当前版本不用于筛选）
- `AUTO_LP_WIDTH_SIDEWAYS_PERCENT` / `AUTO_LP_WIDTH_MILD_UPTREND_PERCENT` / `AUTO_LP_WIDTH_RAPID_PUMP_PERCENT`

退出卫士（可配置）：

- `AUTO_LP_GUARD_WINDOW_SECONDS`：窗口秒数（默认 120 秒）
- `AUTO_LP_GUARD_VOLUME_DROP_PERCENT`：5m 成交量相对峰值回落比例（默认 0.30=30%）
- `AUTO_LP_GUARD_NO_EXIT_MIN_FEE_RATE_5M`：若 5m 手续费/TVL（total_fees/current_pool_value，%）高于阈值，则不因“成交量回落”触发撤退（默认 0=关闭）
- `AUTO_LP_GUARD_LOW_FEE_RATE_5M`：若 5m 手续费/TVL 低于阈值，则“成交量回落”改用低费率阈值（默认 0=关闭）
- `AUTO_LP_GUARD_VOLUME_DROP_PERCENT_LOW_FEE`：低手续费率时的成交量峰值回落比例（默认 0=不启用）
- `AUTO_LP_GUARD_PRICE_TX_DROP_PERCENT`：价格与交易笔数相对峰值回落比例（默认 0.10=10%）

ClickHouse：

- `CLICKHOUSE_RESET_ALL=1`：启动时删除 ClickHouse 当前 DB 下所有表（只跑一次，跑完立即关）

完整示例见：`backend/.env.example`。

---

## 6. 风险控制建议（V1.0）

1) 先开扫描、不开执行：观察一段时间确认候选池质量。
2) 必须设置过滤条件：`MIN_POOL_VALUE_USD`、`MIN_TX_*`。
3) 限制最大活跃任务：`AUTO_LP_MAX_ACTIVE_TASKS`。
4) 强烈建议使用小额测试钱包。
