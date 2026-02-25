# 变更：用户可配置的链模式（多链开关 + 默认链）

## 为什么要做
- 后端已经支持多链（BSC + Base），但目前的交互会经常要求用户选链（Bot 流程）或手动选链（Mini App 搜索）。
- 大多数用户长期只在一条链上操作（常见是 BSC）。每次都选链会增加操作摩擦，也更容易选错链。
- 需要一个“按用户”的安全开关：想用多链的人继续使用；想默认单链的人不再被强制选链。

## 具体改动
- 在 `GlobalConfig` 增加 2 个按用户配置字段：
  - `multi_chain_enabled`：是否启用“多链选择”交互（默认：`true`，保持当前行为不变）。
  - `default_chain`：默认链（默认：`bsc`）。
- Bot：
  - `/config` 增加多链开关、默认链设置入口。
  - 当 `multi_chain_enabled=false` 时，“开仓（new position）”不再弹出选链键盘，直接使用 `default_chain`。
  - 当 `multi_chain_enabled=false` 时，其它需要链上下文的操作（例如钱包 dust swap）也直接使用 `default_chain`，不再要求选链。
  - 当 `multi_chain_enabled=true` 时，保持现有逻辑（服务器开启多链时，需要先选链）。
- Mini App：
  - 当 `multi_chain_enabled=false` 时，链选择器（池子搜索 / 开仓）锁定为 `default_chain`，用户无需选择。
  - 后端 API 在 multi-chain 关闭时强制使用“有效链”（忽略客户端传入的 `chain`），避免前端传错导致走错链。

## 影响范围
- Backend model & DB schema：
  - `backend/base/models/lp_config.go`（`GlobalConfig` 新增列）
  - GORM AutoMigrate 会在启动时自动补齐新列。
- Backend services：
  - `backend/service/user/global_config.go`（默认值 + 更新逻辑）
  - `backend/service/web_server/open_position.go`（有效链解析与覆盖）
- Bot：
  - `backend/service/bot/config_menu.go`、`backend/service/bot/config_callbacks.go`、`backend/service/bot/input_handlers.go`、`backend/service/bot/bot.go`（配置入口 + 行为调整）
- Mini App：
  - `miniapp/src/App.jsx`（按配置锁定/隐藏链选择）
  - `miniapp/src/lib/api.js`（可选：允许 open position 请求不传 `chain`）

## 向后兼容
- 为了不改变现有用户行为，`multi_chain_enabled` 默认 `true`（仍可选链）。
- 不想选链的用户可以在全局配置里关闭 multi-chain 并设置 `default_chain`。
