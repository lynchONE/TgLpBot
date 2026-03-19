# Change: 增强 Web Workbench K 线气泡钱包操作

## Why
- 当前 Web Workbench 的 K 线气泡 tooltip 只能查看钱包标签、复制地址和切换特别关注，缺少“在当前上下文里直接改标签”的快捷操作，用户需要跳去 Smart Money 管理页才能完成命名，操作链路过长。
- 当前“特别关注”钱包会在图上叠加该钱包窗口内的全部蓝色开仓线；当同一钱包在当前池子多次开仓时，图面会出现多条历史虚线，干扰用户只看最新跟踪区间的需求。

## What Changes
- 为 K 线气泡 tooltip 增加钱包标签快捷编辑能力：
  - 支持在 tooltip 内进入编辑态、修改、保存和取消。
  - 对于已存在的 Smart Money 钱包记录，直接更新其 `label`。
  - 对于尚未保存过标签的钱包，后端按地址保存标签，但默认不自动开启监控。
- 调整特别关注钱包的蓝色开仓线口径：
  - 每个特别关注钱包在当前池子和当前数据窗口内，只展示最近一次 `add` 事件对应的蓝色开仓线。
  - 不再同时渲染该钱包更早的历史蓝线。
- 保持现有 tooltip、气泡点击、空白关闭和特别关注切换行为兼容。

## Impact
- Affected specs:
  - `web-workbench`
- Affected code:
  - `webapp/src/App.jsx`
  - `webapp/src/components/KlineChart.jsx`
  - `webapp/src/smartMoneyApi.js`
  - `webapp/src/styles.css`
  - `backend/service/web_server/smart_money.go`
  - `backend/service/smart_money/repository.go`
- 风险与取舍：
  - 若直接复用 `monitored_wallets` 保存标签，需要确保“仅保存标签”不会隐式把钱包加入活跃监控集合。
  - “最近一次开仓线”基于当前已加载 marker 数据窗口计算；若窗口外存在更新开仓事件，则线条显示仍以后端返回窗口为准。
