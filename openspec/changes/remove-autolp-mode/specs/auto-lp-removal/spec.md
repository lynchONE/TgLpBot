## ADDED Requirements
### Requirement: 系统不得再提供 AutoLP 自动开单模式
系统 SHALL 不再提供基于池子扫描结果的 AutoLP 自动开单能力，包括自动扫描、候选评估、自动开仓、自动换仓、自动门禁与自动停用等运行链路。

#### Scenario: 后端启动后不再运行 AutoLP 扫描服务
- **WHEN** 后端服务启动
- **THEN** 系统不会启动任何 AutoLP 扫描、候选评估或自动开单任务

#### Scenario: 扫描结果不再触发自动开单
- **WHEN** 某池子满足原有 AutoLP 候选条件
- **THEN** 系统也不会基于该扫描结果自动创建开仓任务

### Requirement: 系统不得再暴露 AutoLP 用户入口
系统 SHALL 不再向用户或管理员暴露 AutoLP 相关命令、页面或接口，包括 Telegram `/auto`、AutoLP 配置接口、AutoLP 监控接口、AutoLP 盈利曲线接口、管理员 AutoLP 统计/关闭接口，以及 MiniApp 中的 AutoLP 专属入口。

#### Scenario: Telegram 不再提供 AutoLP 命令
- **WHEN** 用户查看 Bot 命令或帮助信息
- **THEN** 不会看到 `/auto` 或任何 AutoLP 配置入口

#### Scenario: MiniApp 不再展示 AutoLP 专属界面
- **WHEN** 用户进入 MiniApp
- **THEN** 页面不会展示 AutoLP 监控、Auto 盈利曲线、管理员 Auto 控制或 Auto 专属任务入口

### Requirement: 策略重开逻辑不得依赖 AutoLP 池子硬筛
系统 SHALL 不再在策略再平衡或重开流程中调用 AutoLP 池子硬筛/候选判断结果来决定是否允许开单。

#### Scenario: 再平衡前不再执行 AutoLP 硬筛回调
- **WHEN** 策略任务进入需要重开的流程
- **THEN** 系统不会再调用 AutoLP 硬筛回调来阻止或允许该次重开
