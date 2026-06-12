## 1. Specification And API
- [x] 1.1 将雷达预览接口参数从 `token_address` 调整为 `pool_address` / `pool_id`，并移除按 token address 的兼容路径。
- [x] 1.2 移除雷达数据源环境变量校验，固定使用现有 RPC 池，不再要求 Bitquery API Key。
- [x] 1.3 更新错误提示，明确 RPC 未配置、池子无效、窗口过大、日志过多、事件不可归因等错误。

## 2. Backend Provider
- [x] 2.1 新增 RPC 池子雷达能力，复用现有 RPC 池选择能力。
- [x] 2.2 实现时间范围到区块范围的查找，避免按固定出块时间粗略估算。
- [x] 2.3 实现 V3 池子扫描：识别池子协议和 PositionManager，扫描 `IncreaseLiquidity`，按 position metadata 过滤目标池子。
- [x] 2.4 实现 V4 池子扫描：扫描 PoolManager `ModifyLiquidity`，过滤目标 poolId，复用 receipt transfer 金额解析。
- [x] 2.5 抽取并复用 LP 事件解析、元数据补齐、金额计算逻辑，避免复制 watcher 中的核心解析代码。
- [x] 2.6 实现 wallet 归因：V3/V4 仅返回可验证 owner 的事件；不可归因事件进入 excluded / warning。
- [x] 2.7 按钱包聚合候选结果，保留最大金额、最近交易、交易对、池子和数据源。
- [x] 2.8 批量导入时将来源上下文保存为目标池子标识。

## 3. Frontend
- [x] 3.1 MiniApp 将筛选导入口从代币地址输入改为池子地址 / poolId 输入。
- [x] 3.2 WebApp 将筛选导入口从代币地址输入改为池子地址 / poolId 输入。
- [x] 3.3 双端候选结果展示池子标识、协议、交易对、金额来源和不可导入原因。

## 4. Validation
- [x] 4.1 增加/更新后端单元测试覆盖参数校验、RPC provider 路径选择和候选聚合。
- [x] 4.2 增加 WebServer 参数解析测试，确认窗口限制按 RPC 方案收敛。
- [x] 4.3 执行 `cd backend; go test ./service/smart_money ./service/web_server`。
- [x] 4.4 执行 MiniApp/WebApp build。
- [x] 4.5 修改完成后执行针对性 diff 检查，确认 API 字段、前后端调用和 OpenSpec 保持一致。
