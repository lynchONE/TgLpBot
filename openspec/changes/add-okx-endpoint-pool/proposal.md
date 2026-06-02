# Change: OKX API 配置支持池化与自动切换

## Why
当前 OKX DEX API 的 base URL 和 API key/secret/passphrase 主要来自 `.env` 配置。某个 OKX 配置出现限流、额度耗尽、密钥不可用或上游异常时，需要改配置并重启服务，钱包兑换、开仓/加仓/退仓 swap、approve spender 和 token 风控查询都会受影响。

需要像现有 RPC 池和池子数据源一样，让管理员能在运行时新增、禁用、切换 OKX 配置，并在当前配置不可用时自动切到可用配置，减少人工介入和停机风险。

## What Changes
- 新增 DB 驱动的 OKX API 配置池，保存名称、base URL、API key、secret、passphrase、启用状态、current 状态和健康检查字段。
- OKX secret/passphrase 等敏感字段必须加密存储；管理接口和 UI 默认只展示脱敏值，不返回明文 secret。
- `exchange.OKXDexService` 改为通过 OKX 配置池选择当前可用配置；DB 配置为空或 DB 不可用时继续使用 `.env`。
- 失败处理支持自动切换：
  - 对网络错误、5xx、OKX 非成功 code、401/403、429/额度耗尽等记录失败。
  - 连续失败达到阈值后临时禁用当前配置并切换到同池下一个可用配置。
  - 额度耗尽类错误禁用到下个月起始时间。
- 新增管理员接口和 MiniApp/WebApp 管理入口，支持列表、新增、重命名、切换、启用/禁用、删除、连通性检查。
- 启动后台健康检查，定期探测 DB 中启用的 OKX 配置并维护状态。

## Impact
- Affected specs: `okx-endpoint-pool`
- Affected code:
  - `backend/base/models/*`
  - `backend/base/database/mysql.go`
  - `backend/base/okxpool/*` 或同等共享包
  - `backend/service/exchange/okx_dex.go`
  - `backend/service/liquidity/*`
  - `backend/service/web_server/*`
  - `miniapp/src/lib/api.js`
  - `miniapp/src/components/AdminPage.jsx`
  - `miniapp/src/components/*`
  - `miniapp/api/admin.js`
  - `webapp/src/api.js`
  - `webapp/src/components/AdminPanel.jsx`
  - `webapp/src/styles.css`
- Backwards compatibility: DB 池为空时保持现有 `.env` OKX 配置行为。
- Security: 不能在日志、API 响应、前端状态中暴露 OKX secret/passphrase 明文。
