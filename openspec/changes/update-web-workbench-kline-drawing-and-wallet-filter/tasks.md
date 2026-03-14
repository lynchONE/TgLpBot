## 1. Implementation
- [x] 1.1 在 `webapp/src/App.jsx` 增加 K 线绘图工具状态、钱包筛选状态，以及基于 `klineMarkers` 的候选钱包/过滤 marker 派生逻辑。
- [x] 1.2 在 `webapp/src/App.jsx` 的 K 线工具栏左上角增加绘图工具入口与钱包气泡筛选下拉面板，支持金额阈值、钱包多选、全选、清空、重置。
- [x] 1.3 扩展 `webapp/src/components/KlineChart.jsx`，支持直线测量和矩形测量的草绘、完成、清除与涨跌幅标签显示。
- [x] 1.4 确保筛选后的 marker 仍兼容现有 tooltip、选中钱包高亮、特别关注虚线与详情抽屉逻辑。
- [x] 1.5 调整 `webapp/src/styles.css`，补充工具按钮、筛选下拉、绘图层与测量标签样式。

## 2. Validation
- [x] 2.1 运行 `cd webapp && npm run build`。
- [ ] 2.2 手工验证直线工具与矩形工具都能正确显示涨跌幅读数。
- [ ] 2.3 手工验证金额阈值、钱包勾选、全选、清空、重置对 K 线气泡过滤生效。
