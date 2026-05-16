# Change: 聪明钱池子收益火焰图

## Why
当前聪明钱「池子视图」主要展示活跃池子列表，信息完整但不够聚焦，用户难以第一时间判断哪个池子更适合跟单开仓。

需要新增一个更直观的池子机会视图，从「聪明钱吃到多少手续费」和「单位资金、单位时间吃手续费有多快」两个角度突出机会池子。

## What Changes
- 在聪明钱「池子视图」下新增二级 Tab：
  - `活跃池子`：保留当前池子列表、筛选、分页、详情和快捷跟单能力。
  - `收益火焰图`：新增按池子聚合的收益热力视图。
- 新增后端接口返回池子收益火焰图数据，支持固定时间窗口：`30s`、`1m`、`5m`、`1h`。
- 火焰图支持两种排序/展示口径：
  - `手续费`：按池子下聪明钱当前未领取手续费 USD 总和排序。
  - `速率`：按「当前手续费 / 仓位金额 / 开仓至今时间」实时计算平均速率，并折算到选定窗口，突出单位资金在单位时间内吃手续费更快的池子。
- 火焰图卡片必须支持快捷跟单，复用当前聪明钱池子开仓入口。
- MiniApp 与 WebApp 的聪明钱池子视图口径保持一致。

## Impact
- Affected specs: `miniapp-smart-money`
- Affected code:
  - `backend/service/web_server/smart_money.go`
  - `backend/service/smart_money/repository.go`
  - `backend/base/models/smart_money.go`
  - `miniapp/src/components/SmartMoneyPage.jsx`
  - `miniapp/src/lib/smartMoneyApi.js`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `webapp/src/smartMoneyApi.js`
  - `miniapp/api/sm.js`
  - `webapp/api/sm.js`
