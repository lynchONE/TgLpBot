## ADDED Requirements

### Requirement: Smart Money 监控通知支持“特别关注开仓”子页
Smart Money 模块 SHALL 在现有“监控通知”内提供一个新的子页 `特别关注开仓`。

该子页 SHALL 提供以下配置项：
- 总开关 `enabled`
- Bark 开关 `bark_enabled`
- 前台提示音开关 `sound_enabled`
- 当前用户特别关注钱包列表或数量

该子页 SHALL 继续复用用户在全局配置中的 Bark 配置，不在此处重复维护 Bark Key / Server / Group。

#### Scenario: 用户打开特别关注开仓子页
- **WHEN** 用户进入 Smart Money 模块并切换到“监控通知”里的 `特别关注开仓`
- **THEN** 页面展示该提醒能力的开关配置与当前特别关注钱包信息

#### Scenario: 用户保存提醒配置
- **WHEN** 用户修改 `enabled`、`bark_enabled` 或 `sound_enabled` 并保存
- **THEN** 后端持久化配置，后续提醒链路使用最新配置

### Requirement: 特别关注钱包必须按用户持久化
系统 SHALL 将“特别关注钱包”保存为用户级配置，而不是仅保存在浏览器本地状态。

特别关注钱包配置 SHALL 至少按 `user_id + chain + wallet_address` 维度存储，并能被 MiniApp 与 WebApp 共同读取。

#### Scenario: 用户在 Web 端特别关注一个钱包
- **WHEN** 用户在 Web 端将某个聪明钱钱包标记为特别关注
- **THEN** 服务端保存该用户对该钱包的特别关注状态

#### Scenario: 用户在移动端查看特别关注列表
- **WHEN** 同一用户在 MiniApp 中打开 `特别关注开仓`
- **THEN** 页面可以读取到该用户已保存的特别关注钱包列表或数量

### Requirement: 特别关注钱包开仓支持 Bark 提醒
当用户开启 `enabled=true` 且 `bark_enabled=true` 时，系统 SHALL 在特别关注钱包产生 `add` 事件时发送 Bark 提醒。

提醒内容 SHALL 包含：
- 钱包标识（地址或标签）
- 交易对名称
- 事件动作（开仓 / `add`）
- 跳转或追踪所需的基础信息，例如 `tx_hash`

#### Scenario: 特别关注钱包发生开仓
- **GIVEN** 用户已特别关注某个钱包，并开启了 Bark 提醒
- **WHEN** 该钱包产生一条新的 LP `add` 事件
- **THEN** 后端向该用户发送一条 Bark 提醒

#### Scenario: 该功能 Bark 开关关闭
- **GIVEN** 用户已开启特别关注开仓提醒，但 `bark_enabled=false`
- **WHEN** 特别关注钱包产生新的 LP `add` 事件
- **THEN** 后端 MUST NOT 发送 Bark 提醒

### Requirement: 特别关注钱包开仓支持前台提示音
当用户开启 `sound_enabled=true` 时，Smart Money 页面在前台收到匹配的特别关注钱包 `add` 事件后 SHALL 播放一声短提示音。

提示音 SHALL 固定为一声“滴”，不提供铃声自定义。

#### Scenario: 页面在前台且提示音开启
- **GIVEN** 用户已开启 `sound_enabled`
- **AND** Smart Money 页面当前处于活跃状态
- **WHEN** 前端收到一个属于当前用户特别关注钱包的 LP `add` 事件
- **THEN** 页面播放一声短提示音

#### Scenario: 页面不在前台或浏览器阻止自动播放
- **WHEN** 前端因页面状态或浏览器策略无法播放提示音
- **THEN** 系统允许静默降级，且不影响 Bark 提醒

### Requirement: 特别关注开仓提醒必须按事件去重
系统 SHALL 对特别关注钱包的开仓提醒做事件级去重，避免同一条链上 `add` 事件重复提醒同一用户。

#### Scenario: watcher 重扫同一条 add 事件
- **GIVEN** 某个特别关注钱包的 `add` 事件已经成功提醒过当前用户
- **WHEN** 后端再次处理到相同 `tx_hash + log_index` 的事件
- **THEN** 系统 MUST NOT 再次向该用户发送重复提醒

### Requirement: 特别关注开仓提醒配置接口
后端 SHALL 提供特别关注钱包列表与“特别关注开仓提醒”配置接口。

这些接口 SHALL：
- 要求 Telegram WebApp `initData` 鉴权
- 要求 MiniApp / WebApp 具备 Smart Money 权限
- 按用户维度返回 watchlist 与提醒配置
- 返回 JSON 响应

#### Scenario: 未登录用户请求配置
- **WHEN** 请求缺少有效 `initData`
- **THEN** 接口返回 HTTP `401`

#### Scenario: 用户更新特别关注列表
- **WHEN** 用户提交新的特别关注钱包列表，或切换单个钱包的特别关注状态
- **THEN** 后端返回保存后的用户级特别关注结果

#### Scenario: 用户更新提醒配置
- **WHEN** 用户提交新的 `enabled`、`bark_enabled`、`sound_enabled`
- **THEN** 后端返回保存后的提醒配置
