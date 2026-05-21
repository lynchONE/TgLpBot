# Change: WebApp 首页接入 SoSoValue 新闻与底部滚动条

## Why
当前 WebApp 首页顶部存在空白展示区域，用户希望接入 SoSoValue 推荐新闻；同时希望在页面最底部增加类似 SoSoValue 的新闻 ticker 滚动条，方便在工作台内持续关注市场资讯。

## What Changes
- 新增 SoSoValue API Key 与新闻同步配置，密钥只在后端使用。
- 新增后端新闻同步服务：
  - 拉取 SoSoValue 推荐新闻 feed。
  - 拉取或复用 SoSoValue 新闻 feed 生成底部 ticker 内容。
  - 将新闻数据写入数据库。
  - 仅保留 24 小时内新闻，超过 24 小时的数据自动清理。
- 增加请求配额保护：
  - 默认推荐新闻和 ticker 各 60 秒请求一次。
  - 在 31 天月份内约 89,280 次请求，低于每月 100,000 次限制。
  - 数据库存储本月已用请求量；达到安全阈值时停止继续请求第三方 API，并继续返回库内已有数据。
- 新增 WebApp 新闻读取接口，前端只读取本地数据库缓存。
- 在 WebApp 首页顶部标注区域展示推荐新闻。
- 在 WebApp 页面最底部展示 ticker 横向滚动条。
- 当配置缺失、额度耗尽或第三方请求失败时，前端显示明确状态，不使用假新闻或静默默认内容。

## Impact
- Affected specs: `web-workbench`
- Affected code:
  - `backend/base/config/config.go`
  - `backend/base/models/*`
  - `backend/base/database/mysql.go`
  - `backend/service/web_server/*`
  - `webapp/src/*`
