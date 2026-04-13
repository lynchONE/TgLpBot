## 1. OpenSpec
- [x] 1.1 确认本提案与现有 `金狗通知 / 跟单 / 特别关注` 相关变更无冲突
- [x] 1.2 审阅并批准本提案

## 2. 数据模型与后端接口
- [x] 2.1 新增 `smart_money_user_watch_wallets`、`smart_money_watch_open_alert_configs`、`smart_money_watch_open_alert_receipts` 模型与迁移
- [x] 2.2 新增 `GET/POST /api/smart_money_watch_wallets`
- [x] 2.3 新增 `GET/POST /api/smart_money_watch_open_alert_config`
- [x] 2.4 新增 `POST /api/smart_money_watch_open_alert_test`
- [x] 2.5 为接口补齐 MiniApp / WebApp 鉴权与 Smart Money 权限检查

## 3. 提醒链路
- [x] 3.1 实现“特别关注开仓提醒”服务，并挂接到 Smart Money watcher 的实时事件回调
- [x] 3.2 对 `add` 事件按用户 watchlist + 配置匹配 Bark 推送
- [x] 3.3 对同一 `tx_hash + log_index` 做用户级去重
- [x] 3.4 复用全局 Bark 配置，并支持该功能自己的 `bark_enabled` 开关

## 4. 前端
- [x] 4.1 MiniApp：在 Smart Money -> 监控通知 中新增 `特别关注开仓` 子页
- [x] 4.2 WebApp：在 Smart Money -> 监控通知 中新增 `特别关注开仓` 子页
- [x] 4.3 MiniApp / WebApp：接入 watchlist API 与 watch-open-alert config API
- [x] 4.4 WebApp：将现有 K 线里的“特别关注”从本地存储迁移到服务端持久化
- [x] 4.5 MiniApp / WebApp：接入 `ws/sm/events`，匹配特别关注钱包 `add` 事件后播放一声短提示音
- [x] 4.6 MiniApp / WebApp：在钱包列表或详情里补充特别关注开关

## 5. 验证
- [x] 5.1 后端验证：接口参数校验、事件去重、Bark 开关分支
- [x] 5.2 前端构建验证：`webapp`、`miniapp` 的 `npm run build`
- [ ] 5.3 手工验证：页面前台时触发提示音；页面关闭或后台时只收 Bark
- [ ] 5.4 手工验证：同一条 `add` 事件不会重复 Bark；同一钱包下一次真实 `add` 仍可再次提醒
