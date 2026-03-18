## ADDED Requirements
### Requirement: Pools 业务必须改由 MySQL 提供
系统 SHALL 将池子列表数据持久化到 MySQL，并由后端 `/api/pools` 及其保留的相关读取逻辑直接从 MySQL 读取，而不再依赖 ClickHouse。

#### Scenario: 读取池子列表
- **WHEN** 客户端调用 `/api/pools`
- **THEN** 后端从 MySQL 读取池子数据并返回结果

#### Scenario: 服务启动后无 ClickHouse 时仍可提供 pools
- **WHEN** 系统启动且环境中不存在任何 ClickHouse 配置
- **THEN** 只要 MySQL 可用，`/api/pools` 仍能正常工作

### Requirement: 系统不得再初始化或依赖 ClickHouse
系统 SHALL 不再包含 ClickHouse 连接初始化、配置项、环境变量、Schema/Migration、依赖包或运行时降级逻辑。

#### Scenario: 后端启动
- **WHEN** 后端进程启动
- **THEN** 启动流程中不会再创建 ClickHouse 连接，也不会再打印 ClickHouse 相关配置或状态日志

#### Scenario: 项目依赖检查
- **WHEN** 开发者检查后端依赖与基础设施代码
- **THEN** 代码库中不再包含 `base/clickhouse` 目录或 `clickhouse-go` 依赖

### Requirement: 系统不得再暴露 ClickHouse 驱动的业务入口
系统 SHALL 删除 Hot Pools、Smart Money、SmartLP、AutoLP/PoolM 及其前后端入口、Bot 命令和代理代码，只保留 Pools 相关能力。

#### Scenario: 后端接口收缩
- **WHEN** 开发者检查路由注册
- **THEN** 代码库中不再注册 `/api/hot_pools`、`/api/smart_money*`、`/api/auto_monitor`、`/api/autolp_*` 等 ClickHouse 驱动接口

#### Scenario: 前端入口收缩
- **WHEN** 用户打开 MiniApp 或 WebApp
- **THEN** 页面中不再展示 Hot Pools、Smart Money、AutoLP/PoolM 相关页面、菜单或数据卡片
