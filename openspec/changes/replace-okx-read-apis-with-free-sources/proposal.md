# Change: 替换 OKX 高频读数据 API 并移除 SmartMoney DeFi

## Why
OKX DEX API 开始收费后，当前价格、K 线、metadata、钱包余额、SmartMoney DeFi 仓位等读数据链路会产生持续调用成本。核心开仓、加仓、退出和钱包兑换执行链路调用 OKX swap/approve 的量不大，并且直接影响交易执行，本次不替换；token 风控调用量也不高，按最新范围继续保留 OKX advanced-info。

免费方案也有明确限制：公开市场数据 API 通常有分钟级或日级额度，第三方钱包余额 API 通常需要 key 且免费额度有限。替换方案需要通过批量查询、请求合并、已知 token 集合和 RPC 校验来满足项目需求，不能把第三方失败伪装成安全或正确结果。

## What Changes
- 价格、K 线、metadata 读数据链路不再依赖 OKX market API，改为 GeckoTerminal 等免费市场数据源与 RPC 组合。
- 价格查询使用批量查询和极短请求合并，不做会影响实时查询的长缓存；交易执行仍以 swap quote 为准。
- K 线查询使用免费 OHLCV 数据源；已收盘 K 线可缓存，正在形成的最后一根 K 线不做长缓存。
- token metadata 使用 RPC 读取 `symbol/name/decimals`，logo 等展示信息使用免费市场数据源补充。
- 钱包余额预览不再调用 OKX balance API，默认使用 RPC 扫描项目已知 token 集；第三方钱包 API 不作为默认依赖。
- 删除 SmartMoney 的 OKX DeFi 仓位功能，包括 OKX DeFi client、后端接口、前端入口和相关展示代码。
- 保留 OKX swap/approve 交易执行链路，保留 OKX advanced-info token 风控链路。

## Impact
- Affected specs: `market-data`, `wallet-balance`, `smart-money-defi`
- Affected code:
  - `backend/service/exchange/okx_dex.go`
  - `backend/service/pricing/token_price.go`
  - `backend/service/token_metadata/`
  - `backend/service/web_server/token_candles.go`
  - `backend/service/web_server/wallet_swap_api.go`
  - `backend/service/web_server/smart_money.go`
  - `backend/service/web_server/smart_money_okx_defi.go`
  - SmartMoney DeFi 相关 `webapp` / `miniapp` API 与页面代码
- Non-goals:
  - 不替换 OKX DEX `swap` / `approve-transaction`。
  - 不替换 token 风控的 OKX `market/token/advanced-info`。
  - 不改动核心 LP 开仓、加仓、退出和余额兑换执行路径。
  - 不因免费源失败而返回“价格为 0”“安全”或其他会掩盖错误的默认结果。
