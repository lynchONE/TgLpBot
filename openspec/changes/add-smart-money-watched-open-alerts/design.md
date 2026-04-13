## Context
- Smart Money 模块已经有“监控通知”页和一套 Bark 复用链路，当前用于“金狗通知 / 聪明钱聚集”。
- Smart Money watcher 已经在后端统一消费 LP `add/remove` 事件，并支持通过 `SetNotifier` 在事件落库后触发广播。
- Web 端已有“特别关注钱包”的交互入口，但当前仅保存在浏览器本地存储：
  - 无法跨设备同步。
  - 后端无法据此知道“当前用户关注了谁”，因此无法可靠发送 Bark。
  - MiniApp 端也无法复用。
- Bark 的基础配置（Key / Server / Group）已在全局配置中存在，适合继续复用。

## Goals / Non-Goals
- Goals:
  - 在 Smart Money“监控通知”内新增“特别关注开仓”子页。
  - 将“特别关注钱包”升级为用户级服务端持久化配置。
  - 当特别关注的钱包发生 `add` 事件时，支持 Bark 推送。
  - 当前台页面处于活跃状态且用户开启提示音时，播放一声短音“滴”。
  - 保持提醒链路实时，不再新增一套重复轮询。
- Non-Goals:
  - 不做历史事件补发或回溯提醒。
  - 不做自定义铃声库，提示音固定为一声短音。
  - 不在本次变更中引入原生 iOS / Android SDK 推送，仅复用 Bark。
  - 不改动 Smart Money watcher 的主扫描逻辑和事件入库路径。

## Decisions
- Decision: 新增用户级特别关注钱包表 `smart_money_user_watch_wallets`
  - 以 `user_id + chain + wallet_address` 唯一。
  - 该表是“谁被当前用户特别关注”的唯一事实来源。
  - Web K 线原本的本地 watch 状态保留为兼容缓存，但以服务端结果为准。
- Decision: 新增提醒配置表 `smart_money_watch_open_alert_configs`
  - 以 `user_id + chain` 唯一。
  - 配置项包含：
    - `enabled`
    - `bark_enabled`
    - `sound_enabled`
  - 不重复保存 Bark Key / Server / Group，继续从 `GlobalConfig` 读取。
- Decision: 新增事件去重表 `smart_money_watch_open_alert_receipts`
  - 以 `user_id + chain + tx_hash + log_index` 唯一。
  - 作用是保证同一条开仓事件最多只给某个用户发送一次 Bark。
  - 不增加用户级冷却时间；本场景需要“每次真实开仓都提醒”，只做事件级去重。
- Decision: “特别关注开仓提醒”走实时回调，不走定时扫描
  - 直接在 Smart Money watcher 的事件回调里复用当前事件。
  - 优点：
    - 更及时。
    - 不需要重复扫 ClickHouse。
    - 更自然地基于单个事件做去重。
- Decision: 前台提示音由前端本地判断并播放
  - 后端继续广播统一 `lp_event`。
  - MiniApp / Web 获取当前用户 watchlist 和提醒配置后，在前端本地判断：
    - 事件类型为 `add`
    - 钱包在 watchlist 中
    - `sound_enabled=true`
    - 当前 Smart Money 页面处于活跃状态
  - 满足时使用 Web Audio API 播放一声短音。
  - 若 Telegram WebView / 浏览器自动播放策略拦截，则静默降级。
- Decision: 监控通知页内新增第三个子页 `特别关注开仓`
  - 继续沿用现有 `GoldenDogPanelContent / GoldenDogPageContent` 的结构风格。
  - 现有“钱包模式 / 池子模式”保持不变，新子页只负责：
    - 总开关
    - Bark 开关
    - 提示音开关
    - 当前 watchlist 数量 / 列表
    - 测试按钮
- Decision: 特别关注入口同时保留在 Web K 线和 Smart Money 钱包视图中
  - Web K 线入口保持可用。
  - MiniApp / Web 的 Smart Money 钱包列表和钱包详情页补充统一入口，方便直接管理 watchlist。

## Risks / Trade-offs
- 风险：Web Audio 在移动端可能要求用户先有一次交互，首次进入页面时无法立刻自动播放。
  - Mitigation: 将其定义为“前台增强提醒”，允许静默失败；后台提醒由 Bark 兜底。
- 风险：当前 Web K 线里的“特别关注”是本地状态，迁移到服务端后需要兼容旧用户的本地列表。
  - Mitigation: 首版允许页面启动时读取本地列表并尝试一次性同步到服务端，再以服务端为准。
- 风险：一个钱包短时间内连续多次 `add`，用户可能会收到多条 Bark。
  - Mitigation: 设计上按“每一个真实 `add` 事件提醒一次”；这符合“只要开仓就提醒”的用户预期。如后续需要节流，可单独追加变更。

## Migration Plan
1. 新增三张表与 AutoMigrate。
2. 增加 watchlist API 与 watch-open-alert config/test API。
3. 将现有 Web K 线“特别关注”切换为服务端持久化。
4. 在 Smart Money watcher notifier 上挂接新提醒服务。
5. 在 MiniApp / Web 的 Smart Money“监控通知”里新增 `特别关注开仓` 子页。
6. 前端接入 `ws/sm/events`，在前台匹配事件后播放短音。

## Open Questions
- 暂无。
