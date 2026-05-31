## Context
聪明钱页面现在已有本地 LP 事件、active position 读模型、钱包余额展示和持仓详情卡片。用户反馈“聪明钱哪里的仓位数据经常不对”，目标不是替换所有本地数据，而是在钱包详情、特殊关注钱包和持仓详情里接入 OKX DeFi 当前资产能力，优先展示更接近钱包实时状态的多链 DeFi 仓位、金额、手续费和区间。

## Goals / Non-Goals
- Goals:
  - 为单钱包按需读取 OKX DeFi 平台列表，展示各链和平台维度持仓金额。
  - 为单个平台按需读取 OKX DeFi 平台详情，展示投资品、position、手续费、金额和区间。
  - 让 WebApp 与 MiniApp 的聪明钱钱包详情、特殊关注钱包和持仓详情共用同一后端语义。
  - OKX 数据失败时清晰返回错误状态，避免静默降级成 0 或空数据。
- Non-Goals:
  - 不改变开仓、撤仓、跟单执行、nonce、授权或 OKX swap 交易链路。
  - 不要求把 OKX DeFi 数据持久化为新的仓位事实表；本次优先做短 TTL 缓存和展示层增强。
  - 不在钱包列表页同步拉取所有平台详情，避免请求爆炸。

## Decisions
- Decision: OKX DeFi 客户端放在现有 `exchange.OKXDexService` 中。
  Rationale: 现有 OKX API key、secret、passphrase、签名和 HTTP 客户端已集中在该服务中，新增 DeFi wallet 端点可复用同一鉴权机制。

- Decision: 平台列表和平台详情拆成两个后端接口/响应层次。
  Rationale: 钱包详情页需要概览和链维度金额，点击某个持仓时才需要昂贵的详情数据。这样能减少列表页延迟和 OKX 限流风险。

- Decision: 对外响应保留 OKX 原始关键字段，同时提供页面直接使用的规范化字段。
  Rationale: OKX DeFi 响应字段较丰富且可能随平台变化；保留 raw/extra 能便于后续排查，规范化字段用于稳定展示金额、手续费和区间。

- Decision: OKX DeFi 数据作为外部数据源显式带 `source`、`status`、`updated_at` 和 `warnings`。
  Rationale: 外部接口不可靠时需要让页面和用户知道数据是否来自 OKX、是否过期或失败，不能用无声 fallback 掩盖真实错误。

## Risks / Trade-offs
- OKX 字段形态可能与文档示例有差异。
  Mitigation: 解析时对外部 API 字段采用结构化类型加 `json.RawMessage` 承接扩展字段；关键金额字段只在成功解析时输出规范化数值。
- 平台详情请求过多会拖慢钱包详情。
  Mitigation: 钱包详情默认只取平台列表；平台详情按用户点击加载，并使用短 TTL 缓存。
- 多链数据与本地 chain_id 命名不一致。
  Mitigation: 后端统一输出 `chain_index`、`chain_id`、`chain` 三类字段，前端按可用字段展示，不自行推断关键金额。

## Migration Plan
1. 新增 OKX DeFi 客户端方法和单元测试。
2. 新增聪明钱 OKX DeFi 概览与详情接口，接入权限校验、缓存和错误状态。
3. WebApp 与 MiniApp 钱包详情和特殊关注钱包接入概览，持仓点击按需加载详情。
4. 运行后端测试和前端构建，做针对性 diff 检查，确认没有触碰交易执行链路。

## Open Questions
- OKX 平台详情中手续费字段是否对所有 CLMM 平台统一命名；如果部分平台只在 `positionList` 的扩展字段中返回，需要以实际响应做字段映射补充。
