# Change: 用户资产盈亏日历与总盈利趋势支持校准

## Why
当前“我的资产”里的盈亏日历按总资产快照差额计算，用户当天转入或转出资产时会把资金划转误计为盈利或亏损。默认快照差额口径应保持不变，但用户需要能在发生充值、提现等外部资金变化时手动校准某一天。

“平仓池子贡献”已有列表和简易进度条，但盈利/亏损贡献不够直观，尤其在池子数量较多时难以快速判断主要来源。

当前“总资产趋势”会随着用户切换钱包而出现明显跳变，无法直观看到策略累计赚了多少钱。用户需要一个独立的“总盈利趋势”，允许指定某个日期的起点盈利，然后按后续每日已校准盈亏累加。

## What Changes
- 用户 LP 统计返回每日快照盈亏、手动校准金额、校准后最终盈亏，并保留转账提示字段。
- 新增用户每日盈亏手动校准接口，允许保存或清除某日修正金额与备注。
- 新增用户总盈利起点接口，允许指定起点日期、起点盈利与备注，并返回累计总盈利曲线。
- 今日盈亏与历史窗口统计统一使用最终展示盈亏口径。
- MiniApp 与 webapp 的主趋势图新增“总资产 / 总盈利”切换；总盈利趋势基于每日最终盈亏累加，并支持起点校准。
- MiniApp 与 webapp 的盈亏日历支持点选日期查看拆解并编辑手动校准。
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
