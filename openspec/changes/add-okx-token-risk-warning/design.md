## Context
OKX advanced-info 接口可返回 `riskControlLevel` 与 `tokenTags`。本次风险关注点是貔貅盘、低流动性与风险等级，且查询对象仅限池子中的非稳定/非主流原生包装代币，避免对 USDT/USDC/WBNB/BNB 等基础资产产生噪音。

## Goals / Non-Goals
- Goals: 池子列表可扫读风险等级；开单前必须看到代币风控信息；貔貅盘禁止执行真实开仓。
- Goals: OKX 查询失败时暴露“风控查询失败”状态，不用安全默认值掩盖风险。
- Non-Goals: 不新增数据库 schema；不把所有 OKX 原始字段完整透传给前端。

## Decisions
- Decision: 后端统一查询并聚合风险信息，前端只渲染结构化结果。
- Decision: 使用短期内存缓存降低 OKX 请求频率；失败结果使用更短 TTL，并在响应中展示错误提示。
- Decision: 风控等级与标签作为开仓检查项返回；貔貅盘为 fail，低流动性或中高以上等级为 warn。

## Risks / Trade-offs
- OKX 接口异常会影响风险信息完整性；响应会明确标注查询失败，避免误判为安全。
- 池子列表批量查询会增加外部 API 调用；通过去重、并发限制和缓存控制耗时。

## Migration Plan
无需数据库迁移。部署后旧客户端会忽略新增 JSON 字段，新客户端展示风险提示。
