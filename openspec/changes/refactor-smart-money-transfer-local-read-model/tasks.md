## 1. 规范
- [x] 1.1 为聪明钱本地转账事件持久化、钱包详情本地聚合读取、转账日盈亏口径补充 spec delta

## 2. 后端：数据模型与迁移
- [x] 2.1 新增 `SmartMoneyWalletTransferEvent` 模型、表名、唯一索引和必要字段
- [x] 2.2 将新模型接入 `backend/base/database/mysql.go` 自动迁移与兼容性补列逻辑
- [x] 2.3 在 smart money repository 中补充转账事件写入、按钱包/按日聚合和按 tx 排除查询 helper

## 3. 后端：Watcher 增量持久化转账事件
- [x] 3.1 扩展 watcher 区块快照结构，提取原生币 `value` 并识别已监控钱包的原生币转入/转出
- [x] 3.2 在 watcher 每轮扫描中按小区块窗口解析已监控钱包的 ERC20 `Transfer` 日志并持久化
- [x] 3.3 排除 LP add/remove 同 tx 造成的内部转账，避免误记为普通转账
- [x] 3.4 为 watcher 增加针对转账解析、去重与钱包方向判定的单元测试

## 4. 后端：资产管理本地聚合读取
- [x] 4.1 使用本地转账事件聚合结果生成 `sm_wallet_daily_snapshots` 的转账字段
- [x] 4.2 将聪明钱钱包详情“今天”的转账标识与金额改为读取本地聚合，不再请求时扫链
- [x] 4.3 调整盈利日历与排行榜口径，仅依赖本地快照/本地聚合的转账数据
- [x] 4.4 删除 `today transfer detection incomplete` 这类由请求时扫链引入的 warning 路径

## 5. 验证
- [x] 5.1 `cd backend && go test ./service/smart_money/... ./service/assets/...`
- [ ] 5.2 如环境可用，执行 `openspec validate refactor-smart-money-transfer-local-read-model --strict`
- [x] 5.3 若本地无 `openspec` CLI，在变更说明中明确记录无法执行校验
