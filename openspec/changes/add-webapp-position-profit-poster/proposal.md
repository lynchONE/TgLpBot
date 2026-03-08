# Change: WebApp 仓位收益海报

## Why
- WebApp 当前可以查看仓位、K 线和未实现盈亏，但缺少一个“一键生成可分享收益图”的能力，用户想晒单或回顾本次开单表现时不够直观。
- 当前仓位卡片已经具备用户信息、投入金额、未实现利润、收益百分比等核心数据，只缺少统一的视觉包装与导出能力。
- 用户希望收益图中同时展示用户头像/名字、开单投入金额、未实现利润、收益百分比，以及币种名称和头像；币种头像可通过 OKX Market Token API 获取。

## What Changes
- 在 `webapp` 的“仓位”模块中，为运行中的任务新增“收益图”入口，点击后生成并预览一张可下载的收益海报。
- 新增后端接口，为指定任务返回生成收益海报所需的聚合数据：
  - 当前仓位快照：交易对、链、任务 ID、开单时间、投入金额、未实现利润、收益百分比。
  - 开单以来的曲线数据：基于 OKX K 线构建“开单以来价格收益曲线”。
  - 主题代币信息：币名、币符号、币头像 URL（优先来自 OKX `token/basic-info`）。
- 海报内容默认包含：
  - 用户头像、用户名字
  - 交易对名称、主题代币头像/名称
  - 本次投入金额
  - 未实现利润（绝对收益）
  - 盈利百分比
  - 开单以来曲线
  - 生成时间 / 链信息

## Scope Assumption
- 本提案默认落地到 `webapp`，入口位于“仓位”卡片。
- “收益曲线”在 V1 中定义为“主题代币自开单时刻到当前的价格收益曲线”；卡片上的“未实现利润 / 盈利百分比”仍使用后端实时仓位计算结果，不伪装成历史逐点真实 LP PnL。

## Impact
- Affected specs (new):
  - `specs/webapp-position-profit-poster/spec.md`
- Affected code (implementation stage):
  - Backend:
    - `backend/service/web_server/position_profit_poster.go`（新增）
    - `backend/service/web_server/server.go`
    - `backend/service/web_server/compat_routes.go`
    - `backend/service/exchange/okx_dex.go`
  - WebApp:
    - `webapp/src/App.jsx`
    - `webapp/src/api.js`
    - `webapp/src/components/PositionProfitPosterModal.jsx`（新增）
    - `webapp/src/styles.css`
- External dependency:
  - OKX Market API
    - `POST /api/v6/dex/market/token/basic-info`
    - 现有 `candles` 行情接口继续复用

## Risks / Tradeoffs
- 仓位的“真实历史未实现盈亏曲线”目前没有快照表，V1 只能准确展示“当前未实现收益摘要”，曲线部分采用“价格收益曲线”作为视觉表达。
- OKX 代币头像或 K 线在个别代币上可能缺失，因此必须提供本地降级方案（首字母头像、摘要海报无曲线模式）。

## Open Questions
1. 默认按当前对话上下文，我将入口放在 `webapp` 的“仓位”卡片；如果你想放到 `miniapp`，我会改提案目标范围。
2. V1 默认提供“预览 + 下载 PNG”；如果你还想要“复制到剪贴板”或“分享至 Telegram”，可以一起纳入实现。
