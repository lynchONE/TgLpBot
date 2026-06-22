# Change: 自动跟单配置支持任务名称

## Why
当前自动跟单配置主要通过目标钱包地址、触发规则和执行钱包区分。用户维护多条配置时，很难快速判断某条配置是在监控哪个聪明钱、对应什么策略或备注。

## What Changes
- 自动跟单配置新增可选任务名称字段，用于用户自定义说明。
- 保存配置时校验并规范化任务名称长度，避免空白、过长或不可控内容进入数据库。
- WebApp 与 MiniApp 的自动跟单配置表单增加任务名称输入。
- 自动跟单配置列表、编辑态和相关跟单任务展示优先使用任务名称，未设置时继续按目标钱包/钱包组展示。

## Impact
- Affected specs: `smart-money-follow`, `miniapp-smart-money`
- Affected code:
  - `backend/base/models/smart_money_follow.go`
  - `backend/base/database/mysql.go`
  - `backend/service/smart_money_follow/service.go`
  - `backend/service/web_server/smart_money_auto_follow.go`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `shared/frontend/smartMoneyApi.js`

