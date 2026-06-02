## Context
现有 OKX 请求集中在 `backend/service/exchange/okx_dex.go`：
- `/swap` 用于钱包兑换、入仓/退仓 swap、entry swap preview。
- `/approve-transaction` 用于获取 approve spender。
- `market/token/advanced-info` 用于 token 风控。

现有 RPC 池已经提供了可参考模式：DB 优先、`.env` 兜底、current 选择、健康状态、额度耗尽禁用到下月、后台检查与管理员 UI。池子数据源则提供了管理员管理上游配置的 API/UI 模式。

## Goals / Non-Goals
- Goals:
  - 让管理员运行时增加和切换 OKX API 配置。
  - 当前配置不可用时自动切到同池可用配置。
  - 保持 DB 空池时读取 `.env` 的兼容行为。
  - 密钥加密存储、响应和日志脱敏。
- Non-Goals:
  - 不替换 OKX DEX swap/approve/advanced-info 的业务语义。
  - 不新增 0x/LI.FI 等 provider 的池化。
  - 不改变链上 OKX router / tokenApprove 合约白名单逻辑。

## Decisions
- Decision: 新增 `okx_api_configs` 表作为池化单元。
  - 字段建议：`name`、`base_url`、`api_key`、`secret_key_encrypted`、`passphrase_encrypted`、`is_current`、`is_enabled`、`disabled_until`、`disabled_reason`、`consecutive_failures`、`last_checked_at`、`last_success_at`、`last_latency_ms`、`last_error`。
  - `api_key` 可展示脱敏值；`secret_key` 和 `passphrase` 只加密存储。
- Decision: OKX 配置池不按链拆分。
  - OKX DEX API key/base URL 当前是全局配置，具体链通过 `chainIndex/chainId` query 参数区分。
  - 后续如果 OKX 账号按链拆分，可在表上增 `scope`/`chain` 字段扩展。
- Decision: `.env` 作为最后兜底来源，不写入 DB。
  - DB 为空或不可用时返回 env effective config。
  - DB 中有可用配置时优先使用 DB。
- Decision: 失败记录由 OKX client wrapper 调用 manager 完成。
  - 每次请求绑定实际使用的配置 ID。
  - 成功时清空连续失败和临时禁用状态。
  - 失败时记录错误摘要；遇到可切换错误时禁用当前配置并确保重新选主。
- Decision: 连通性检查使用低风险读接口。
  - 优先调用 `/approve-transaction`，使用 BSC 或已启用链的稳定币地址，`approveAmount=1`。
  - 如果缺少链配置或 token 地址，则只做签名请求到一个明确的 OKX DEX endpoint 并要求 HTTP/业务 code 正常。

## Risks / Trade-offs
- API key 失败可能是账号级别问题，自动切换能降低影响，但不能保证所有 key 都可用。
  - Mitigation: 管理页展示错误摘要和最后成功时间，允许管理员手动禁用或切换。
- 健康检查会消耗 OKX 调用额度。
  - Mitigation: 默认较低频率，例如 10 到 30 分钟；管理员手动检查即时触发。
- `.env` 兜底如果也失效，系统仍会返回真实错误。
  - Mitigation: 不用安全默认值掩盖错误，调用方看到明确的 OKX 错误。

## Migration Plan
1. AutoMigrate 新增 `okx_api_configs`。
2. 启动时不自动迁移 `.env` 到 DB，避免无意持久化密钥。
3. 管理员可通过 UI 手动添加 DB 配置。
4. DB 配置为空时服务继续使用 `.env`。
5. 回滚时删除/忽略 DB 配置即可回到 `.env` 行为。

## Open Questions
- 管理页是否需要支持导入当前 `.env` 配置为 DB 记录？默认不做，避免把生产密钥意外持久化到管理面。
- 健康检查默认间隔取 10 分钟还是 30 分钟？实现时可用常量或环境变量控制。
