# Change: 管理员授权支持功能模块勾选并重设计页面

## Why
当前管理员授权只支持 MiniApp 总开关、钱包额度和任务额度，无法按热门池、仓位、资产、聪明钱、兑换、创建池子等功能模块精细授权。管理员页面的授权表单也偏堆叠，编辑用户、生成授权码、查看授权范围时需要频繁扫读字段，日常运营容易误操作。

## What Changes
- 扩展用户授权和授权码配置，支持保存“可访问功能模块”清单，并在管理员列表、详情、授权码列表和兑换结果中返回模块授权信息。
- 管理员可在 `webapp` 与 `miniapp` 的授权工作台勾选单个功能模块、按模块分组批量全选/清空，并可一键授权所有功能模块。
- 授权码生成与编辑也支持选择默认功能模块；用户兑换授权码后继承该授权码的模块权限。
- 前端模块入口与关键页面访问 SHALL 根据模块授权过滤；管理员账号仍可访问管理员模块和管理能力。
- 重设计管理员模块页面：以“用户授权 / 授权码 / 公告”工作区为主，采用清晰的列表-详情布局、模块权限矩阵、状态摘要和确认操作，兼顾桌面与移动端易用性。

## Impact
- Affected specs: `admin-access-workbench`
- Affected code:
  - `backend/base/models/user_access.go`
  - `backend/base/models/auth_code.go`
  - `backend/service/user/access.go`
  - `backend/service/web_server/admin_access_workbench.go`
  - `backend/service/web_server/admin_access_workbench_test.go`
  - `backend/service/web_server/access_control.go`
  - `backend/service/web_server/server.go`
  - `webapp/src/api.js`
  - `webapp/src/utils.js`
  - `webapp/src/App.jsx`
  - `webapp/src/components/AdminAccessWorkbench.jsx`
  - `webapp/src/components/AdminPanel.jsx`
  - `webapp/src/styles.css`
  - `miniapp/src/lib/api.js`
  - `miniapp/src/App.jsx`
  - `miniapp/src/components/AdminAccessWorkbench.jsx`
  - `miniapp/src/components/AdminPage.jsx`
  - `miniapp/src/index.css`
