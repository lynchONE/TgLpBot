# Change: Add Smart Money pool source scope tabs

## Why
当前聪明钱池子视图按所有聪明钱钱包聚合，无法区分“手动添加”和“合约发现”两类来源。用户需要在保留全量视图的同时，快速切换到指定来源范围查看池子列表和对应统计。

## What Changes
- 在聪明钱池子视图的活跃池子列表中增加来源范围切换：全部、手动添加、合约发现。
- `/api/sm/pools` 支持按钱包来源过滤，并在 SQL 聚合前限制来源范围，确保钱包数、仓位数、金额和区间聚合均来自当前范围。
- WebApp 调用池子列表接口时传递当前来源范围，切换 tab 后重置分页并重新加载。

## Impact
- Affected specs: `miniapp-smart-money`
- Affected code:
  - `backend/service/web_server/smart_money.go`
  - `backend/service/smart_money/repository.go`
  - `webapp/src/smartMoneyApi.js`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `webapp/src/styles.css`
