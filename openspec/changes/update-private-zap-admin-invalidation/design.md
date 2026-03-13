## Context
当前 Private Zap 的绑定记录保存在 `wallet_chain_contracts`，Redis 也缓存了按钱包和链解析出的 Zap 地址。旧方案通过“运行时目标版本”和“已绑定版本”比对来决定是否重部署。

这次调整的目标不是改变 Private Zap 的部署/绑定模型，而是把“失效触发方式”从配置版本号切换为管理员主动操作。

## Goals / Non-Goals
- Goals:
  - 提供管理员按链失效现有 Private Zap 绑定的入口
  - 保持用户侧体验不变：下一次开单自动重新部署
  - 简化运行时判断，去掉版本比对
- Non-Goals:
  - 不做链上强制失效
  - 不回收旧合约授权
  - 不删除历史绑定记录

## Decisions
- Decision: 使用“数据库地址置空 + 删除 Redis 缓存”作为失效手段
  - Rationale: 改动最小，不需要迁移表结构，也不影响已有唯一键约束
- Decision: Private Zap 解析不再比较版本号
  - Rationale: 失效由管理员显式触发，运行时只需要判断“有没有有效绑定地址”
- Decision: 管理端按链提供单独按钮
  - Rationale: 管理员的操作意图是链级批量失效，而不是用户级细粒度编辑

## Risks / Trade-offs
- 旧的链上合约不会真正失效，只是后端不再继续使用
- 旧版带版本号的 Redis key 可能遗留，但新逻辑不会再读取它们
- 管理员误触会导致该链用户下一次开单额外支付部署 gas

## Migration Plan
1. 新增管理端按钮和 admin API
2. 修改 Private Zap 解析逻辑，去掉版本比对
3. 失效动作只清空 `contract_address` 等绑定值，不删除记录
4. 保留 `version` 字段以兼容现有表结构，但运行时不再使用它做失效判断

## Open Questions
- 是否需要在管理端显示每条链当前绑定数量和最近一次失效时间？
  - 本次先不做，保持最小改动
