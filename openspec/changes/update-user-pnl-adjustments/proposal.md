# Change: 用户资产盈亏日历支持转账扣除与手动修正

## Why
当前“我的资产”里的盈亏日历按总资产快照差额计算，用户当天转入或转出资产时会把资金划转误计为盈利或亏损。用户需要能看清原始资产变化、自动扣除转账后的盈亏，并在链上转账识别不完整时手动修正某一天。

“平仓池子贡献”已有列表和简易进度条，但盈利/亏损贡献不够直观，尤其在池子数量较多时难以快速判断主要来源。

## What Changes
- 用户 LP 统计返回每日原始资产变化、转账净额、自动修正盈亏、手动修正金额和最终展示盈亏。
- 新增用户每日盈亏手动修正接口，允许保存或清除某日修正金额与备注。
- 今日盈亏与历史窗口统计统一使用最终展示盈亏口径。
- MiniApp 与 webapp 的盈亏日历支持点选日期查看拆解并编辑手动修正。
- MiniApp 与 webapp 的“平仓池子贡献”改为更直观的正负条形图展示，并保留明细。

## Impact
- Affected specs: asset-management
- Affected code:
  - `backend/base/models/asset_management.go`
  - `backend/base/database/mysql.go`
  - `backend/service/assets/user.go`
  - `backend/service/web_server/assets.go`
  - `backend/service/web_server/server.go`
  - `backend/service/web_server/compat_routes.go`
  - `webapp/src/api.js`
  - `webapp/src/components/AssetManagementPanel.jsx`
  - `webapp/src/styles.css`
  - `miniapp/src/lib/api.js`
  - `miniapp/src/components/AssetManagementPage.jsx`
  - `miniapp/api/positions.js`
