## Context
AutoLP 当前横跨后端扫描服务、策略调度、Telegram Bot、Web API、MiniApp 管理与监控界面，属于跨模块能力。

用户要求不是“关闭默认开关”，而是“彻底去掉自动开单模式以及相关代码”，因此需要明确哪些数据与字段只做运行时下线，哪些接口与代码要直接移除。

## Goals / Non-Goals
- Goals:
  - 完整移除 AutoLP 自动开单能力及所有用户入口
  - 删除“扫描池子后判断条件是否满足再开单”的运行逻辑
  - 保证手动开单、普通仓位查看、SmartLP 等能力可继续工作
- Non-Goals:
  - 不在本次变更中清理历史交易记录或历史 AutoLP 数据
  - 不强制做数据库表删表迁移；若存在历史字段，可在不影响编译与运行的前提下暂时保留

## Decisions
- Decision: 先删除运行时链路与入口，再按编译结果清理剩余依赖。
  - Why: AutoLP 代码分散在多个层级，先切断入口和服务生命周期，可以更快收敛到一个可编译状态。
- Decision: 策略层移除 AutoLP 硬筛回调，不再以池子扫描结果决定再平衡是否允许重开。
  - Why: 这是用户明确要求删除的“扫描池子判断条件是否满足开单”逻辑。
- Decision: MiniApp 删除 Auto 专属监控、盈利曲线、管理员 Auto 控制，以及实时仓位里的 Auto 标签。
  - Why: 前端继续保留这些入口会形成废弃功能残留，且会持续依赖后端已删除接口。

## Risks / Trade-offs
- 删除 AutoLP 服务后，部分历史字段或统计口径可能暂时失去用途，但保留它们能降低一次性数据库兼容风险。
- 若某些管理页或实时接口仍隐式依赖 `is_auto`、`auto_mode_enabled` 等字段，需要在实现时根据编译与页面行为继续裁剪。
- 当前环境未安装 `openspec` CLI，本次无法执行 `openspec validate --strict`，只能按目录与格式约束手工编写。

## Migration Plan
1. 删除 AutoLP 后端服务、Bot 命令、Web API 注册与前端调用。
2. 删除策略层 AutoLP 硬筛回调与再平衡前检查。
3. 运行 `go test ./...` 与 `npm run build`，补清理残留引用。
4. 保留历史 AutoLP 数据表与字段，但确保它们不再驱动任何运行逻辑。

## Open Questions
- 是否需要在本次变更继续移除数据库模型中的 `AutoLPUserConfig`、`AutoLPEvent` 与 `is_auto` 历史字段，取决于编译影响与后续是否还要保留历史展示口径。
