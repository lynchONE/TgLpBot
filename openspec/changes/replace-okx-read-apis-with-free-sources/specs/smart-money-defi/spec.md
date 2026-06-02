## ADDED Requirements

### Requirement: SmartMoney 不再提供 OKX DeFi 仓位功能
系统 MUST 移除依赖 OKX DeFi user asset API 的 SmartMoney DeFi 仓位功能。

#### Scenario: 后端路由清理
- **WHEN** 系统启动 Web API 服务
- **THEN** 后端 MUST NOT 注册 SmartMoney DeFi overview/detail 接口
- **AND** MUST NOT 调用 OKX DeFi user asset platform list/detail API

#### Scenario: 前端入口清理
- **WHEN** 用户打开 SmartMoney 页面
- **THEN** 页面 MUST NOT 展示 DeFi 仓位 tab、卡片或详情入口
- **AND** MUST NOT 请求 SmartMoney DeFi overview/detail API

### Requirement: SmartMoney 非 DeFi 功能必须保留
移除 DeFi 仓位功能 MUST NOT 删除 SmartMoney 钱包监听、LP 仓位、池子详情、跟单和活动流功能。

#### Scenario: 查看 SmartMoney LP 仓位
- **WHEN** 用户查看 SmartMoney 钱包或池子相关信息
- **THEN** 系统 MUST 继续提供非 OKX DeFi 的现有 SmartMoney 功能
