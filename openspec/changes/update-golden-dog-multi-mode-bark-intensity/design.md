## Context
当前金狗通知已经具备以下基础能力：
- 配置按 `user_id + chain` 存在 `smart_money_golden_dog_configs`
- 扫描器会定时读取最近的 Smart Money add 事件，按交易对聚合钱包活跃度
- Bark 的 Key / Server / Group 继续复用 `GlobalConfig`

这次需求是在现有能力上继续扩展，而不是推翻重做：
- 保留原有“聪明钱聚集模式”
- 再增加“池子参数模式”
- 为每种模式增加通知强度与测试入口
- 同时优化 WebApp / MiniApp 的配置 UI

此外，PoolM 快照数据已经由 `pool_sync` 持续写入 MySQL `pools` 表，因此新增模式不需要面向用户请求时实时访问外部 PoolM API；直接读取本地快照更稳，也更容易做冷却和测试。

## Goals / Non-Goals
- Goals:
  - 在同一个金狗通知页面中同时承载两种告警模式
  - 让 WebApp / MiniApp 的金狗通知页在桌面和移动端都更紧凑、更易扫读
  - 为每种模式提供独立的 Bark 强度配置
  - 提供一键测试通知能力
  - 复用现有 Bark 全局配置，不在该页重复维护 Bark Key / Server / Group
- Non-Goals:
  - 不做通用规则引擎
  - 不在本次变更中支持“最大值阈值”“区间阈值”“OR 逻辑”
  - 不新增每种模式独立的 Bark Key / Bark Server

## Decisions
- Decision: 配置结构从“单模式平铺字段”演进为“同一配置下的两组模式字段”。
  - Why: 用户明确要求“之前的监控”和“现在新增的监控”都要存在，并且都能配置通知强度；用单行配置扩展字段即可兼容现有逻辑和权限模型。

- Decision: 池子参数模式的阈值采用“多个最小阈值 + AND 组合”。
  - Why: 这是最容易理解且最接近用户描述的第一版行为；任一阈值留空就表示不参与过滤。
  - 覆盖字段：
    - `total_fees`：手续费
    - `transaction_count`：交易笔数
    - `current_pool_value`：TVL
    - `total_volume`：Vol
    - `poolm_fee_rate`：费率
    - `active_liquidity_ratio`：活跃费率 / 活跃流动性占比

- Decision: Bark 强度使用三个固定档位枚举，而不是自由拼装参数。
  - Why: 用户诉求是明确的三档强度；固定档位更利于 UI 呈现和测试。
  - 档位定义：
    - `ring`：普通响铃
    - `persistent_ring`：持续响铃
    - `critical_ring`：静音模式下响铃
  - Bark 参数映射由后端统一封装；当自建 Bark 服务不支持高级参数时，允许降级为普通响铃并返回可观测错误/日志。

- Decision: 新增独立测试接口，而不是复用保存接口。
  - Why: 测试发送不应该依赖配置已经落库，也不应修改运行态配置。
  - 方案：新增 `POST /api/smart_money_golden_dog_test`，接收当前草稿中的模式、强度和文案上下文，直接触发一次 Bark 测试推送。

- Decision: 告警去重继续复用 `smart_money_golden_dog_alert_states`，但将 key 语义升级为“模式前缀 + 目标键”。
  - Why: 可以避免额外建表或重命名旧字段，迁移成本最小。
  - 示例：
    - `wallet_pair:bsc:0xaaa|0xbbb`
    - `pool_metric:bsc:0xpooladdress`

- Decision: 池子参数模式只扫描“新鲜快照”。
  - Why: PoolM 快照如果长期未更新，继续拿来推送会导致误报。
  - 第一版约束：仅当 `pools.updated_at` 在可接受的新鲜窗口内时，池子参数模式才参与告警评估；窗口大小按 `pool_sync` 间隔推导并设置兜底上限。

## Risks / Trade-offs
- Bark 的高级提醒参数在不同服务端版本上的兼容性可能不完全一致。
  - Mitigation: 后端统一封装 Bark 参数；测试接口返回明确错误；运行时记录模式和失败原因。

- 池子参数模式依赖 `pools` 快照刷新，如果 `pool_sync` 中断，会降低告警实时性。
  - Mitigation: 只使用新鲜快照；对陈旧数据不发通知。

- 同一页面承载两套规则会让配置项变多。
  - Mitigation: 使用紧凑卡片 + 摘要条 + 折叠高级项；默认先展示每种模式最重要的 4~6 个控制项。

## Migration Plan
1. 扩展 `smart_money_golden_dog_configs`，把现有字段视为“聪明钱聚集模式”字段并补齐强度字段。
2. 为池子参数模式新增一组字段，默认全部关闭/为空，避免对现有用户产生行为变化。
3. Bark 强度默认值统一为 `ring`，保证老用户升级后行为接近当前版本。
4. 告警状态表继续沿用，新增模式后统一按带前缀的 key 写入。
5. 前端读取新接口结构后，WebApp / MiniApp 页面一起切到双模式视图。

## Open Questions
- “活跃费率”在第一版中按 `active_liquidity_ratio` 实现，并在 UI 上给出辅助说明；如果后续确认需要不同字段，再单独扩展。
