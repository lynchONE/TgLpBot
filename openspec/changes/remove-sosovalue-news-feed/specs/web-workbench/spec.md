## ADDED Requirements
### Requirement: WebApp 不得再展示 SoSoValue 新闻
WebApp SHALL 不再请求、渲染或配置 SoSoValue 推荐新闻与底部新闻 ticker。

#### Scenario: 用户打开 WebApp
- **WHEN** 用户打开 WebApp
- **THEN** 页面不请求 `/api/news_feed`
- **AND** 页面不展示 SoSoValue 推荐新闻区域
- **AND** 页面底部不展示新闻 ticker

#### Scenario: 用户打开设置弹层
- **WHEN** 用户打开 WebApp 设置弹层
- **THEN** 设置项中不包含底部新闻速度或 SoSoValue 新闻相关配置

### Requirement: 后端不得再依赖 SoSoValue 新闻服务
后端 SHALL 不再启动 SoSoValue 新闻同步任务，不再暴露 SoSoValue 新闻读取接口，也不再读取 SoSoValue 新闻相关环境变量。

#### Scenario: 后端启动
- **WHEN** 后端进程启动
- **THEN** 后端不启动 SoSoValue 新闻同步任务
- **AND** 不要求配置 `SOSO_VALUE_API_KEY`
- **AND** 不注册 `/api/news_feed` 路由

#### Scenario: 数据库迁移
- **WHEN** 后端执行 AutoMigrate
- **THEN** 后端不再注册 SoSoValue 新闻模型或月度用量模型
- **AND** 既有 SoSoValue 数据库表不再由应用代码读写

