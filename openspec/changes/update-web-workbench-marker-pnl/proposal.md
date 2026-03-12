# Change: Web workbench 撤仓气泡展示估算盈亏与撤仓价格

## Why
- Web workbench 当前的聪明钱 K 线气泡只能展示加仓/减仓金额，用户无法直接判断某个钱包这次撤仓相对建仓是盈利还是亏损。
- 撤仓气泡缺少与当前 K 线关联的价格信息，用户需要手动对照图表估算撤仓发生时的价格，使用成本较高。
- `smart_lp_events` 当前未持久化事件时点的 USD 价格快照，因此该能力需要明确采用“按已采集仓位事件回放得到的估算已实现盈亏”，避免误解为审计级收益。

## What Changes
- 扩展 `GET /api/smart_money_pool_markers`，为 `remove` 事件补充基于同钱包同仓位历史 `add/remove` 回放得到的估算成本与估算已实现盈亏字段。
- Web workbench 的 K 线气泡详情面板在减仓事件中展示：
  - 估算成本
  - 估算已实现盈亏（正负金额）
  - 当前图表对应时间点的 K 线价格
- 前端文案需明确这是“估算盈亏”，避免与严格的历史结算收益混淆。

## Scope Assumption
- V1 以单池、单钱包、单仓位维度回放 `smart_lp_events`，仓位键优先使用 `wallet + contract + token_id`；当 `token_id` 缺失时回退到 `wallet + contract + tick range`。
- V1 的盈亏口径为“基于已采集事件和流动性占比回放得到的估算已实现盈亏”，不承诺覆盖 TTL 外或历史漏采事件。
- V1 的“撤仓时价格”取当前图表上与事件时间最近的 candle 价格，不额外新增历史价格服务。

## Impact
- Affected specs:
  - `specs/web-workbench/spec.md`
- Affected code:
  - `backend/service/web_server/smart_money_pool_markers.go`
  - `backend/service/web_server/smart_money_pool_markers_test.go`
  - `webapp/src/App.jsx`
  - `webapp/src/styles.css`
- Risks / tradeoffs:
  - 当仓位的历史建仓事件已过 ClickHouse TTL 或曾经漏采时，减仓事件可能无法得到完整成本，前端需要按“不可用”处理。
  - 估算盈亏依赖仓位事件回放和流动性占比，不等同于交易所逐笔结算收益。
