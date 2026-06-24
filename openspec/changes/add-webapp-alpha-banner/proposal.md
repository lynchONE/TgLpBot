# Change: 首页顶部展示 Alpha 信息

## Why
当前 webapp 首页顶部只承担品牌、登录和工作台配置入口，缺少对当日 Alpha 空投与稳定度状态的即时提示。用户需要在空间有限的顶部居中区域快速看到今日空投项目和稳定度看板摘要，减少跳转外部页面的成本。

## What Changes
- 在 webapp 首页顶部居中区域新增 Alpha 信息条。
- 从 `https://alpha123.uk/api/data?fresh=1` 读取 `airdrops`，展示今日空投的项目名称、数量、积分、Token 与 `date time` 拼接时间。
- 从 `https://alpha123.uk/stability/stability_feed_v3.json` 读取稳定度数据，展示紧凑稳定度摘要，优先呈现异常/不稳定项目。
- 顶部空间有限时只展示摘要与少量条目，避免挤压登录区和工作台布局。
- 为今日空投增加铃铛提醒入口，默认在空投时间前 3 分钟通过 Bark 推送，支持在小弹框内设置提前时间与提醒强度。

## Impact
- Affected specs: `web-workbench`
- Affected code: `webapp/src/App.jsx`、`webapp/src/api.js`、`webapp/src/components/*`、`webapp/src/styles/*`、`webapp/api/alpha.js`、`backend/service/web_server/alpha_proxy.go`、`backend/service/web_server/alpha_reminder.go`、`backend/base/models/lp_config.go`
