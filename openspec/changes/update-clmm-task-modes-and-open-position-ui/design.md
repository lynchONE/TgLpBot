## Context
当前 CLMM 任务的自动越界处理主要由 `rebalance_enabled` 决定，`paused` 只是在轮询层直接跳过执行。这个建模对“双向再平衡 / 双向撤出终止”足够，但无法覆盖本次新增的“上破再平衡、下破撤出终止”模式。

同时，前端目前把“暂停任务”和“再平衡开关”拆成两个入口，用户需要自行理解它们之间的优先级；开仓页则仍默认开启再平衡，并保留了较重的仓位建议和前置兑换确认块，整体决策路径偏长。

## Goals / Non-Goals
- Goals:
  - 为 CLMM 任务引入明确的越界模式枚举，覆盖本次新增的非对称模式。
  - 保持 `paused` 现有运行语义不变，但在前端交互上统一为可感知的“任务模式”入口。
  - 让开仓页和仓位卡使用一致的模式文案、按钮分组和默认值。
  - 简化前置兑换展示，去掉额外复选框，同时不降低执行前确认的安全性。
- Non-Goals:
  - 本次不重构 Telegram Bot 的整套任务创建交互，只保证后端字段和兼容逻辑可继续服务 Bot。
  - 本次不移除数据库中的 `rebalance_enabled`、`stop_loss_enabled` 等旧字段。
  - 本次不删除后端 `sizing_advice` 返回结构，只移除开仓页对该信息的默认展示。

## Decisions
- Decision: 为任务新增持久化字段 `out_of_range_mode`
  - 建议取值：
    - `rebalance_all`
    - `exit_all`
    - `rebalance_up_exit_down`
  - 旧字段 `rebalance_enabled` 继续保留，用于兼容旧客户端和旧数据：
    - `true` 映射为 `rebalance_all`
    - `false` 映射为 `exit_all`
  - 新接口与实时仓位响应优先读写 `out_of_range_mode`，旧字段作为兼容镜像同步维护。
  - Why: 非对称模式已经无法由单个布尔表示，继续叠加布尔条件会让策略分支和前端状态解释越来越混乱。

- Decision: `paused` 继续作为独立状态，不直接并入 `out_of_range_mode`
  - 前端可以把“暂停任务”显示为第四个模式按钮，但后端持久化仍保持为：
    - `paused=true`
    - `out_of_range_mode` 保留用户最近一次选择的自动处理模式
  - 当用户从暂停切回某个自动模式时：
    - 更新 `out_of_range_mode`
    - 同时将 `paused=false`
  - 当用户切到暂停时：
    - 仅将 `paused=true`
    - 不覆盖已保存的 `out_of_range_mode`
  - Why: 现有暂停逻辑已经稳定，若把暂停直接存成枚举值，会额外引入“恢复时该回到哪个模式”的状态恢复问题。

- Decision: 开仓接口新增 `task_mode` 请求字段，作为新前端的统一提交入口
  - 建议支持：
    - `rebalance_all`
    - `exit_all`
    - `rebalance_up_exit_down`
    - `pause`
  - 当 `task_mode=pause` 时：
    - 任务创建后 `paused=true`
    - `out_of_range_mode` 保存为当前界面中对应的自动模式默认值；若用户未额外指定，则使用 `exit_all`
  - 当 `task_mode` 缺失时，回退到旧逻辑：
    - 优先读 `rebalance_enabled`
    - 再回退默认值 `exit_all`
  - Why: 让新旧客户端共存，同时避免为一次开仓提交两个容易冲突的前端字段。

- Decision: 新增独立的任务模式更新接口，旧的 `toggle_rebalance` 保持兼容
  - 新增 `POST /api/task_update_mode`
  - 请求支持：
    - `taskMode`
    - 可选 `paused`
  - 旧的 `task_toggle_rebalance` 继续可用，但仅负责在 `rebalance_all` 与 `exit_all` 之间切换
  - Why: 新模式已经超出 toggle 的表达能力，继续复用 toggle 命名会让接口语义失真。

- Decision: 越界执行逻辑改为“方向 + 模式”双维度分发
  - `rebalance_all`：
    - 上破 => 再平衡
    - 下破 => 再平衡
  - `exit_all`：
    - 上破 => 撤出并结束
    - 下破 => 撤出并结束
  - `rebalance_up_exit_down`：
    - 上破 => 再平衡
    - 下破 => 撤出并结束
  - `paused=true`：
    - 不进入任何自动处理分支
  - 单边池 `range_activation_pending` 的延迟激活逻辑保持不变，并继续优先于模式分发。
  - Why: 这样可以在不改动现有重试链路的前提下，只重写触发条件与分发规则。

- Decision: 开仓页去掉三档建议展示，但保留后端返回
  - `preview_open_position` 仍可返回 `sizing_advice`
  - WebApp / MiniApp 默认不展示保守 / 中性 / 激进三档卡片
  - Why: 这部分计算不是本次主决策路径，先隐藏展示可显著降低界面噪声，同时不破坏现有接口兼容性。

- Decision: 前置兑换改为紧凑摘要 + 最终提交即确认
  - 页面仍保留：
    - 推荐滑点
    - 当前滑点
    - 预计到账
    - 兑换路径
    - 前置兑换滑点输入框
  - 页面移除额外复选框“我已确认本次前置兑换”
  - 当预览存在且需要前置兑换时，用户点击最终“确认开仓”即向后端提交 `confirm_entry_swap=true`
  - Why: 用户已经在同一张开仓卡片里完成查看和点击确认，再增加复选框收益很低，只会拉长操作路径。

## Risks / Trade-offs
- 风险: 新旧字段并存期间，`out_of_range_mode` 与 `rebalance_enabled` 可能出现不一致。
  - Mitigation: 所有写路径统一同时维护两个字段；所有读路径优先读 `out_of_range_mode`，缺失时再回退旧字段。

- 风险: 把“暂停任务”作为模式按钮展示，用户可能误解为暂停会覆盖原模式。
  - Mitigation: 卡片文案明确写成“暂停（保留原模式）”或在恢复时直接高亮恢复后的模式。

- 风险: 去掉前置兑换复选框后，如果前端没有正确基于预览自动传 `confirm_entry_swap=true`，会导致开仓被后端拦截。
  - Mitigation: 在 WebApp / MiniApp 提交层统一封装，不让组件自行拼接该确认字段，并补充接口单测与前端构建验证。

## Migration Plan
1. 为 `strategy_tasks` 增加 `out_of_range_mode` 字段，并在模型层定义默认值 `exit_all`。
2. 在任务创建、任务模式更新、旧 toggle 接口中同步维护 `out_of_range_mode` 与 `rebalance_enabled` 的兼容映射。
3. 重写策略层越界分发逻辑，按方向与模式执行再平衡或撤出终止；暂停态继续直接跳过。
4. 扩展实时仓位响应，返回当前模式字段与展示文案，供 WebApp / MiniApp 仓位卡渲染。
5. 调整开仓页与仓位卡交互，统一为模式按钮组；同步移除三档建议展示和前置兑换复选框。
6. 补充后端单测与前端构建验证。

## Open Questions
- WebApp 的旧仓位列表页是否也要同步从“自动 / 手动”切换为完整四模式按钮，还是先只覆盖 MiniApp 与当前开仓弹窗。
- Bot 侧是否需要在后续单独补一个“任务模式”配置入口；本次若不做，至少要确保 Bot 创建任务时仍能按兼容映射落到正确默认值。
