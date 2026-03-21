## ADDED Requirements

### Requirement: 管理员操作能力迁移到资产管理模块
系统 SHALL 将 MiniApp 现有独立管理页中的管理员操作能力迁移并整合到“资产管理”模块的管理员侧。

管理员操作能力 MUST 至少包括：
- 在线用户
- 活跃任务
- 用户详情
- 系统配置
- RPC 管理
- Private Zap 管理

#### Scenario: 管理员在 MiniApp 进入运行管理
- **WHEN** 管理员进入 MiniApp 的“资产管理 > 运行管理”
- **THEN** 系统提供在线用户、活跃任务与用户详情能力

#### Scenario: 管理员在 MiniApp 进入系统管理
- **WHEN** 管理员进入 MiniApp 的“资产管理 > 系统管理”
- **THEN** 系统提供系统配置、RPC 管理与 Private Zap 管理能力

### Requirement: webapp 提供管理员操作工作区
系统 SHALL 在 webapp 的资产管理模块中提供管理员操作工作区，并覆盖与 MiniApp 对应的核心管理能力。

#### Scenario: 管理员在 webapp 进入运行管理
- **WHEN** 管理员在 webapp 打开资产管理模块并切换到“运行管理”
- **THEN** 系统展示在线用户、活跃任务和用户详情视图

#### Scenario: 管理员在 webapp 进入系统管理
- **WHEN** 管理员在 webapp 打开资产管理模块并切换到“系统管理”
- **THEN** 系统展示系统配置、RPC 管理和 Private Zap 管理视图

### Requirement: 旧管理入口迁移后保持能力兼容
系统 SHALL 在迁移 MiniApp 旧管理入口后，保持原有管理员操作能力与权限校验不变。

#### Scenario: 迁移后访问在线用户
- **WHEN** 管理员通过新模块访问“在线用户”
- **THEN** 系统返回与迁移前相同口径的在线用户数据

#### Scenario: 迁移后访问 RPC 管理
- **WHEN** 管理员通过新模块访问“RPC 管理”
- **THEN** 系统复用或兼容现有 RPC 管理接口与权限校验
