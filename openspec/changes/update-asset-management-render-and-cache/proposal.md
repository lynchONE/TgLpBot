# 变更：优化资产管理模块渲染与缓存性能

## Why
- 当前 `miniapp` 与 `webapp` 的资产管理模块在进入页面时会并发请求多组数据，并在自动轮询时反复进入整页 loading，导致首屏慢、图表重复初始化、页面闪烁明显。
- 资产总览、历史曲线、LP 统计与聪明钱排行榜在短时间内变化有限，允许接受 60 秒内的短期陈旧数据，但当前前后端都没有复用结果，重复请求放大了延迟与渲染开销。
- 资产管理模块还会提前装载管理员子页和相关依赖，增加非资产场景下的包体和执行成本。

## What Changes
- 为用户资产与管理员聪明钱资产相关接口增加 `60s` Redis 短期响应缓存，并支持手动刷新时显式绕过缓存。
- 将 `miniapp` 与 `webapp` 的资产管理页面改为“首次加载 + 后台静默刷新”模式；已有数据时自动轮询不得清空卡片、图表和列表。
- 调整资产页面请求顺序，优先返回并渲染总览数据，再后台补齐历史曲线与 LP 统计。
- 将资产管理模块和管理员子区域改为按需装载；未激活的管理员页签不得提前发起数据请求。
- 本次不调整图表库、头像资源和现有视觉结构。

## Impact
- Affected specs:
  - `asset-management`
  - `admin-smart-money-analytics`
  - `admin-operations-workspace`
- Affected code:
  - `backend/service/web_server/assets.go`
  - `backend/service/web_server/json_response.go`
  - `backend/base/database/redis.go`
  - `miniapp/src/App.jsx`
  - `miniapp/src/components/AssetManagementPage.jsx`
  - `miniapp/src/lib/api.js`
  - `webapp/src/App.jsx`
  - `webapp/src/components/AssetManagementPanel.jsx`
  - `webapp/src/api.js`
