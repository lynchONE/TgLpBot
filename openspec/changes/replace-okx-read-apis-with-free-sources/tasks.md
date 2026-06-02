## 1. 市场数据
- [x] 1.1 梳理现有 OKX price/K 线/metadata 调用点，确认不触碰 OKX swap/approve。
- [x] 1.2 复用免费市场数据 provider，支持批量价格查询和错误透传。
- [x] 1.3 修改 `token_price` 服务：移除 OKX market price 依赖，使用批量查询、请求合并和 2 秒短缓存。
- [x] 1.4 修改 K 线接口：使用免费 OHLCV，最后一根 K 线实时刷新或仅秒级合并。
- [x] 1.5 修改 token metadata：RPC 读取链上静态字段，GeckoTerminal、DexScreener、Trust Wallet 等免费来源补充 logo。

## 2. 风控
- [x] 2.1 按最新范围确认 token 风控继续保留 OKX `market/token/advanced-info`。
- [x] 2.2 确认 `backend/service/web_server/token_risk.go` 未被替换为 GoPlus 或其他 provider。

## 3. 钱包余额
- [x] 3.1 建立钱包余额已知 token 集来源：稳定币、wrapped native、任务、交易历史、钱包兑换限价单和热门池。
- [x] 3.2 用 RPC 扫描原生币和已知 ERC20/BEP20 余额，并控制候选 token 上限。
- [x] 3.3 不接入第三方钱包 API 默认依赖，仅保留未来可选发现增强方案。
- [x] 3.4 修改钱包兑换预览接口，移除 OKX balance API 调用。

## 4. SmartMoney DeFi 删除
- [x] 4.1 删除 OKX DeFi client 方法、response 类型和相关缓存。
- [x] 4.2 删除 SmartMoney DeFi overview/detail 后端路由、handler 和测试。
- [x] 4.3 删除 `webapp` / `miniapp` 的 DeFi API 调用、面板、入口和空状态。
- [x] 4.4 确认 SmartMoney 钱包监听、LP 仓位、活动流等非 DeFi 功能保留。

## 5. 验证
- [x] 5.1 运行 Go 相关测试或针对性编译检查。
- [x] 5.2 运行前端相关 build/test。
- [x] 5.3 做针对性 diff 检查，确认 OKX swap/approve 执行链路未被改动。
- [x] 5.4 搜索确认不再存在 OKX market price/K 线/metadata、OKX balance、OKX DeFi 调用；OKX advanced-info 风控按要求保留。
