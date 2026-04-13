# 变更：Smart Money 监控通知新增“特别关注开仓提醒”

## 为什么
- 当前 Smart Money 的“监控通知”只覆盖“金狗通知 / 聪明钱聚集”，还不能对“我特别关注的钱包开仓”做单独提醒。
- 现有“特别关注钱包”只存在于 Web K 线页的本地存储中，不是用户级持久化配置，无法跨设备同步，也无法让后端稳定发送 Bark。
- 用户希望在手机端看到被特别关注的聪明钱一旦开仓就收到提醒：页面在前台时播放一声短提示音，页面不在前台时仍可通过 Bark 收到通知。

## 变更内容
- 复用现有 Smart Money“监控通知”页，在当前“金狗通知 / 池子监控”之外新增一个子页：`特别关注开仓`。
- 新增用户级“特别关注钱包”持久化能力：
  - 服务端保存当前用户在指定链上的特别关注钱包列表。
  - Web 现有 K 线里的“加入特别关注 / 取消特别关注”改为读写服务端，而不是只写本地存储。
  - MiniApp / Web 的 Smart Money 模块可读取并展示当前用户的特别关注钱包列表，用于提醒配置和后续扩展。
- 新增“特别关注开仓提醒”配置：
  - 总开关 `enabled`
  - Bark 开关 `bark_enabled`
  - 前台提示音开关 `sound_enabled`
  - 提醒链 `chain`
  - 可选测试按钮
- 新增后端提醒链路：
  - 不新增定时扫描器，直接挂接现有 Smart Money watcher 的实时 LP 事件回调。
  - 当某个“特别关注钱包”产生 `add` 事件时，按用户配置决定是否发送 Bark。
  - 同一条链上事件只通知一次，避免 watcher 重扫、服务重启或重复回放导致重复推送。
- 新增前端前台提示音：
  - MiniApp / Web 在 Smart Money 页面活跃时订阅现有 `ws/sm/events`。
  - 收到匹配“我特别关注的钱包 + add 事件”的消息后，如果 `sound_enabled=true`，播放一声固定短音“滴”。
  - 浏览器或 Telegram WebView 因自动播放策略阻止时，允许静默降级，不影响 Bark。

## 影响
- 新增 MySQL 表：
  - `smart_money_user_watch_wallets`
  - `smart_money_watch_open_alert_configs`
  - `smart_money_watch_open_alert_receipts`
- 新增后端接口：
  - `GET/POST /api/smart_money_watch_wallets`
  - `GET/POST /api/smart_money_watch_open_alert_config`
  - `POST /api/smart_money_watch_open_alert_test`
- 影响后端模块：
  - `backend/service/web_server/smart_money.go`
  - `backend/service/web_server/smart_money_watch_open_alert.go`
  - `backend/service/smart_money_watch_open_alert/*`
  - `backend/base/models/*`
  - `backend/base/database/mysql.go`
- 影响前端模块：
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `miniapp/src/lib/smartMoneyApi.js`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `webapp/src/smartMoneyApi.js`
  - `webapp/src/App.jsx`
