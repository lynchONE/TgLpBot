## 1. Implementation
- [x] 1.1 创建 OpenSpec 变更文档并补充性能优化需求
- [x] 1.2 调整 `smart_lp_events` 表结构：TTL=2天，排序键改为面向 `ts/chain/pool/wallet`
- [x] 1.3 新增 Smart Money 小时级聚合表 / 物化视图，并回填最近 2 天数据
- [x] 1.4 将 Smart Money overview 相关 ClickHouse 查询切换到聚合表
- [x] 1.5 为 Smart Money 池信息增加 Redis 缓存，TTL=24小时
- [x] 1.6 为 Hot Pools 响应增加 Redis 缓存优先读取，TTL=10秒
- [x] 1.7 更新或新增测试，并执行格式化与基础校验
