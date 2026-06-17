# Change: 移除 SoSoValue 新闻功能

## Why
WebApp 不再需要 SoSoValue 新闻展示与底部滚动条，继续保留会增加后端第三方 API 配置、同步任务、数据库表和前端布局复杂度。

## What Changes
- 移除后端 SoSoValue 新闻同步服务、`/api/news_feed` 接口、相关配置项和启动/停止逻辑。
- 移除 SoSoValue 新闻数据模型与 AutoMigrate 注册；现有数据库表不再由应用代码读写，物理删表由运维按环境单独执行。
- 移除 WebApp 新闻读取 client、顶部推荐新闻组件、底部新闻 ticker、ticker 速度设置和相关样式。
- 移除 SoSoValue 新闻相关测试与未完成 OpenSpec 变更文档，避免后续误认为该功能仍在规划中。

## Impact
- Affected specs: web-workbench
- Affected code:
  - `backend/base/config/config.go`
  - `backend/base/database/mysql.go`
  - `backend/base/models/soso_value_news.go`
  - `backend/service/web_server/server.go`
  - `backend/service/web_server/soso_value_news.go`
  - `backend/main.go`
  - `webapp/src/App.jsx`
  - `webapp/src/api.js`
  - `webapp/src/components/NewsPanels.jsx`
  - `webapp/src/components/WorkbenchChrome.jsx`
  - `webapp/src/styles/*`

