# Change: Web workbench 热门池子支持按头像筛选同代币池子

## Why
- Web workbench 的热门池子列表当前只能按排序和关键字筛选，用户看到某个代币后，无法一键查看该非稳定币对应的其他池子。
- 热门池子卡片左侧头像天然代表该池子的核心代币，点击头像按同代币展开相关池子，能显著降低手动搜索成本。
- 仅在前端筛当前已加载的前 60 条热门池子不满足“所有池子数据”的诉求，因此需要后端 `hot_pools` 支持按代币地址筛选。

## What Changes
- Web workbench 热门池子卡片左侧头像新增点击行为：
  - 点击后按该池可识别出的“非稳定币代币地址”筛选热门池子列表
  - 再次点击同一头像时取消筛选
- 热门池子面板新增当前代币筛选状态展示与清除入口。
- 扩展 `GET /api/hot_pools`，支持按代币地址筛选返回包含该代币的池子数据，而不是仅返回默认热门榜单。

## Scope Assumption
- V1 仅在“能明确识别出唯一非稳定币”的池子上启用该交互。
- 当池子两边都是稳定币，或无法从地址/符号可靠推断唯一非稳定币时，头像点击不触发筛选。
- “所有池子数据”定义为当前链、当前排序、当前 timeframe 下，包含该代币地址的池子集合；不跨链，不改变排序规则。

## Impact
- Affected specs:
  - `specs/web-workbench/spec.md`
- Affected code:
  - `backend/service/web_server/hot_pools.go`
  - `webapp/src/api.js`
  - `webapp/src/App.jsx`
  - `webapp/src/utils.js`
  - `webapp/src/styles.css`
- Risks / tradeoffs:
  - 某些双非稳定币池子难以唯一判断“主代币”，V1 选择不启用头像筛选，避免错误筛选。
  - 代币筛选会放大热门池子接口返回规模，需要继续保留 `limit` 上限控制。
