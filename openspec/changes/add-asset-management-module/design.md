## Context
- 当前用户侧已有 `realtime_positions`，可以实时得到 `wallet_usd`、`position_usd`、`fee_usd`、`total_usd`，但没有历史资产快照，也没有稳定的资产趋势接口。
- 当前 `wallet_balance_snapshots` 仅保存默认钱包的 BNB / USDT 日快照，无法覆盖多钱包、LP 持仓价值、未领取手续费等资产维度。
- 当前 `trade_records` 已记录完整开平仓闭环，`profit_usdt` 已扣除 Gas，适合作为用户侧“已实现 LP 盈亏”的唯一统计来源。
- 当前 Smart Money 已具备 `sm_lp_events`、`sm_lp_positions`、钱包/池子/事件列表与详情，但缺少按日归档的数据层，因此无法稳定支撑钱包曲线、窗口统计和排行榜。
- 当前 MiniApp 已有独立 `AdminPage`，承担在线用户、活跃任务、系统配置、RPC 与 Private Zap 管理；如果新模块继续新增管理员能力而不整合入口，会造成导航割裂。

## Goals / Non-Goals
- Goals:
  - 为普通用户提供稳定的资产变化图表与 LP 盈亏窗口统计。
  - 为管理员提供聪明钱钱包资产变化、LP 统计与每日排行能力。
  - 在 `miniapp` 与 `webapp` 提供一致的资产管理模块信息架构。
  - 将 MiniApp 现有管理页能力迁移到新模块的管理员侧，避免管理功能分散。
  - 明确“历史统计不含当天、当天单独展示”的统一口径。
  - 在不引入重型新依赖的前提下复用现有实时查询、成交记录和 Smart Money 事件数据。
- Non-Goals:
  - 不在第一期实现任意日期范围报表与导出。
  - 不在第一期做税务口径、成本分摊、跨账户审计级报表。
  - 不保证聪明钱侧覆盖钱包中所有任意历史 token 的完整组合估值。

## Decisions
- Decision: 历史窗口一律不包含当天
  - 历史统计窗口统一按 `[窗口起点, 今天 00:00)` 查询。
  - 当天数据统一按 `[今天 00:00, 当前时间)` 单独展示。
  - 服务器自然日口径沿用现有服务本地时区，默认按 `Asia/Shanghai` 配置运行。

- Decision: 每日汇总任务在 `00:05` 后汇总上一自然日
  - 汇总任务只处理“上一自然日”的最终结果，避免把未完成的当天数据混入历史。
  - 汇总任务必须幂等；同一天重复执行只能覆盖更新，不能生成重复记录。
  - 需要提供按指定日期补算的内部入口，方便回填和修复跨日失败。

- Decision: 用户资产曲线采用“日快照 + 当天实时点”的双层模型
  - 历史曲线基于按日快照表返回，保证查询稳定且成本可控。
  - 当天点位由 `realtime_positions` 现算，作为“今日实时预览”，不并入历史累计窗口。
  - 快照指标固定为：
    - `wallet_usd`
    - `position_usd`
    - `fee_usd`
    - `total_usd`

- Decision: 用户 LP 历史统计只使用 `trade_records`
  - `profit_usdt` 作为已实现盈亏唯一来源，避免和实时估值混口径。
  - 按 `closed_at` 落入的自然日进行日统计，再支持 `近1天 / 近7天 / 近30天` 汇总。
  - 历史窗口返回：
    - 已实现盈亏
    - 平仓笔数
    - 胜/负笔数
    - 胜率
    - 平均单笔收益
    - 最佳/最差单日或池子摘要
  - 当天区域单独返回：
    - 今日已实现盈亏
    - 今日平仓笔数
    - 今日仍在运行仓位的浮动盈亏摘要（若可实时获取）

- Decision: 聪明钱收益采用“估算已实现 PnL”口径，并显式标注估算属性
  - 配对规则优先使用 `wallet_address + chain_id + pool_address + nft_token_id`。
  - 当 `nft_token_id` 不可用时，回退到 `wallet_address + chain_id + pool_address + tick_lower + tick_upper`。
  - 单次平仓估算收益定义为：
    - `remove_total_usd - matched_add_total_usd`
  - 若 add/remove 任一侧缺少 USD 快照，或出现无法唯一配对的多次 add，则不计入收益额，并返回 `unmatched_count` / `warning`。

- Decision: 聪明钱钱包资产曲线采用“可识别总资产”口径
  - 第一阶段不做全链任意 token 组合扫描。
  - 钱包资产快照由以下部分构成：
    - 原生币余额折算 USD
    - 稳定币余额折算 USD
    - 近 30 天参与过 LP 的 token 链上余额折算 USD
    - 当前 open LP 的估算持仓价值
  - 这样可以在现有代码结构下控制复杂度，同时覆盖绝大多数运营分析需求。

- Decision: 新增四张按日汇总表
  - `user_asset_daily_snapshots`
    - `user_id`
    - `wallet_id`（可空；空表示用户聚合）
    - `chain`
    - `snapshot_day`
    - `wallet_usd`
    - `position_usd`
    - `fee_usd`
    - `total_usd`
    - `captured_at`
  - `user_lp_daily_stats`
    - `user_id`
    - `wallet_id`（可空；空表示用户聚合）
    - `chain`
    - `stat_day`
    - `realized_pnl_usd`
    - `closed_count`
    - `win_count`
    - `loss_count`
    - `break_even_count`
  - `sm_wallet_daily_snapshots`
    - `wallet_address`
    - `chain_id`
    - `snapshot_day`
    - `native_usd`
    - `stable_usd`
    - `tracked_token_usd`
    - `open_lp_usd`
    - `total_usd`
    - `tracked_token_count`
  - `sm_lp_daily_stats`
    - `wallet_address`
    - `chain_id`
    - `stat_day`
    - `estimated_realized_pnl_usd`
    - `matched_remove_count`
    - `unmatched_remove_count`
    - `add_count`
    - `remove_count`
    - `active_pool_count`

- Decision: API 采用独立资产接口，不挤进现有 `realtime_positions`
  - 用户侧：
    - `POST /api/assets/overview`
    - `POST /api/assets/history`
    - `POST /api/assets/lp_stats`
  - 管理员侧：
    - `POST /api/admin/assets/smart_money_overview`
    - `POST /api/admin/assets/smart_money_wallet`
    - `POST /api/admin/assets/smart_money_leaderboard`
  - 用户侧接口要求 Telegram WebApp `initData` 认证。
  - 管理员侧接口要求 Telegram WebApp `initData` + admin 权限。

- Decision: 资产管理模块采用统一的信息架构
  - 用户侧主视图命名为“我的资产”。
  - 管理员侧拆分为：
    - `聪明钱资产`
    - `运行管理`
    - `系统管理`
  - `运行管理` 承接现有 MiniApp 管理页中的：
    - 在线用户
    - 活跃任务
    - 用户详情
  - `系统管理` 承接现有 MiniApp 管理页中的：
    - 系统配置
    - RPC 管理
    - Private Zap 管理

- Decision: MiniApp 先做入口迁移，后台接口尽量复用
  - 现有 `/api/admin/*` 能力第一阶段不强制改名或重写。
  - 新模块优先复用现有管理接口，先完成 UI/导航整合。
  - 新增的资产统计接口独立建设；旧管理接口保持兼容。

- Decision: webapp 与 MiniApp 共享同一能力边界，但保留不同交互容器
  - MiniApp 使用底部导航进入资产管理模块，再通过页签切换用户视图与管理员视图。
  - webapp 使用独立工作区模块接入资产管理能力，并保留桌面布局下更宽的图表与排行区域。
  - 双端共享统计口径与核心接口，但不强制要求完全一致的视觉布局。

## Risks / Trade-offs
- 聪明钱钱包总资产不是“全钱包绝对精确净值”
  - Mitigation: 明确命名为“可识别总资产”，并在 UI 中展示口径说明。
- 跨日汇总任务失败会导致某天历史图断点
  - Mitigation: 汇总任务幂等、支持补算、在接口中返回缺失日期提示。
- 不同时区会导致“当天不参与统计”的边界感知差异
  - Mitigation: 第一阶段统一使用服务端时区，并在前端文案中明确“按北京时间”。
- Smart Money 的估算已实现收益依赖 add/remove 事件配对与 USD 快照质量
  - Mitigation: 对未匹配和缺失快照进行计数与显式提示，不静默混入排行榜。
- MiniApp 旧管理页迁移会带来入口变更成本
  - Mitigation: 第一阶段保留兼容路由或兼容入口跳转，避免管理员找不到原有功能。

## Migration Plan
1. 新增日快照 / 日统计表并接入自动迁移。
2. 实现用户侧和聪明钱侧的按日汇总服务、补算入口与调度任务。
3. 增加用户资产接口与管理员聪明钱资产接口。
4. 在 MiniApp 新增资产管理入口，并迁移旧管理页能力。
5. 在 webapp 新增资产管理工作区，并实现与 MiniApp 对齐的双端视图。
6. 补充边界测试、回填说明与运营文档。

## Open Questions
- 是否要在第一期就支持“按单钱包筛选用户资产”，还是先默认展示“用户全量钱包聚合”即可。
- 聪明钱排行榜是否还需要增加“近 7 天连续盈利天数”一类行为指标；当前提案先不纳入核心范围。
