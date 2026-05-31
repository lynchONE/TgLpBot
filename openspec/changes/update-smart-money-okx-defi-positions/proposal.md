# Change: 接入 OKX DeFi 钱包持仓优化聪明钱仓位展示

## Why
- 当前聪明钱仓位主要依赖本地 LP 事件、链上读取和快照聚合，部分钱包的仓位金额、手续费和区间经常不准，尤其是钱包详情页和特殊关注钱包需要看到各链当前 DeFi 仓位时，本地数据无法完整覆盖。
- OKX 已提供 DeFi 用户资产平台列表和平台详情接口，可以作为钱包当前 DeFi 持仓的外部权威补充，减少单纯依赖事件回放和 RPC 读取造成的缺失、延迟和误差。

## What Changes
- 后端新增 OKX DeFi 用户资产平台列表与平台详情客户端能力，复用现有 OKX 鉴权配置，请求 `defi-user-asset-platform-list` 与 `defi-user-asset-platform-detail` 对应能力。
- 聪明钱钱包接口新增 OKX DeFi 数据聚合：
  - 钱包详情页返回该钱包各链 DeFi 资产概览、协议/平台列表、平台金额、链维度金额和数据更新时间。
  - 特殊关注钱包列表与详情复用同一聚合数据，展示关注钱包的各链 DeFi 仓位概况。
  - 单个持仓详情可返回 OKX 平台详情中的投资品、position 列表、手续费、仓位金额、价格区间或 tick 区间等重点字段。
- WebApp 和 MiniApp 聪明钱视图补充 OKX DeFi 展示：
  - 钱包详情页展示各链数据、平台持仓和金额汇总。
  - 特殊关注钱包展示 OKX DeFi 总额和链维度概览。
  - 点击某个 OKX 持仓后展示详细数据，重点突出手续费、仓位金额和区间。
- 兼容性：
  - 保留现有本地聪明钱 LP 事件、仓位详情和手续费快照链路。
  - OKX 数据失败时必须显式返回错误/告警状态，不用静默默认值掩盖真实问题。

## Impact
- Affected specs:
  - `webapp-smart-money`
  - `miniapp-smart-money`
  - `analytics-performance`
- Affected code:
  - `backend/service/exchange/okx_dex.go`
  - `backend/service/web_server/smart_money.go`
  - `backend/service/smart_money/*`
  - `backend/service/assets/*`
  - `webapp/src/smartMoneyApi.js`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `miniapp/src/lib/smartMoneyApi.js`
  - `miniapp/src/components/SmartMoneyPage.jsx`
- Risks:
  - OKX 外部接口可能限流或字段形态变化，需要超时、缓存、错误透传和最小化请求次数。
  - DeFi 详情请求可能按平台维度放大请求数，必须避免在列表页同步拉取所有平台明细。
