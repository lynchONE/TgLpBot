# Change: Web Workbench K 线按仓位状态区分刷新速率

## Why
当前 Web Workbench 自绘 K 线只有一个轮询刷新间隔。用户盯盘持仓代币时需要较高刷新频率，但浏览没有对应仓位的池子时继续高频请求 OKX candles 会增加 API 调用量，并且对实际体验收益有限。

## What Changes
- Web Workbench K 线刷新设置拆分为两档：
  - 当前 K 线展示代币命中用户现有仓位代币时，使用“有对应仓位”刷新速率。
  - 当前 K 线展示代币没有对应仓位时，使用“无对应仓位”刷新速率。
- K 线自动轮询根据当前选中池子、展示代币和实时仓位数据动态切换刷新间隔。
- 设置面板展示并持久化两档 K 线刷新速率，保留旧 `gmgn_kline` 配置的兼容迁移。

## Impact
- Affected specs: `web-workbench`
- Affected code:
  - `webapp/src/App.jsx`
- 风险：
  - 仓位数据刷新延迟会导致 K 线刷新档位短时间滞后。
  - 需要准确从仓位数据中提取 token 地址，避免把无关池子的 K 线误判为持仓代币。
