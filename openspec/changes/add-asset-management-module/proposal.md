# 变更：新增资产管理模块

## Why
- 当前 MiniApp 主要提供实时仓位、热门池子和 Smart Money 视图，但缺少“历史资产变化”和“按时间窗口汇总 LP 盈亏”的资产管理能力。
- 用户侧已经有 `realtime_positions`、`trade_records` 等实时与成交数据，但没有把“历史快照、跨日统计、当天单独展示”整理成稳定的资产视图。
- 管理员侧已经有 Smart Money 钱包、仓位、事件与池子能力，但缺少“钱包资产变化曲线、不同时间维度 LP 统计、每日盈利排行”等经营分析视角。
- MiniApp 现有独立管理页的能力较分散，后续如果继续新增资产与运营分析功能，管理入口会继续碎片化。

## What Changes
- 新增“资产管理”跨端新模块，要求同时在 `miniapp` 与 `webapp` 实现。
- 新增用户侧“我的资产”视图：
  - 展示总资产、钱包余额、LP 持仓价值、未领取手续费等核心指标。
  - 通过图表展示资产变化趋势，默认展示总资产，并支持切换到钱包余额 / LP 持仓 / 手续费。
  - 提供 `近1天 / 近7天 / 近30天` 的 LP 盈亏统计。
  - 历史统计固定不包含当天；当天数据单独展示。
- 新增管理员侧“聪明钱资产”视图：
  - 展示聪明钱钱包资产变化曲线与资产拆分。
  - 统计其 `近1天 / 近7天 / 近30天` 的 LP 参与情况与估算收益。
  - 提供“昨日盈利排行”，并补充收益率榜、参与次数榜。
  - 在钱包详情中展示当日活动、活跃池子数、未匹配事件提示。
- 将 MiniApp 现有管理页能力迁移并整合到新模块的管理员侧：
  - 在线用户
  - 活跃任务
  - 用户详情
  - 系统配置
  - RPC 管理
  - Private Zap 管理
- 双端信息架构统一：
  - MiniApp：原独立 `admin` 入口下的管理能力迁入“资产管理”模块。
  - webapp：新增资产管理工作区/模块，提供与 MiniApp 对齐的用户资产与管理员视图。
- 新增按自然日汇总的快照与统计任务：
  - 每天 `00:05` 后汇总上一自然日数据。
  - 汇总任务支持幂等重跑与指定日期补算。
  - 当天数据由实时查询提供，不并入历史窗口。

## Impact
- Affected specs:
  - `asset-management`
  - `admin-smart-money-analytics`
  - `admin-operations-workspace`
- Affected code:
  - `backend/service/web_server/*`
  - `backend/service/wallet/*`
  - `backend/service/smart_money/*`
  - `backend/service/realtime/*`
  - `miniapp/src/App.jsx`
  - `miniapp/src/components/AdminPage.jsx`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `miniapp/src/components/*`
  - `miniapp/src/lib/api.js`
  - `webapp/src/App.jsx`
  - `webapp/src/components/*`
  - `webapp/src/api.js`
  - `webapp/src/utils.js`
- Affected data:
  - 复用 `trade_records`、`sm_lp_events`、`sm_lp_positions`
  - 新增用户资产日快照 / 用户 LP 日统计 / 聪明钱钱包日快照 / 聪明钱 LP 日统计表

## Confirmed Decisions
- 用户资产主图默认采用 `总资产 = 钱包余额 + LP 持仓估值 + 未领手续费`
- 聪明钱钱包余额变化第一期按“可识别总资产”口径实现
- 时间窗口第一期固定为 `近1天 / 近7天 / 近30天`
- 排行榜默认按“昨日已实现收益额榜”，同时提供“收益率榜 / 参与次数榜”
- 该功能作为全新模块建设，要求 `webapp` 与 `miniapp` 双端落地
- MiniApp 现有独立管理页能力迁移并整合进该模块的管理员侧
