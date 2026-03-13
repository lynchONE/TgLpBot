## 1. Implementation
- [x] 1.1 创建 OpenSpec 变更文档，描述热门池子代币元数据缓存与展示方案
- [x] 1.2 新增 `token_metadata` MySQL 模型并接入自动迁移
- [x] 1.3 实现代币元数据服务，打通 Redis 热缓存、MySQL 持久化与 OKX 批量接口
- [x] 1.4 改造热门池子接口，补充展示代币选择逻辑与 `display_token_*` 返回字段
- [x] 1.5 调整 `webapp` 热门池子 UI，头像改为代币图标，DEX 图标缩小放到协议标签旁
- [x] 1.6 新增或更新相关测试，并执行后端测试与前端构建校验
