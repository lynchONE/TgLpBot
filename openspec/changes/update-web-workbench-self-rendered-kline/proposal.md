# Change: 更新 Web Workbench 自绘 K 线

## 背景
- 当前 `webapp/src/App.jsx` 中的 Web Workbench K 线面板依赖第三方 `iframe`，交互受限，而且容易受到嵌入限制或风控策略影响。
- 用户希望在站内稳定查看与当前选中池子联动的 K 线，并叠加类似外部图表中的钱包活动标记。
- 项目已经具备实现一方图表的两个核心基础：
  - 项目已配置 OKX DEX / Market API 凭证，并已有签名请求封装
  - `smart_lp_events` 提供聪明钱 / 监控钱包活动数据
- 当前 `pool_ohlcv` 走的是 GeckoTerminal，上游能力和使用体验都不理想；本次方案优先切换到 OKX 的 `GET /api/v6/dex/market/candles`。

## 变更内容
- 将 Web Workbench 的 K 线面板从第三方 `iframe` 替换为基于 `lightweight-charts` 的自绘蜡烛图 + 成交量图。
- K 线数据源改为 OKX Web3 Market API，而不是 GeckoTerminal。
- 由于 OKX K 线接口是 token 维度而不是 pool 维度，前端会根据当前选中池子推导“展示代币”：
  - 稳定币 / 非稳定币池子默认选非稳定币
  - 双非稳定币池子提供 `token0 / token1` 切换
- 保留 GMGN 外跳入口，不再依赖其嵌入能力。
- 为 Web Workbench K 线面板增加原生控制项：
  - 周期切换
  - 手动刷新
  - 聪明钱覆盖层开关
- 新增一个面向 token 维度的 K 线接口，后端代理 OKX candles。
- 新增一个面向池子维度的聪明钱标记接口，用于在图表上叠加监控钱包的加减仓活动。
- 增加标记聚合与详情面板，避免同一根 K 线上多钱包活动相互遮挡。
- 当缺少聪明钱权限或 ClickHouse 不可用时，K 线仍正常渲染，仅关闭覆盖层。

## 影响范围
- 受影响的 specs：
  - `web-workbench`
- 受影响的代码：
  - `webapp/src/App.jsx`
  - `webapp/src/api.js`
  - `webapp/src/components/KlineChart.jsx`
  - `webapp/src/styles.css`
  - `backend/service/web_server/server.go`
  - `backend/service/web_server/token_candles.go`（新文件）
  - `backend/service/web_server/smart_money_pool_markers.go`（新文件）
  - `openspec/changes/update-web-workbench-self-rendered-kline/*`
- 风险：
  - 标记重叠可能影响可读性
  - 用户频繁切换池子或周期时会增加 OKX 与 ClickHouse 查询压力
  - pool 维度与 token 维度混用时，需要处理展示代币选择逻辑
  - 聪明钱覆盖层的体验受权限状态影响
- 兼容性：
  - 该变更对后端接口是增量式的。
  - Web Workbench 的 K 线模块将从第三方嵌入切换为自绘图表，但继续保留 GMGN 外跳路径。
