## ADDED Requirements

### Requirement: 按池子从 RPC 发现大额加池钱包
系统 SHALL 提供基于池子标识的大额加池钱包候选预览能力。用户 MUST 能输入链、池子地址或 V4 poolId、最低加池 USD 金额、时间窗口和返回数量，系统 MUST 通过项目配置的 RPC 池查询链上 LP 事件并返回符合条件的钱包候选及对应交易证据。

#### Scenario: 用户按 V3 池子筛选候选钱包
- **WHEN** 用户输入 BSC V3 池子地址、最低加池金额、时间窗口和数量限制后请求预览
- **THEN** 系统 MUST 使用 RPC 查询该池子对应 PositionManager 的 V3 加 LP 事件
- **AND** 系统 MUST 仅返回属于该池子且 USD 金额大于等于阈值的候选钱包
- **AND** 每个候选 MUST 包含钱包地址、加池金额、交易时间、交易哈希、池子地址、协议、交易对、区间、金额来源和数据源
- **AND** 系统 MUST 标记该钱包是否已在聪明钱监控列表中

#### Scenario: 用户按 V4 poolId 筛选候选钱包
- **WHEN** 用户输入 BSC V4 poolId、最低加池金额、时间窗口和数量限制后请求预览
- **THEN** 系统 MUST 使用 RPC 查询配置的 V4 PoolManager `ModifyLiquidity` 事件
- **AND** 系统 MUST 仅处理目标 poolId 的加 LP 事件
- **AND** 系统 MUST 复用交易 receipt 中的 ERC20 transfer 解析补齐加池金额
- **AND** 系统 MUST 仅返回可验证钱包归因且 USD 金额大于等于阈值的候选钱包

#### Scenario: RPC 不可用
- **WHEN** 用户请求池子雷达预览但当前链 RPC 不可用，或 V4 请求缺少必要的 PoolManager 配置
- **THEN** 系统 MUST 返回明确配置错误或 RPC 错误
- **AND** 系统 MUST NOT 返回伪成功空结果

#### Scenario: 不再支持按代币地址直接筛选
- **WHEN** 用户只提供 token address 而未提供池子地址或 V4 poolId
- **THEN** 系统 MUST 拒绝请求并提示需要池子标识
- **AND** 系统 MUST NOT 自动按 token address 扫描所有相关池子

#### Scenario: 事件无法可靠归因
- **WHEN** 某条 V3 或 V4 加池事件无法解析出可验证的 LP owner 钱包
- **THEN** 系统 MUST 排除该事件并记录排除原因
- **AND** 系统 MUST NOT 使用空地址、默认地址或未经验证的 `tx.from` 作为候选钱包

#### Scenario: 加池金额无法计算
- **WHEN** 某条加池事件无法得到 token 金额、token decimals 或可信 USD 价格
- **THEN** 系统 MUST 排除该事件并记录排除原因或返回明确错误
- **AND** 系统 MUST NOT 使用 0、默认价格或默认金额替代真实 USD 金额

### Requirement: 池子雷达候选钱包批量导入聪明钱
系统 SHALL 允许用户将池子雷达预览后确认的钱包批量导入聪明钱监控列表。导入 MUST 写入现有 `monitored_wallets` 监控体系，并保留来源池子标识。

#### Scenario: 用户确认导入池子雷达候选钱包
- **WHEN** 用户从池子雷达预览结果中选择一个或多个钱包并提交导入
- **THEN** 系统 MUST 校验每个钱包地址和来源池子标识
- **AND** 新钱包 MUST 创建为启用状态的聪明钱监控钱包
- **AND** 钱包来源 MUST 标记为池子雷达来源
- **AND** 来源上下文 MUST 保存目标池子地址或 V4 poolId

#### Scenario: 导入已存在的钱包
- **WHEN** 用户导入的钱包已经存在于聪明钱监控列表
- **THEN** 系统 MUST 不创建重复记录
- **AND** 如果该钱包已停用，系统 MUST 将其重新启用
- **AND** 响应 MUST 返回已存在、重新启用和新创建的钱包数量

### Requirement: 双端提供池子雷达导入入口
MiniApp 和 WebApp 的聪明钱钱包视图 SHALL 提供池子雷达筛选导入入口。入口 MUST 支持输入池子地址或 V4 poolId、筛选条件、预览候选、勾选钱包和执行批量导入。

#### Scenario: 用户在 MiniApp 使用池子雷达
- **WHEN** 用户在 MiniApp 打开聪明钱钱包视图并点击池子雷达导入
- **THEN** 前端 MUST 展示适配移动端的池子标识输入和候选预览界面
- **AND** 用户点击预览后 MUST 展示候选钱包列表和交易证据
- **AND** 用户确认导入后 MUST 刷新聪明钱钱包列表

#### Scenario: 用户在 WebApp 使用池子雷达
- **WHEN** 用户在 WebApp 打开聪明钱钱包视图并点击池子雷达导入
- **THEN** 前端 MUST 展示适配桌面端的池子标识输入、候选表格和批量操作
- **AND** 用户点击预览后 MUST 展示候选钱包列表、金额来源、数据源、交易哈希、协议和区间
- **AND** 用户确认导入后 MUST 刷新聪明钱钱包列表

### Requirement: 权限与成本控制
系统 SHALL 对池子雷达预览和批量导入执行聪明钱模块权限校验，并对 RPC 扫描成本执行代码级限制。

#### Scenario: 用户没有聪明钱权限
- **WHEN** 用户没有聪明钱模块权限但请求池子雷达预览或导入
- **THEN** 系统 MUST 拒绝请求
- **AND** 前端 MUST 展示权限不足提示

#### Scenario: 扫描范围超过限制
- **WHEN** 用户请求的时间窗口、日志数量或区块范围超过系统代码常量限制
- **THEN** 系统 MUST 拒绝请求或中止扫描并返回明确错误
- **AND** 系统 MUST 提示用户缩小时间范围
