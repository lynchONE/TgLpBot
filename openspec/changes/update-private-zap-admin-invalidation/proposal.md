# Change: 用管理员按链失效替代 Private Zap 版本号失效

## Why
- 当前 Private Zap 的失效策略依赖环境变量版本号，运维需要改配置并重启服务，排查和操作都不直观。
- 管理员真正需要的是“按链让现有绑定失效”，让用户下一次开单时自动重新部署并重新绑定。
- 现有 Redis key 和数据库版本比对逻辑对管理端不透明，增加了理解和维护成本。

## What Changes
- 新增管理员按链失效 Private Zap 绑定的能力，入口放在 Mini App 管理页。
- 管理员点击某条链的失效按钮后，后端会将该链所有 `wallet_chain_contracts(kind=zap_simple)` 的 `contract_address` 置空，并清除对应 Redis 缓存。
- Private Zap 解析逻辑改为：
  - 先查 Redis
  - Redis miss 时查数据库
  - 数据库存在且 `contract_address` 有效则复用
  - 否则按当前链重新部署、配置并绑定
- 不再依赖 `PRIVATE_ZAP_VERSION`、`BSC_PRIVATE_ZAP_VERSION`、`BASE_PRIVATE_ZAP_VERSION` 来决定绑定是否失效。

## Impact
- Affected specs: `zap-contracts`
- Affected code:
  - `backend/service/liquidity/private_zap.go`
  - `backend/service/web_server/*` admin handler / router
  - `miniapp/src/components/SystemConfigCard.jsx`
  - `miniapp/src/lib/api.js`
  - `miniapp/api/admin.js`
- 运维方式变化：
  - 不再通过 bump 配置版本号触发失效
  - 改为管理员在 Mini App 中按链点击失效按钮
