# Change: 为热门池子增加代币元数据缓存与展示

## Why
- 热门池子当前只返回池子分析数据，前端只能用交易所图标占位，无法展示池子中真正的主题代币图标。
- 代币图标与名称来自 OKX `token/basic-info` 接口，如果每次请求都实时拉取，会引入额外延迟、上游波动和重复请求成本。
- 这类数据天然属于低频变更元数据，更适合走持久化维表 + Redis 热缓存，而不是直接塞进 ClickHouse 热门池子事实查询。

## What Changes
- 新增代币元数据持久化层，按 `chain + token_address` 保存 symbol、name、logo_url、来源与过期时间。
- 新增代币元数据服务，优先读 Redis，其次读 MySQL，最后批量调用 OKX `token/basic-info` 补齐 miss。
- 热门池子接口在返回前根据池子交易对挑选展示代币，并附加 `display_token_*` 字段。
- `webapp` 热门池子列表将头像位切换为主题代币图标，同时将交易所图标缩小后放到协议版本标签旁展示。

## Impact
- Affected specs: `hot-pool-token-metadata`
- Affected code:
  - `backend/base/models/*`
  - `backend/base/database/mysql.go`
  - `backend/service/token_metadata/*`
  - `backend/service/web_server/hot_pools.go`
  - `backend/service/web_server/server.go`
  - `webapp/src/App.jsx`
  - `webapp/src/styles.css`
