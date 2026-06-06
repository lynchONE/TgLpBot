# Design: 替换 OKX 高频读数据 API 并移除 SmartMoney DeFi

## 目标
- 降低 OKX 收费后高频读数据链路的持续成本。
- 保持核心交易执行链路稳定，避免在同一变更里替换 swap provider。
- 明确免费方案的额度约束，并通过批量查询、请求合并和 RPC 校验控制调用量。
- 删除 SmartMoney OKX DeFi 仓位功能，避免继续维护 OKX DeFi 依赖。
- K 线展示效果以 OKX 为准，本次继续保留 OKX Market candles。

## 非目标
- 不替换 OKX swap/approve。
- 不替换 OKX basic-info 头像源和 OKX advanced-info 风控。
- 不替换 OKX Market candles K 线。
- 不引入需要付费才能满足基础查询量的强依赖。
- 不用长缓存掩盖实时价格和正在形成 K 线的变化。

## 市场数据方案
价格使用 GeckoTerminal simple token price 批量接口。限制和处理方式：
- 按 `chain + token_address[]` 批量查询，避免每个 token 单独请求。
- 对同一批次请求做 singleflight 合并。
- 只保留 2 秒级短缓存用于页面瞬时重复请求，不使用分钟级或更长实时价格缓存。
- 稳定币与 wrapped native 继续使用明确资产规则处理；交易执行和成交金额仍以 swap quote 或链上结果为准。

K 线继续使用 OKX Market candles：
- `token_candles` 直接按 `chain + token_address + bar + limit + before/after` 请求 OKX candles。
- 后端将 OKX 毫秒时间戳、字符串价格和成交量归一化为前端 candles JSON。
- 正在形成的最后一根 K 线仅做秒级请求合并或短缓存，避免旧数据覆盖实时展示。

metadata 拆成两类：
- 链上静态 metadata：`decimals/symbol/name` 通过 RPC 合约调用读取，并持久化缓存。
- 展示增强 metadata：logo 等优先从 OKX `token/basic-info` 补充，缺失时继续尝试 GeckoTerminal token 接口、DexScreener 批量 token 接口和 Trust Wallet 静态资产；全部失败时保留 RPC metadata 并暴露缺失展示信息。display metadata 与 RPC 结果独立合并，RPC 未返回某个 token 时不得丢弃 provider 已返回的有效 logo。

## 风控方案
token 风控继续使用 OKX `market/token/advanced-info`。原因是当前调用量不高，且 GoPlus 等免费风控源额度较低，贸然替换会增加风控不确定性。本次仅确认不改动 `token_risk.go` 的 OKX 风控链路。

## 钱包余额方案
默认使用 RPC：
- 原生币余额用 `eth_getBalance`。
- ERC20/BEP20 余额用 `balanceOf` 扫描项目已知 token 集。
- 已知 token 集来源包括稳定币、wrapped native、用户策略任务 token、交易历史 token、钱包兑换限价单 token、热门池目录 token。
- 候选 token 去重并设置查询上限，避免免费 RPC 被一次预览打爆。

第三方钱包 API 只作为未来可选发现增强，不作为默认依赖。Alchemy、Moralis、GoldRush/Covalent、DeBank、Zerion 等服务通常需要 key 且免费额度有限；即使启用，也只能用于发现候选 token，非零余额仍必须通过 RPC 校验。

## SmartMoney DeFi 删除
删除范围包括：
- OKX DeFi user asset platform list/detail client 方法、response 类型和相关缓存。
- SmartMoney DeFi overview/detail 后端路由、handler 和测试。
- `webapp` / `miniapp` 中调用 DeFi overview/detail 的 API、面板、卡片和入口。

保留范围包括：
- SmartMoney 钱包监听、LP 仓位、池子详情、跟单和活动流。
- 与 OKX swap 执行无关的 SmartMoney 现有核心功能。

## 风险与缓解
- 免费市场数据源可能限流：通过批量、请求合并、短错误传播和必要的 provider 扩展点缓解。
- RPC 扫余额无法发现未知 token：通过项目已知 token 集覆盖业务内 token，避免依赖低额度第三方钱包 API。
- OKX K 线调用量回升：通过秒级请求合并和短缓存控制重复请求；如果后续成本再次成为问题，需要单独评估效果等价的数据源，而不是直接回退到展示效果较差的免费源。
