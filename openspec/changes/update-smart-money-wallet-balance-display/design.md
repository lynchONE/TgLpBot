## Context
- 聪明钱钱包余额的现成口径已经存在于 `assets.Service` 中，包含原生币、稳定币、近 30 天参与 LP 的代币余额以及当前 open LP 估算持仓，并汇总为 `TotalUSD`。
- 当前聪明钱主界面的 `/api/sm/wallets` 和 `/api/sm/stats` 返回结构未包含该余额字段，因此前端无法直接展示。

## Goals / Non-Goals
- Goals:
  - 为聪明钱钱包列表和钱包详情补充统一的钱包余额展示。
  - 避免重新实现一套新的链上余额计算逻辑。
  - 保持 WebApp 和 MiniApp 展示一致。
- Non-Goals:
  - 不调整聪明钱资产服务的余额口径。
  - 不新增新的资产拆分字段展示，仅补充总余额。

## Decisions
- Decision: 复用 `assets.Service` 中聪明钱钱包 live cache 的 `TotalUSD`
  - Why: 该口径已存在、带缓存、且用于聪明钱资产模块，能保证不同页面看到的钱包总额一致。
- Decision: 在聪明钱钱包接口层做 enrichment，而不是把余额持久化到 `monitored_wallets`
  - Why: 钱包余额是动态数据，写入主表会引入额外同步问题；接口层 enrichment 更符合现有缓存模式。

## Risks / Trade-offs
- 风险: 钱包列表分页返回 10 条时，逐个 enrichment 仍会触发缓存读取，首次缓存未命中时可能带来少量额外耗时。
  - Mitigation: 复用现有 live cache，后端优先走 Redis；仅当前页钱包需要 enrichment。
- 风险: 资产服务偶发失败时，余额字段可能为空。
  - Mitigation: 返回可空字段，前端按 `--` 降级展示，不影响钱包页面其他功能。

## Migration Plan
1. 扩展聪明钱钱包列表/详情响应结构，增加 `wallet_balance_usd`。
2. 在后端接口层调用资产服务对当前页钱包做 balance enrichment。
3. WebApp 与 MiniApp 的钱包列表、详情页补充余额展示。
4. 运行后端测试与前端构建验证。

## Open Questions
- 无。
