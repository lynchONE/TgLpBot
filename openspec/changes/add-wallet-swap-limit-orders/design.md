## Context
- 现有 `wallet_swap_single` 已支持指定 provider 的即时兑换：报价阶段聚合 OKX、0x、LI.FI，执行阶段重新取可执行交易数据。
- 限价单会保存用户授权的未来自动交易意图，并在后台触发真实链上交易，属于资金相关功能，必须显式启用、可取消、可审计。
- 触发条件需要用链上/聚合器报价判断，而不是直接依赖 UI 计算结果。

## Goals / Non-Goals
- Goals:
  - 支持用户设置卖出代币、买入代币、卖出数量、目标价格或目标到账金额。
  - 订单开启后，后台在报价满足条件时自动兑换。
  - 用户可查看订单状态、最近检查结果、成交交易哈希和失败原因。
  - 自动执行必须复用现有钱包、模块权限、provider 交易校验与交易记录链路。
- Non-Goals:
  - 不实现链上订单簿或托管合约，订单只在本服务内后台触发执行。
  - 不保证精确成交价；成交仍受 provider 报价刷新、滑点和链上状态变化影响。
  - 不在本次范围内实现跨链限价单。

## Decisions
- Decision: 使用目标到账金额作为核心触发条件
  - 订单保存 `target_to_amount`，表示同一笔 `from_amount` 在报价中至少需要得到的 `to_token` 数量。
  - UI 可让用户输入“目标价格”，但提交给后端时必须能换算成明确的 `target_to_amount`；后端保存原始 `target_price` 供展示。
  - 这样触发判断只比较 provider quote 的 `net_to_amount` 与订单目标，避免不同 token 小数位和价格方向带来的歧义。

- Decision: 订单状态机显式化
  - 状态包含 `open`、`triggering`、`filled`、`cancelled`、`failed`。
  - worker 触发前必须用数据库条件更新把 `open` 原子切换为 `triggering`，防止多实例重复执行。
  - 成功后写入 `filled`、`tx_hash`、`actual_to_amount`；失败后写入 `failed` 和 `last_error`，不做静默重试。

- Decision: 报价检查使用 provider 偏好
  - 用户可选择 `best`、`okx`、`0x` 或 `li.fi`。
  - `best` 表示在报价检查时选择当前可用且净到账最高的 provider。
  - 指定 provider 时只用该 provider 的报价触发，执行时仍按该 provider 重新获取可执行交易数据。

- Decision: worker 小步轮询并带锁执行
  - 后台 worker 定时加载到期的 `open` 订单，按订单维度串行执行检查。
  - 每次检查写入 `last_checked_at`、`last_quote_provider`、`last_quote_to_amount` 与 `last_error`。
  - worker 的检查间隔、批量大小和最大并发使用配置项控制。

## Risks / Trade-offs
- 远端 provider 报价到执行之间可能漂移，执行阶段必须重新取价，并依赖用户设置的滑点保护。
- 后台自动交易会使用用户已保存的钱包私钥，必须继续沿用现有加密私钥读取路径，且不得记录明文私钥。
- 若用户余额或 allowance 状态在订单创建后变化，触发时可能失败；失败需要明确展示，不用 fallback 假装成功。

## Migration Plan
1. 新增 `WalletSwapLimitOrder` model 并加入 AutoMigrate。
2. 新增限价单 repository/service 和 worker。
3. 新增 `wallet_swap_limit_order` API 与 compat route。
4. 在 `main.go` 启动 worker，并在 shutdown 时停止。
5. 改造 `webapp` 一键兑换页面，新增限价单模式与订单列表。
6. 补充后端单测和 `webapp` 构建验证。

## Open Questions
- 订单失败后是否允许用户手动重开，初版建议通过复制原订单参数创建新订单实现。
