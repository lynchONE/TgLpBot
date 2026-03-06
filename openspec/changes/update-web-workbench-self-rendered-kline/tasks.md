## 1. 实现
- [x] 1.1 为 Web Workbench 新增基于 OKX 的 `token_candles` 接口。
- [x] 1.2 为 Web Workbench 新增池子维度的聪明钱 marker API。
- [x] 1.3 将 `webapp` 的 K 线 `iframe` 替换为基于 `lightweight-charts` 的原生渲染。
- [x] 1.4 实现“池子 -> 展示代币”的推导与 `token0 / token1` 切换规则。
- [x] 1.5 为 K 线面板增加周期切换、刷新和覆盖层开关。
- [x] 1.6 在 `webapp/` 中实现 marker 聚合和活动详情抽屉。
- [x] 1.7 保留 K 线面板中的 GMGN 外跳入口。

## 2. 验证
- [x] 2.1 在 `webapp/` 中运行 `npm run build`。
- [x] 2.2 验证切换池子或展示代币时，OKX K 线会同步更新。
- [x] 2.3 验证切换池子时图表和聪明钱 marker 会同步更新。
- [x] 2.4 验证缺少聪明钱权限时会自动降级为纯 K 线模式。
