# Change: 为聪明钱钱包增加自定义头像上传与绑定能力

## Why
- 当前聪明钱钱包头像完全依赖项目内置随机图标，无法体现钱包自己的识别信息，用户也不能手动维护。
- WebApp 和 MiniApp 都已经有钱包管理与详情视图，补充自定义头像能力后，能够让同一个钱包在列表、详情、跟踪场景里保持一致的视觉标识。
- 头像文件需要落到服务端统一管理，而不是继续放在前端静态资源里；项目已经具备服务器环境，适合接入已部署好的 MinIO `avatar` bucket。

## What Changes
- Backend:
  - 为聪明钱钱包模型增加头像字段，用于持久化钱包绑定的头像地址或对象键。
  - 新增 MinIO 连接配置项，用于连接对象存储并将钱包头像上传到 `avatar` bucket。
  - 为聪明钱钱包增加头像上传接口和头像更新能力，校验上传文件类型、大小，并把上传结果绑定到指定钱包。
- WebApp:
  - 在聪明钱钱包编辑入口支持上传头像、预览已绑定头像，并在保存后刷新展示。
  - 钱包列表、详情等所有使用钱包头像的位置优先展示自定义头像，无自定义头像时继续回退到现有随机图标。
- MiniApp:
  - 与 WebApp 保持相同能力，支持上传头像、预览、保存和统一展示。

## Impact
- Affected specs:
  - `smart-money-wallet-avatar`
- Affected code:
  - `backend/base/config/*`
  - `backend/base/models/smart_money.go`
  - `backend/base/database/*`
  - `backend/service/web_server/smart_money.go`
  - `backend/service/smart_money/*`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `webapp/src/smartMoneyApi.js`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `miniapp/src/lib/smartMoneyApi.js`
