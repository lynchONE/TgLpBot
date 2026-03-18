## Context
当前 ClickHouse 是 `/api/pools`、Hot Pools、Smart Money、SmartLP、AutoLP/PoolM 的共同分析底座，但用户明确要求仅保留池子列表相关业务，其余能力全部下线。

仓库中还存在一个未提交的 `remove-autolp-mode` 提案，但该提案只覆盖 AutoLP；本次需求范围更大，既要移除 AutoLP，也要移除 Hot Pools、Smart Money、SmartLP 以及 ClickHouse 本身。

## Goals / Non-Goals
- Goals:
  - 将 `pools` 数据改为持久化到 MySQL，并由保留的池子业务读取 MySQL
  - 删除 ClickHouse 初始化、配置、依赖和所有运行时分支
  - 删除所有基于 ClickHouse 的后端服务、Web API、Bot 入口和前端模块
  - 保证项目在删除后仍可编译、可启动，且 `/api/pools` 继续可用
- Non-Goals:
  - 不为 Hot Pools、Smart Money、AutoLP/PoolM 提供 MySQL 替代实现
  - 不保留“未配置 ClickHouse 时降级”的兼容逻辑
  - 不在本次处理与 ClickHouse 无关的功能重构

## Decisions
- Decision: `pools` 采用 MySQL 主库直接承载，不再保留分析库抽象层。
  - Why: 用户只保留这一条业务，直接落主库是实现与维护成本最低的方案。
- Decision: `pools` 的写入来源复用现有外部抓取逻辑，并按当前 ClickHouse `pools` 表字段设计等价的 MySQL 新表。
  - Why: 现有抓取链路已经能产出池子数据，复用它可以降低改造成本，同时保持现有字段口径不变。
- Decision: 所有 ClickHouse 相关能力直接删除，不保留空实现、兼容路由或 feature flag。
  - Why: 需求是彻底移除，而不是继续维护可选依赖。
- Decision: 前端按“删除入口 + 删除页面/组件 + 删除 API client/proxy”方式同步收缩。
  - Why: 若只删除后端接口而保留前端入口，会持续制造死链接和构建依赖。
- Decision: 本提案覆盖 `remove-autolp-mode` 中 AutoLP 下线范围。
  - Why: AutoLP 本身依赖 ClickHouse/PoolM，本次统一清理能避免两次大规模改动互相冲突。

## Risks / Trade-offs
- 删除 ClickHouse 后，Hot Pools、Smart Money、SmartLP、AutoLP/PoolM 的历史分析能力全部失效，无法通过简单开关恢复。
- 复用现有外部抓取逻辑时，需要把它与 AutoLP/PoolM 其他已删除能力解耦，只保留池子抓取与 MySQL 落库部分。
- 当前工作区已有未提交修改，且部分文件与本提案高频变更路径重叠；实施时需要避免覆盖现有编辑。
- 当前环境缺少 `openspec` CLI，本次只能手工维护提案文件，无法本地执行 `openspec validate --strict`。

## Migration Plan
1. 新增 MySQL `pools` 模型与迁移，字段参考当前 ClickHouse `pools` 表。
2. 从现有外部抓取逻辑中拆出池子抓取与保存流程，改为写入 MySQL `pools` 新表。
3. 将 `/api/pools` 与仍保留的池子读取逻辑改为 MySQL。
4. 删除 ClickHouse 初始化、配置、依赖和 `base/clickhouse` 代码。
5. 删除 Hot Pools、Smart Money、SmartLP、AutoLP/PoolM 的后端服务、路由、Bot 入口与前端模块。
6. 运行 Go 测试和前端构建，补齐残留引用。

## Open Questions
- 是否同步清理 MySQL 中仅服务于已删除业务的历史模型/配置表，可在实现阶段根据编译影响再决定。
