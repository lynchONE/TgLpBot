## Context
- 热门池子查询的核心事实数据来自 ClickHouse `poolm_top_fees_realtime`，本身已经有 10 秒级响应缓存。
- OKX `token/basic-info` 已经在项目内封装为批量接口，可按 `chainIndex + tokenContractAddress[]` 一次查询多个代币的 symbol、name 与 logo。
- 代币 logo 属于低频变更元数据，和热门池子的时序分析数据更新频率完全不同。

## Goals / Non-Goals
- Goals:
  - 让热门池子接口返回可直接渲染的主题代币图标信息。
  - 避免每次热门池子请求都重复访问 OKX。
  - 保留现有 ClickHouse 热门池子查询与 Redis 整包缓存路径，尽量减少事实查询层改动。
- Non-Goals:
  - 不把代币 logo 等元数据冗余写入 ClickHouse 热门池子事实表。
  - 不在本次改动中统一改造 Smart Money、Mini App 等其它列表展示。
  - 不引入新的外部存储组件。

## Decisions
- Decision: 代币元数据采用 MySQL 维表 + Redis 热缓存
  - MySQL 作为持久化存储，保证 Redis 丢失后仍可恢复。
  - Redis 作为热缓存，避免热门池子冷启动后每次都回查数据库。
- Decision: 热门池子接口采用 Go 层 enrich，而不是 ClickHouse JOIN
  - 热门池子的事实查询继续只查 ClickHouse。
  - 取回池子列表后，在 Go 层批量解析展示代币并补齐元数据。
  - 这样可以保持 ClickHouse 查询简单稳定，也方便后续替换 OKX 或加入人工修正源。
- Decision: 展示代币选择规则优先挑选“非 base-like”代币
  - 结合链配置中的稳定币地址、wrapped native 地址以及交易对 symbol 判断。
  - 若池子一侧是稳定币或 wrapped native，优先展示另一侧代币。
  - 若两侧都无法明确判定，则回退到 `token0`。
- Decision: OKX 查询使用批量拉取并带负缓存
  - 对同一批热门池子中重复出现的代币地址去重后只请求一次。
  - 对 OKX 查不到的代币写入短 TTL 负缓存，避免反复打空请求。

## Risks / Trade-offs
- OKX logo 可能偶发失效或返回空值。
  - Mitigation: 前端仍保留 DEX 图标与首字母 fallback。
- MySQL 维表会引入一次额外读写路径。
  - Mitigation: 通过 Redis 命中优先、批量查询与整包响应缓存将额外开销控制在首次 miss。
- “非稳定代币”判断可能在双 Meme 或双主币池子中不完全符合预期。
  - Mitigation: 对这类无法明确判定的池子回退到 `token0`，保证行为稳定可解释。

## Migration Plan
1. 新增 `token_metadata` MySQL 模型并加入自动迁移。
2. 新增代币元数据服务，打通 Redis / MySQL / OKX 三层查询链路。
3. 在热门池子接口中补充 `display_token_*` 字段。
4. 调整 `webapp` 热门池子卡片展示顺序与样式。

## Open Questions
- 暂无，本次先将 OKX 作为唯一元数据来源，后续若出现缺失再补充手工兜底或二级来源。
