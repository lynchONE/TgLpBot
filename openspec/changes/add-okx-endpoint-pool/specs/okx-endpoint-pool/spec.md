## ADDED Requirements

### Requirement: OKX API 配置池
系统 MUST 支持通过 MySQL 管理多个 OKX API 配置。每个配置至少 MUST 包含名称、base URL、API key、secret、passphrase、启用状态、当前状态和健康检查信息。

secret 和 passphrase MUST 加密存储，管理接口和 UI MUST 默认只返回脱敏后的敏感字段。

#### Scenario: 管理员新增 OKX API 配置
- **GIVEN** 管理员已通过 Telegram WebApp `initData` 认证
- **WHEN** 管理员提交名称、base URL、API key、secret 和 passphrase
- **THEN** 系统 MUST 持久化该配置
- **AND** secret 与 passphrase MUST 以加密形式保存
- **AND** 响应 MUST 返回更新后的配置列表且不包含 secret/passphrase 明文

### Requirement: OKX 当前配置选择
系统 MUST 在 OKX 配置池中维护最多一个 current 配置。存在可用 DB 配置时，OKX 请求 MUST 优先使用 current 配置；没有 current 时 MUST 自动选择第一个可用配置并设为 current。

#### Scenario: 管理员切换 current 配置
- **GIVEN** OKX 配置池中存在多个启用且可用的配置
- **WHEN** 管理员将其中一个配置切换为 current
- **THEN** 后续 OKX 请求 MUST 使用该配置签名并发送请求
- **AND** 其他 OKX 配置 MUST 不再处于 current 状态

### Requirement: OKX 配置不可用时自动切换
当 current OKX 配置不可用时，系统 MUST 自动禁用或临时跳过该配置，并切换到下一个启用且可用的配置。

不可用信号至少 MUST 包括网络错误、HTTP 5xx、HTTP 401/403、HTTP 429、OKX 返回非 `0` code 且错误表示认证、限流或额度问题。

#### Scenario: 当前配置连续失败后切换
- **GIVEN** current OKX 配置连续请求失败达到系统阈值
- **AND** 配置池中存在另一个启用且可用的配置
- **WHEN** 后续 OKX 请求需要发送
- **THEN** 系统 MUST 选择另一个可用配置执行请求
- **AND** 原 current 配置 MUST 记录最后错误、连续失败次数和禁用原因

### Requirement: OKX 额度耗尽禁用到下月
当系统识别到 OKX 配置额度耗尽或明确的 quota/monthly limit 错误时，系统 MUST 将该配置禁用到下个月起始时间，且在禁用期间 MUST NOT 自动选择该配置。

#### Scenario: OKX 配置额度耗尽
- **GIVEN** current OKX 配置返回额度耗尽错误
- **WHEN** 系统记录该失败
- **THEN** 该配置 MUST 被标记为不可用直到下个月起始时间
- **AND** 系统 MUST 尝试切换到另一个可用配置

### Requirement: OKX `.env` 兼容兜底
当 OKX 配置池为空、DB 未初始化或没有可用 DB 配置时，系统 MUST 继续使用现有 `.env` 中的 `OKX_DEX_API_URL`、`OKX_API_KEY`、`OKX_SECRET_KEY` 和 `OKX_PASSPHRASE`。

系统 MUST NOT 在没有可用 OKX 配置时返回伪造的成功结果或安全默认值。

#### Scenario: DB 配置池为空
- **GIVEN** `okx_api_configs` 表中没有任何启用配置
- **WHEN** 系统执行 OKX `/swap` 或 `/approve-transaction` 请求
- **THEN** 系统 MUST 使用 `.env` OKX 配置
- **AND** 若 `.env` 配置也不可用，请求 MUST 返回明确错误

### Requirement: 管理员 OKX 配置池管理
系统 MUST 提供管理员专用 API 和 MiniApp 管理入口，用于查看、新增、重命名、切换、启用、禁用、删除和检查 OKX API 配置。

非管理员 MUST NOT 查看或修改 OKX 配置池。

#### Scenario: 非管理员访问 OKX 配置池
- **WHEN** 非管理员请求 OKX 配置池管理接口
- **THEN** 系统 MUST 返回 forbidden
- **AND** 系统 MUST NOT 返回任何 OKX API key、secret 或 passphrase 信息

#### Scenario: 管理员检查 OKX 配置
- **GIVEN** 管理员已创建一个 OKX API 配置
- **WHEN** 管理员触发该配置的连通性检查
- **THEN** 系统 MUST 使用该配置执行低风险 OKX 请求
- **AND** 系统 MUST 更新 `last_checked_at`、`last_success_at`、`last_latency_ms` 或 `last_error`
