## Context
OKX advanced-info 接口可返回 `riskControlLevel` 与 `tokenTags`，当前文档只提供单代币 `GET /token/advanced-info` 查询，未提供批量 advanced-info 查询能力。风控关注点是貔貅盘、低流动性与风险等级，查询对象仅限池子中的非稳定、非主流原生/包装代币，避免对 USDT/USDC/WBNB/BNB 等基础资产产生噪音。

## Goals / Non-Goals
- Goals: 池子列表可扫读风险等级；开单前必须看到代币风控信息；貔貅盘禁止执行真实开仓。
- Goals: 风控结果持久化，池子列表刷新不得对每个代币同步调用 OKX。
- Goals: OKX 查询失败时暴露“未知/失败/限流”状态，不用安全默认值掩盖风险。
- Non-Goals: 不完整透传 OKX 所有原始字段；不新增批量查询假实现。

## Decisions
- Decision: 后端统一查询并聚合风控信息，前端只渲染结构化结果。
- Decision: 新增 `token_risk_snapshots` 表，按 `chain + token_address` 唯一保存 OKX 风控快照、下次刷新时间和最近失败信息。
- Decision: 池子列表/搜索接口只批量读取数据库快照；缺失或过期数据返回“待后台刷新/上次快照”，并进入后台队列。
- Decision: 后台刷新队列全局限速调用 OKX 单币 advanced-info，避免列表刷新触发 429。
- Decision: 开仓链路在缺失或过期时执行单币即时刷新并写库；若 OKX 限流或失败，返回明确未知/失败提示。
- Decision: 风控等级与标签作为开仓检查项返回；貔貅盘为 `fail`，低流动性或中高以上等级为 `warn`。

## Risks / Trade-offs
- OKX 接口异常会影响风控信息完整性；响应会明确标注查询失败，避免误判为安全。
- 快照会有短时间滞后；高风险、低流动性和错误结果使用更短 TTL，普通结果使用较长 TTL，平衡实时性与限流。
- 池子列表首次遇到新代币时可能先显示“待后台刷新”；后台队列会逐步补齐并写入数据库。

## Migration Plan
通过 GORM AutoMigrate 新增 `token_risk_snapshots` 表。旧客户端会忽略新增 JSON 字段；新客户端展示风控提示。部署后已有池子会在列表访问和开仓访问过程中逐步填充快照。
